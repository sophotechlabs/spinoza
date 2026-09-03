package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type deadlineWriter struct {
	header         http.Header
	readDeadlines  []time.Time
	writeDeadlines []time.Time
}

func TestSlowRequestBodyHitsTheReadDeadline(t *testing.T) {
	was := ordinaryReadTimeout
	ordinaryReadTimeout = 25 * time.Millisecond
	t.Cleanup(func() { ordinaryReadTimeout = was })
	srv := New(nil, testAssets(), testToken)
	handler := srv.guard(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusRequestTimeout, "the request body took too long")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_, err = fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: %s\r\n%s: %s\r\nContent-Length: 100\r\n\r\nx", ts.Listener.Addr(), AuthHeader, testToken)
	if err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusRequestTimeout)
	}
}

func TestSlowResponseReaderHitsTheWriteDeadline(t *testing.T) {
	was := ordinaryWriteTimeout
	ordinaryWriteTimeout = 25 * time.Millisecond
	t.Cleanup(func() { ordinaryWriteTimeout = was })
	failed := make(chan error, 1)
	srv := New(nil, testAssets(), testToken)
	handler := srv.guard(func(w http.ResponseWriter, _ *http.Request) {
		block := make([]byte, 64<<10)
		for {
			_, err := w.Write(block)
			if err != nil {
				failed <- err
				return
			}
		}
	})
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n%s: %s\r\n\r\n", ts.Listener.Addr(), AuthHeader, testToken)
	if err != nil {
		t.Fatalf("write request: %v", err)
	}
	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("the write deadline returned no error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a client that stopped reading held the response writer")
	}
}

func (w *deadlineWriter) Header() http.Header {
	return w.header
}

func (w *deadlineWriter) Write(body []byte) (int, error) {
	return len(body), nil
}

func (w *deadlineWriter) WriteHeader(int) {}

func (w *deadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.readDeadlines = append(w.readDeadlines, deadline)
	return nil
}

func (w *deadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.writeDeadlines = append(w.writeDeadlines, deadline)
	return nil
}

func TestOrdinaryRequestsReceiveConnectionDeadlines(t *testing.T) {
	srv := New(nil, testAssets(), testToken)
	writer := &deadlineWriter{header: http.Header{}}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:34115/api/resources", http.NoBody)
	req.Header.Set(AuthHeader, testToken)
	srv.guard(func(http.ResponseWriter, *http.Request) {})(writer, req)
	if len(writer.readDeadlines) != 1 || writer.readDeadlines[0].IsZero() {
		t.Fatalf("read deadlines = %v", writer.readDeadlines)
	}
	if len(writer.writeDeadlines) != 1 || writer.writeDeadlines[0].IsZero() {
		t.Fatalf("write deadlines = %v", writer.writeDeadlines)
	}
}

func TestOnlyRealWebSocketRoutesBypassOrdinaryDeadlines(t *testing.T) {
	for _, path := range []string{"/ws", "/api/exec", "/api/nodeshell", "/api/shell"} {
		writer := &deadlineWriter{header: http.Header{}}
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:34115"+path, http.NoBody)
		req.Header.Set(AuthHeader, testToken)
		req.Header.Set("Upgrade", "websocket")
		New(nil, testAssets(), testToken).guard(func(http.ResponseWriter, *http.Request) {})(writer, req)
		if len(writer.readDeadlines) != 0 || len(writer.writeDeadlines) != 0 {
			t.Fatalf("%s deadlines = %v / %v", path, writer.readDeadlines, writer.writeDeadlines)
		}
	}

	writer := &deadlineWriter{header: http.Header{}}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:34115/assets/chunk.js", http.NoBody)
	req.Header.Set("Upgrade", "websocket")
	New(nil, testAssets(), testToken).guard(func(http.ResponseWriter, *http.Request) {})(writer, req)
	if len(writer.readDeadlines) != 1 || len(writer.writeDeadlines) != 1 {
		t.Fatalf("ordinary upgraded-looking request deadlines = %v / %v", writer.readDeadlines, writer.writeDeadlines)
	}
}

func TestOnlyFingerprintedPublicAssetsAreCacheable(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "/assets/chunk-Ab12cd34.js", want: true},
		{path: "/assets/chunk.js", want: false},
		{path: "/assets/chunk-short.js", want: false},
		{path: "/favicon.svg", want: false},
		{path: "/api/object-name-Ab12cd34.json", want: false},
	} {
		if got := fingerprintedAsset(tc.path); got != tc.want {
			t.Fatalf("fingerprintedAsset(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}
