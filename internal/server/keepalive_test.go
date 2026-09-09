package server

import (
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestAQuietFeedStillSaysItIsAlive(t *testing.T) {
	backend := &flaky{}
	ts, _ := flakyServerPinging(t, backend, time.Hour, 20*time.Millisecond)
	ctx, conn := openAwkwardFeed(t, ts)

	for range 60 {
		var msg api.ServerMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatalf("reading the feed: %v", err)
		}
		if msg.Type == "alive" {
			return
		}
	}
	t.Fatal("a feed with nothing to report never said it was alive, so a hung backend looks healthy")
}
