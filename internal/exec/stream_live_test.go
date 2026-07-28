package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

const (
	streamStdin  = 0
	streamStdout = 1
	streamErr    = 3
	streamResize = 4
	streamClose  = 255
)

type fakeKubelet struct {
	mu      sync.Mutex
	stdin   bytes.Buffer
	sizes   []Size
	path    string
	query   string
	done    chan struct{}
	closing sync.Once
}

func newFakeKubelet() *fakeKubelet {
	return &fakeKubelet{done: make(chan struct{})}
}

func (f *fakeKubelet) finish() {
	f.closing.Do(func() {
		close(f.done)
	})
}

func (f *fakeKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.path = r.URL.Path
	f.query = r.URL.RawQuery
	f.mu.Unlock()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"v5.channel.k8s.io"},
	})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx := r.Context()
	for {
		kind, data, readErr := conn.Read(ctx)
		if readErr != nil {
			return
		}
		if kind != websocket.MessageBinary {
			continue
		}
		if len(data) == 0 {
			continue
		}
		if f.handle(ctx, conn, data) {
			return
		}
	}
}

func (f *fakeKubelet) handle(ctx context.Context, conn *websocket.Conn, data []byte) bool {
	payload := data[1:]
	switch data[0] {
	case streamStdin:
		f.mu.Lock()
		f.stdin.Write(payload)
		f.mu.Unlock()
		_ = conn.Write(ctx, websocket.MessageBinary, append([]byte{streamStdout}, bytes.ToUpper(payload)...))
	case streamResize:
		var size struct {
			Width  uint16
			Height uint16
		}
		if json.Unmarshal(payload, &size) == nil {
			f.mu.Lock()
			f.sizes = append(f.sizes, Size{Cols: size.Width, Rows: size.Height})
			f.mu.Unlock()
		}
	case streamClose:
		if len(payload) == 1 && payload[0] == streamStdin {
			f.succeed(ctx, conn)
			return true
		}
	}
	return false
}

func (f *fakeKubelet) succeed(ctx context.Context, conn *websocket.Conn) {
	status, _ := json.Marshal(metav1.Status{Status: metav1.StatusSuccess})
	_ = conn.Write(ctx, websocket.MessageBinary, append([]byte{streamErr}, status...))
	_ = conn.Close(websocket.StatusNormalClosure, "")
	f.finish()
}

func (f *fakeKubelet) recorded() (string, []Size, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stdin.String(), append([]Size{}, f.sizes...), f.path, f.query
}

func TestStreamRunsAShellOverTheRealClient(t *testing.T) {
	kubelet := newFakeKubelet()
	srv := httptest.NewServer(kubelet)
	defer srv.Close()

	cs, err := kubernetes.NewForConfig(&restclient.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	streamer := NewStreamer(cs, &restclient.Config{Host: srv.URL})

	stdinReader, stdinWriter := io.Pipe()
	resize := make(chan Size, 1)
	var stdout bytes.Buffer
	failed := make(chan error, 1)

	go func() {
		failed <- streamer.Stream(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "loki"}, Options{
			Command: []string{ShellPath},
			Stdin:   stdinReader,
			Stdout:  &stdout,
			Resize:  resize,
		})
	}()

	resize <- Size{Cols: 120, Rows: 40}
	_, _ = stdinWriter.Write([]byte("uptime\n"))

	waitFor(t, func() bool {
		stdin, sizes, _, _ := kubelet.recorded()
		if stdin != "uptime\n" {
			return false
		}
		return len(sizes) == 1
	}, "the kubelet never saw both the keystrokes and the resize")
	_ = stdinWriter.Close()

	select {
	case streamErr := <-failed:
		if streamErr != nil {
			t.Fatalf("stream: %v", streamErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stream did not finish")
	}

	stdin, sizes, path, query := kubelet.recorded()
	if stdin != "uptime\n" {
		t.Fatalf("stdin = %q", stdin)
	}
	if stdout.String() != "UPTIME\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if len(sizes) != 1 {
		t.Fatalf("sizes = %v", sizes)
	}
	if sizes[0].Cols != 120 {
		t.Fatalf("cols = %d", sizes[0].Cols)
	}
	if path != "/api/v1/namespaces/monitoring/pods/loki-0/exec" {
		t.Fatalf("path = %q", path)
	}
	if !strings.Contains(query, "container=loki") {
		t.Fatalf("query = %q", query)
	}
	if !strings.Contains(query, "command=%2Fbin%2Fsh") {
		t.Fatalf("query = %q", query)
	}
	if !strings.Contains(query, "tty=true") {
		t.Fatalf("query = %q", query)
	}
}

func TestStreamReportsABrokenConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	cs, err := kubernetes.NewForConfig(&restclient.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	streamer := NewStreamer(cs, &restclient.Config{
		Host:            srv.URL,
		TLSClientConfig: restclient.TLSClientConfig{CAFile: "/nonexistent/ca.crt"},
	})

	streamErr := streamer.Stream(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0"}, Options{
		Command: []string{ShellPath},
		Stdin:   strings.NewReader(""),
		Stdout:  io.Discard,
		Resize:  make(chan Size),
	})
	if streamErr == nil {
		t.Fatal("expected an error")
	}
}

func TestStreamReportsARefusedUpgrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cs, err := kubernetes.NewForConfig(&restclient.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	streamer := NewStreamer(cs, &restclient.Config{Host: srv.URL})

	streamErr := streamer.Stream(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0"}, Options{
		Command: []string{ShellPath},
		Stdin:   strings.NewReader(""),
		Stdout:  io.Discard,
		Resize:  make(chan Size),
	})
	if streamErr == nil {
		t.Fatal("expected an error")
	}
}
