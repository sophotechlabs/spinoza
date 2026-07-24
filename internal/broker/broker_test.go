package broker

import (
	"context"
	"testing"
	"time"
)

func TestNewStubSeedsFivePods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewStub(ctx)
	rows, rv := b.Snapshot()
	if len(rows) != 5 {
		t.Fatalf("seeded rows = %d, want 5", len(rows))
	}
	if rv != "0" {
		t.Fatalf("initial rv = %q, want 0", rv)
	}
}

func TestStubSubscribeCancelIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewStub(ctx)
	ch, unsub := b.Subscribe()

	unsub()
	_, open := <-ch
	if open {
		t.Fatal("channel still open after cancel")
	}
	unsub()
}

func TestStubTickAddsThenDeletesEphemeral(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := newStub(ctx, time.Hour)
	ch, unsub := s.Subscribe()
	defer unsub()

	blocked := make(chan Event)
	s.mu.Lock()
	s.subs[blocked] = struct{}{}
	s.mu.Unlock()

	s.tick()
	added := <-ch
	if added.Kind != "added" {
		t.Fatalf("first tick kind = %q, want added", added.Kind)
	}
	if added.Row.Name != "ephemeral-job-1" {
		t.Fatalf("added name = %q, want ephemeral-job-1", added.Row.Name)
	}
	rows, rv := s.Snapshot()
	if len(rows) != 6 {
		t.Fatalf("rows after add = %d, want 6", len(rows))
	}
	if rv != "1" {
		t.Fatalf("rv after add = %q, want 1", rv)
	}

	s.tick()
	deleted := <-ch
	if deleted.Kind != "deleted" {
		t.Fatalf("second tick kind = %q, want deleted", deleted.Kind)
	}
	if deleted.UID != "stub-ephemeral" {
		t.Fatalf("deleted uid = %q, want stub-ephemeral", deleted.UID)
	}
	rows, rv = s.Snapshot()
	if len(rows) != 5 {
		t.Fatalf("rows after delete = %d, want 5", len(rows))
	}
	if rv != "2" {
		t.Fatalf("rv after delete = %q, want 2", rv)
	}
}

func TestStubLoopTicksAndStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := newStub(ctx, 2*time.Millisecond)
	ch, unsub := s.Subscribe()
	defer unsub()

	select {
	case ev := <-ch:
		if ev.Kind != "added" {
			t.Fatalf("first auto tick kind = %q, want added", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not tick")
	}

	cancel()
	time.Sleep(40 * time.Millisecond)
}
