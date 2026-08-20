package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func clientFor(t *testing.T, handler http.HandlerFunc) kubernetes.Interface {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: ts.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func collect(t *testing.T, stream *Stream, want int) []string {
	t.Helper()
	lines := []string{}
	deadline := time.After(5 * time.Second)
	for len(lines) < want {
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				return lines
			}
			lines = append(lines, line.Text)
		case <-deadline:
			t.Fatalf("timed out after %d lines", len(lines))
		}
	}
	return lines
}

func TestOpenStreamsLines(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\nsecond\nthird\n"))
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	lines := collect(t, stream, 3)
	if len(lines) != 3 {
		t.Fatalf("lines = %v, want 3", lines)
	}
	if lines[0] != "first" || lines[2] != "third" {
		t.Fatalf("lines = %v", lines)
	}

	if _, ok := <-stream.Lines; ok {
		t.Fatalf("channel stayed open after the body ended")
	}
}

func TestACleanEndCarriesNoError(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\nsecond\n"))
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	drainLines(t, stream)

	if stream.Err() != nil {
		t.Fatalf("err = %v, want none for a pod that finished logging", stream.Err())
	}
}

func TestALineTooLongToReadIsReportedRatherThanEndingTheStreamQuietly(t *testing.T) {
	huge := make([]byte, maxLineBytes+1)
	for i := range huge {
		huge[i] = 'x'
	}
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\n"))
		_, _ = w.Write(huge)
		_, _ = w.Write([]byte("\n"))
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web", Follow: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	drainLines(t, stream)

	if stream.Err() == nil {
		t.Fatal("a log line the reader could not take ended the stream as if the pod had finished")
	}
}

func TestClosingAStreamIsNotAFailure(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\n"))
		flush(w)
		<-r.Context().Done()
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web", Follow: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	collect(t, stream, 1)
	stream.Close()
	drainLines(t, stream)

	if stream.Err() != nil {
		t.Fatalf("err = %v, want a deliberate close to read as a clean end", stream.Err())
	}
}

func drainLines(t *testing.T, stream *Stream) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-stream.Lines:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the stream never ended")
		}
	}
}

func TestOpenSendsLogOptions(t *testing.T) {
	query := ""
	path := ""
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		path = r.URL.Path
		_, _ = w.Write([]byte("line\n"))
	})

	stream, err := Open(context.Background(), cs, Request{
		Namespace: "flux-system",
		Name:      "web",
		Container: "app",
		TailLines: 100,
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	collect(t, stream, 1)

	if path != "/api/v1/namespaces/flux-system/pods/web/log" {
		t.Fatalf("path = %q", path)
	}
	values, parseErr := url.ParseQuery(query)
	if parseErr != nil {
		t.Fatalf("parse query %q: %v", query, parseErr)
	}
	if values.Get("container") != "app" {
		t.Fatalf("container = %q, want app", values.Get("container"))
	}
	if values.Get("follow") != "true" {
		t.Fatalf("follow = %q, want true", values.Get("follow"))
	}
	if values.Get("tailLines") != "100" {
		t.Fatalf("tailLines = %q, want 100", values.Get("tailLines"))
	}
}

func TestOpenOmitsTailLinesWhenUnset(t *testing.T) {
	query := ""
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte("line\n"))
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	collect(t, stream, 1)

	values, parseErr := url.ParseQuery(query)
	if parseErr != nil {
		t.Fatalf("parse query %q: %v", query, parseErr)
	}
	if values.Has("tailLines") {
		t.Fatalf("tailLines present without a request value: %q", query)
	}
}

func TestOpenReturnsErrorFromAPI(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web"})
	if err == nil {
		t.Fatalf("expected an error from a failing log request")
	}
}

func TestCloseStopsAFollowingStream(t *testing.T) {
	released := make(chan struct{})
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\n"))
		flush(w)
		<-r.Context().Done()
		close(released)
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web", Follow: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	lines := collect(t, stream, 1)
	if len(lines) != 1 || lines[0] != "first" {
		t.Fatalf("lines = %v", lines)
	}

	stream.Close()
	stream.Close()

	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatalf("server request was not canceled by Close")
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-stream.Lines:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("channel never closed after Close")
		}
	}
}

func TestCloseUnblocksAFullBuffer(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		for range lineBuffer * 2 {
			_, _ = w.Write([]byte("line\n"))
		}
		flush(w)
		<-r.Context().Done()
	})

	stream, err := Open(context.Background(), cs, Request{Namespace: "flux-system", Name: "web", Follow: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for len(stream.Lines) < lineBuffer {
		select {
		case <-deadline:
			t.Fatalf("buffer never filled (%d lines)", len(stream.Lines))
		default:
		}
	}

	stream.Close()

	drain := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-stream.Lines:
			if !ok {
				return
			}
		case <-drain:
			t.Fatalf("channel never closed after Close with a full buffer")
		}
	}
}

func TestCancelledContextStopsTheStream(t *testing.T) {
	cs := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first\n"))
		flush(w)
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := Open(ctx, cs, Request{Namespace: "flux-system", Name: "web", Follow: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	collect(t, stream, 1)

	cancel()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-stream.Lines:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("channel never closed after the context was canceled")
		}
	}
}

func flush(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}
