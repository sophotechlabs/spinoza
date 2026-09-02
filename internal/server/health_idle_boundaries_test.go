package server

import (
	"testing"
	"time"
)

func TestHealthWatcherStopsAtItsNextTickWhenNobodyIsWatching(t *testing.T) {
	srv := New(noCluster{}, testAssets(), testToken)
	srv.pingEvery = time.Millisecond
	srv.mu.Lock()
	srv.watching = true
	srv.mu.Unlock()
	done := make(chan struct{})
	go func() {
		srv.pingUntilNobodyIsWatching(t.Context())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the health watcher stayed alive without browser sessions")
	}
	srv.mu.Lock()
	watching := srv.watching
	srv.mu.Unlock()
	if watching {
		t.Fatal("the idle health watcher still reports itself as running")
	}
}
