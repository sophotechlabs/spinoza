package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func benchStore(tb testing.TB) *Store {
	tb.Helper()
	store, err := Open(context.Background(), filepath.Join(tb.TempDir(), "history.db"))
	if err != nil {
		tb.Fatalf("open: %v", err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

func benchEntry() Entry {
	return Entry{
		Cluster:   "https://p-mk1:6443",
		At:        time.Unix(1700000000, 0),
		Verb:      "scale",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Kind:      "Deployment",
		Namespace: "shop",
		Name:      "web",
		Detail:    "replicas 3 to 4",
		Outcome:   "done",
	}
}

func BenchmarkRecordOnTheWritePath(b *testing.B) {
	store := benchStore(b)
	ctx := context.Background()
	entry := benchEntry()

	for b.Loop() {
		err := store.record(ctx, entry)
		if err != nil {
			b.Fatalf("record: %v", err)
		}
	}
}

func BenchmarkRecordWhileAnotherWriterContends(b *testing.B) {
	store := benchStore(b)
	ctx := context.Background()
	entry := benchEntry()

	stop := make(chan struct{})
	var contending sync.WaitGroup
	contending.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = store.record(ctx, entry)
		}
	})

	for b.Loop() {
		err := store.record(ctx, entry)
		if err != nil {
			b.Fatalf("record: %v", err)
		}
	}
	close(stop)
	contending.Wait()
}

func TestARecordLandsWellInsideTheTimeoutItIsGiven(t *testing.T) {
	store := benchStore(t)
	ctx := context.Background()
	entry := benchEntry()

	started := time.Now()
	for range 100 {
		err := store.record(ctx, entry)
		if err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	each := time.Since(started) / 100

	if each > 50*time.Millisecond {
		t.Fatalf(
			"a record took %s; the write path holds the response for up to %s and SQLite waits %s",
			each, 10*time.Second, 5*time.Second,
		)
	}
}
