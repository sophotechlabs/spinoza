package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestClosingTheServerHangsUpOnEveryFeed(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	writeErr := wsjson.Write(ctx, conn, api.ClientMsg{
		Type: "subscribe", SubID: "main", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default",
	})
	if writeErr != nil {
		t.Fatalf("subscribe: %v", writeErr)
	}
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected a snapshot")
	}

	hungUp := make(chan struct{})
	go func() {
		defer close(hungUp)
		for {
			_, _, readErr := conn.Read(ctx)
			if readErr != nil {
				return
			}
		}
	}()
	srv.Close()

	select {
	case <-hungUp:
	case <-time.After(5 * time.Second):
		t.Fatal("a browser was left holding a socket the process was about to drop")
	}
}

func TestClosingTheServerDoesNotWaitForPeerHandshakes(t *testing.T) {
	feed, _ := heldPair(t)
	terminal, _ := heldPair(t)
	srv := New(&stubCluster{}, testAssets(), testToken)
	srv.sessions[&wsSession{conn: feed}] = struct{}{}
	srv.terminals[terminal] = mk1

	started := time.Now()
	srv.Close()

	if took := time.Since(started); took >= time.Second {
		t.Fatalf("shutdown took %s for peers that were not reading", took)
	}
}

func TestAFeedThatStopsReadingIsReleased(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.feedPingEvery = 10 * time.Millisecond
	srv.feedPingWait = 20 * time.Millisecond
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	waitForServer(t, func() bool { return len(srv.openSessions()) == 0 },
		"a non-reading browser kept its feed session")
}

func TestAFeedThatAnswersPingsStaysOpen(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.feedPingEvery = 10 * time.Millisecond
	srv.feedPingWait = 100 * time.Millisecond
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	go func() {
		for {
			if _, _, readErr := conn.Read(ctx); readErr != nil {
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	pingCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if err := conn.Ping(pingCtx); err != nil {
		t.Fatalf("ping after several server heartbeats: %v", err)
	}
	if len(srv.openSessions()) != 1 {
		t.Fatal("a responsive browser lost its feed session")
	}
}
