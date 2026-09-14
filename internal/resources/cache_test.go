package resources

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func panicking(context.Context) (int, bool) {
	panic("the builder blew up")
}

func recovered(t *testing.T, run func()) (caught any) {
	t.Helper()
	defer func() {
		caught = recover()
	}()
	run()
	return nil
}

func TestACacheCanBeBuiltAgainAfterItsBuilderPanicked(t *testing.T) {
	held := &recent[int]{}
	caught := recovered(t, func() {
		shared(t.Context(), held, time.Now, time.Second, panicking)
	})
	if caught == nil {
		t.Fatal("the panic did not reach the caller")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	value, ok := shared(ctx, held, time.Now, time.Second, func(context.Context) (int, bool) {
		return 42, true
	})

	if !ok || value != 42 {
		t.Fatalf("value = %d, ok = %v; the build gate was never released after the panic", value, ok)
	}
}

func TestAPanickingBuildStoresNothing(t *testing.T) {
	held := &recent[int]{}

	recovered(t, func() {
		shared(t.Context(), held, time.Now, time.Second, panicking)
	})

	held.mu.Lock()
	defer held.mu.Unlock()
	if !held.at.IsZero() || held.value != 0 {
		t.Fatalf("cache = %d at %s, want nothing kept from a build that never finished", held.value, held.at)
	}
	if held.building != nil {
		t.Fatal("the build gate is still held after the panic")
	}
}

func TestAWaiterIsReleasedWhenTheBuildItWaitedOnPanics(t *testing.T) {
	held := &recent[int]{}
	entered := make(chan struct{})
	release := make(chan struct{})
	var group sync.WaitGroup
	group.Go(func() {
		recovered(t, func() {
			shared(t.Context(), held, time.Now, time.Second, func(context.Context) (int, bool) {
				close(entered)
				<-release
				panic("the builder blew up")
			})
		})
	})
	<-entered
	waited := make(chan bool, 1)
	group.Go(func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, ok := shared(ctx, held, time.Now, time.Second, func(context.Context) (int, bool) {
			return 7, true
		})
		waited <- ok
	})
	time.Sleep(20 * time.Millisecond)
	close(release)
	group.Wait()

	if !<-waited {
		t.Fatal("the waiter timed out; the panicking build never woke it")
	}
}

func TestABuildThatFailsNormallyStillStoresNothingAndReleases(t *testing.T) {
	held := &recent[int]{}
	failing := errors.New("the cluster did not answer")

	value, ok := shared(t.Context(), held, time.Now, time.Second, func(context.Context) (int, bool) {
		_ = failing
		return 0, false
	})

	if !ok || value != 0 {
		t.Fatalf("value = %d, ok = %v", value, ok)
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	if !held.at.IsZero() || held.building != nil {
		t.Fatal("a failed build was cached or left the gate held")
	}
}
