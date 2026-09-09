package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func drainServer(t *testing.T) *Server {
	t.Helper()
	return New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
}

func drainHeldSocket(t *testing.T) *websocket.Conn {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := accept(w, r)
		if err != nil {
			return
		}
		accepted <- socket
	}))
	defer ts.Close()
	client, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.CloseNow() })
	held := <-accepted
	t.Cleanup(func() { _ = held.CloseNow() })
	return held
}

func drainOpenFeed(t *testing.T) (*Server, *websocket.Conn) {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	conn, _, err := websocket.Dial(t.Context(), wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	drainAwaitFeeds(t, srv)
	return srv, conn
}

func drainAwaitFeeds(t *testing.T, srv *Server) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.openSessions()) > 0 {
			return
		}
		time.Sleep(drainStep)
	}
	t.Fatal("the feed never reached the server, so this test proves nothing")
}

func TestDrainWithNothingOpenComesBackAtOnce(t *testing.T) {
	srv := drainServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	started := time.Now()
	srv.Drain(ctx)
	waited := time.Since(started)

	if waited > time.Second {
		t.Fatalf("draining nothing took %s, want it to come straight back", waited)
	}
	if srv.Ready().Ready {
		t.Fatal("the drain ran and readiness still said yes")
	}
}

func TestDrainCalledTwiceIsStillSafe(t *testing.T) {
	srv := drainServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	srv.Drain(ctx)
	srv.Drain(ctx)

	if srv.Ready().Ready {
		t.Fatal("the drain ran twice and readiness still said yes")
	}
}

func TestDrainClosesAFeedWithTheCodeTheClientReconnectsOn(t *testing.T) {
	srv, conn := drainOpenFeed(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Drain(ctx)
	}()

	readErr := drainReadUntilClosed(ctx, conn)
	<-done

	if websocket.CloseStatus(readErr) == CloseStaleToken {
		t.Fatalf("the feed was closed with %d, which the page treats as final and never retries", CloseStaleToken)
	}
	if websocket.CloseStatus(readErr) != websocket.StatusServiceRestart {
		t.Fatalf("close status = %v (%v), want %d so the page reconnects",
			websocket.CloseStatus(readErr), readErr, websocket.StatusServiceRestart)
	}
}

func drainReadUntilClosed(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return err
		}
	}
}

func TestDrainStopsWaitingForAStreamThatWillNotFinish(t *testing.T) {
	srv := drainServer(t)
	held := drainHeldSocket(t)
	srv.trackExec(held, stubClusterID)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	srv.Drain(ctx)
	waited := time.Since(started)

	if waited < 200*time.Millisecond {
		t.Fatalf("the drain gave up after %s, want it to wait for the stream until the deadline", waited)
	}
	if waited > 10*time.Second {
		t.Fatalf("the drain took %s, want the deadline to bound it", waited)
	}
	if open := srv.terminalsOn(stubClusterID); len(open) != 0 {
		t.Fatalf("%d streams were still open after the deadline passed", len(open))
	}
	if drainStillOpen(t, held) {
		t.Fatal("the stream that would not finish was left open after the deadline passed")
	}
}

func drainStillOpen(t *testing.T, held *websocket.Conn) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	defer cancel()
	return held.Write(ctx, websocket.MessageText, []byte("still here")) == nil
}

func TestDrainWaitsForAStreamThatFinishesInTime(t *testing.T) {
	srv := drainServer(t)
	held := drainHeldSocket(t)
	srv.trackExec(held, stubClusterID)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		time.Sleep(4 * drainStep)
		srv.forgetExec(held)
	}()
	srv.Drain(ctx)
	<-finished

	if !drainStillOpen(t, held) {
		t.Fatal("a stream that finished on its own was closed under it")
	}
}

func TestADrainingServerRefusesAnExecAndANodeShell(t *testing.T) {
	for _, path := range []string{"/api/exec", "/api/nodeshell", "/api/shell"} {
		t.Run(path, func(t *testing.T) {
			srv := drainServer(t)
			srv.drainBegin()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path+"?namespace=prod&pod=web&node=one", http.NoBody)
			release, allowed := srv.admitLive(rec, req)

			if allowed || release != nil {
				t.Fatal("a draining server admitted a new live session")
			}
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
			if !strings.Contains(rec.Body.String(), drainReason) {
				t.Fatalf("body = %q, which does not say why", rec.Body.String())
			}
		})
	}
}

func TestAServerThatIsNotDrainingAdmitsALiveSession(t *testing.T) {
	srv := drainServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/exec", http.NoBody)
	release, allowed := srv.admitLive(rec, req)

	if !allowed || release == nil {
		t.Fatalf("a live session was refused with %d: %s", rec.Code, rec.Body.String())
	}
	release()
}
