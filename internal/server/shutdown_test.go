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
