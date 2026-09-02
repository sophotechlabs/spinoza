package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestFeedHeartbeatCancelsASessionThatStoppedAnswering(t *testing.T) {
	sess, peer, _ := rawSession(t, nil)
	_ = peer.CloseNow()
	sess.pingEvery = time.Millisecond
	sess.pingWait = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		sess.keepAlive(ctx, cancel)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the heartbeat kept a feed whose peer had gone away")
	}
	if ctx.Err() == nil {
		t.Fatal("the failed heartbeat returned without canceling the session")
	}
}

func TestTableSubscriptionAtTheConnectionLimitIsRefused(t *testing.T) {
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	ts := awkwardServer(t, &awkward{hold: hold})
	ctx, conn := openAwkwardFeed(t, ts)

	for index := range maxSubscriptions {
		sendMsg(ctx, t, conn, api.ClientMsg{
			Type:      "logs-subscribe",
			SubID:     fmt.Sprintf("logs-%d", index),
			Namespace: "default",
			Name:      "web",
		})
	}
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type: "subscribe", SubID: "one-too-many", Resource: "pods",
	})

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError || msg.SubID != "one-too-many" {
		t.Fatalf("message = %+v, want the refused table subscription", msg)
	}
	if msg.Message != "this connection already holds the maximum number of subscriptions" {
		t.Fatalf("message = %q", msg.Message)
	}
}
