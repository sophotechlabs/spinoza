package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubShell struct {
	mu      sync.Mutex
	written []byte
	cols    uint16
	rows    uint16
	out     chan []byte
	done    chan error
	closed  bool
}

func newStubShell() *stubShell {
	return &stubShell{out: make(chan []byte, 8), done: make(chan error, 1)}
}

func (f *stubShell) Read(p []byte) (int, error) {
	chunk, open := <-f.out
	if !open {
		return 0, io.EOF
	}
	return copy(p, chunk), nil
}

func (f *stubShell) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *stubShell) Resize(cols, rows uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols = cols
	f.rows = rows
}

func (f *stubShell) Done() <-chan error {
	return f.done
}

func (f *stubShell) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	close(f.out)
}

func (f *stubShell) typed() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return string(f.written)
}

func (f *stubShell) size() (uint16, uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cols, f.rows
}

func shellServer(t *testing.T, open LocalShellOpener) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	if open != nil {
		srv.UseLocalShell(open)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func decodeLocalShell(t *testing.T, body []byte) api.LocalShell {
	t.Helper()
	var found api.Capabilities
	err := json.Unmarshal(body, &found)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return found.LocalShell
}

func dialShell(t *testing.T, ts *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/shell" + query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func readShellFrame(t *testing.T, conn *websocket.Conn) (byte, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("an empty frame arrived")
	}
	return data[0], data[1:]
}

func sendShellFrame(t *testing.T, conn *websocket.Conn, channel byte, payload []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frame := append([]byte{channel}, payload...)
	err := conn.Write(ctx, websocket.MessageBinary, frame)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestABrowserTabIsToldTheLocalShellIsDesktopOnly(t *testing.T) {
	ts := shellServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/capabilities", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	support := decodeLocalShell(t, body)
	if support.Available {
		t.Fatal("a browser tab was offered a shell on the machine")
	}
	if !strings.Contains(support.Reason, "desktop") {
		t.Fatalf("the refusal does not mention the desktop app: %s", support.Reason)
	}
}

func TestTheDesktopWindowIsOfferedALocalShell(t *testing.T) {
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return newStubShell(), nil
	})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/capabilities", nil)

	if !decodeLocalShell(t, body).Available {
		t.Fatalf("the desktop window was refused a shell: %s", body)
	}
}

func TestOpeningALocalShellWithoutOneIsRefused(t *testing.T) {
	ts := shellServer(t, nil)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/shell", nil)

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestALocalShellCarriesWhatItPrints(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	shell.out <- []byte("arch@mk1 ~ %")

	channel, payload := readShellFrame(t, conn)
	if channel != api.ExecChannelStdout {
		t.Fatalf("channel = %d, want stdout", channel)
	}
	if string(payload) != "arch@mk1 ~ %" {
		t.Fatalf("payload = %q", payload)
	}
}

func TestALocalShellTakesWhatIsTyped(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	sendShellFrame(t, conn, api.ExecChannelStdin, []byte("kubectl get pods\n"))

	waitForServer(t, func() bool { return shell.typed() == "kubectl get pods\n" }, "the shell never saw the keystrokes")
}

func TestALocalShellFollowsTheWindowSize(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	sendShellFrame(t, conn, api.ExecChannelResize, []byte(`{"cols":120,"rows":40}`))

	waitForServer(t, func() bool {
		cols, rows := shell.size()
		return cols == 120 && rows == 40
	}, "the shell was never resized")
}

func TestALocalShellIgnoresASizeItCannotRead(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	sendShellFrame(t, conn, api.ExecChannelResize, []byte("not json"))
	sendShellFrame(t, conn, api.ExecChannelStdin, []byte("x"))

	waitForServer(t, func() bool { return shell.typed() == "x" }, "the shell never saw the keystrokes")
	cols, rows := shell.size()
	if cols != 0 || rows != 0 {
		t.Fatalf("an unreadable size still resized to %dx%d", cols, rows)
	}
}

func TestALocalShellIgnoresAChannelItDoesNotKnow(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	sendShellFrame(t, conn, api.ExecChannelStderr, []byte("ignored"))
	sendShellFrame(t, conn, api.ExecChannelStdin, []byte("y"))

	waitForServer(t, func() bool { return shell.typed() == "y" }, "the shell never saw the keystrokes")
}

func TestTheFirstSizeComesFromTheQuery(t *testing.T) {
	sizes := make(chan [2]uint16, 1)
	ts := shellServer(t, func(cols, rows uint16) (LocalShell, error) {
		sizes <- [2]uint16{cols, rows}
		return newStubShell(), nil
	})

	dialShell(t, ts, "?cols=100&rows=30")

	select {
	case size := <-sizes:
		if size[0] != 100 || size[1] != 30 {
			t.Fatalf("opened at %dx%d, want 100x30", size[0], size[1])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the shell was never opened")
	}
}

func TestAnUnreadableSizeFallsBackToAUsableOne(t *testing.T) {
	sizes := make(chan [2]uint16, 1)
	ts := shellServer(t, func(cols, rows uint16) (LocalShell, error) {
		sizes <- [2]uint16{cols, rows}
		return newStubShell(), nil
	})

	dialShell(t, ts, "?cols=wide&rows=0")

	select {
	case size := <-sizes:
		if size[0] != 80 || size[1] != 24 {
			t.Fatalf("opened at %dx%d, want 80x24", size[0], size[1])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the shell was never opened")
	}
}

func TestAShellThatWillNotStartIsReportedToTheTab(t *testing.T) {
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return nil, errors.New("/bin/zsh is missing")
	})
	conn := dialShell(t, ts, "")

	channel, payload := readShellFrame(t, conn)

	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d, want error", channel)
	}
	if !strings.Contains(string(payload), "/bin/zsh is missing") {
		t.Fatalf("payload = %q", payload)
	}
}

func TestTheTabHearsWhenTheShellLeaves(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	shell.done <- errors.New("the shell stopped")

	channel, payload := readShellFrame(t, conn)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d, want error", channel)
	}
	if string(payload) != "the shell stopped" {
		t.Fatalf("payload = %q", payload)
	}
}

func TestACleanExitSaysNothingInParticular(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	shell.done <- nil

	channel, payload := readShellFrame(t, conn)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d, want error", channel)
	}
	if len(payload) != 0 {
		t.Fatalf("payload = %q, want nothing", payload)
	}
}

func TestALocalShellIgnoresATextFrame(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	if err := conn.Write(t.Context(), websocket.MessageText, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sendShellFrame(t, conn, api.ExecChannelStdin, []byte("after"))

	waitForServer(t, func() bool { return shell.typed() == "after" }, "the shell never saw the keystrokes")
	if strings.Contains(shell.typed(), "hello") {
		t.Fatalf("typed = %q, want the text frame left out", shell.typed())
	}
}

func TestALocalShellIgnoresAnEmptyFrame(t *testing.T) {
	shell := newStubShell()
	ts := shellServer(t, func(uint16, uint16) (LocalShell, error) {
		return shell, nil
	})
	conn := dialShell(t, ts, "")

	if err := conn.Write(t.Context(), websocket.MessageBinary, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	sendShellFrame(t, conn, api.ExecChannelStdin, []byte("after"))

	waitForServer(t, func() bool { return shell.typed() == "after" }, "the shell never saw the keystrokes")
}
