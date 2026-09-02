package portforward

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestCancelingAStartDuringResolutionDoesNotLaunchAForward(t *testing.T) {
	runner := newStubRunner(45123)
	resolver := &gateResolver{entered: make(chan struct{}, 1), release: make(chan struct{})}
	registry := newTestRegistry(t, runner, resolver)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := registry.Start(ctx, podTarget(), 8080)
		done <- err
	}()
	<-resolver.entered

	cancel()
	close(resolver.release)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("start error = %v, want request cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the canceled start did not return after resolution")
	}
	if runner.count() != 0 {
		t.Fatalf("runner calls = %d, want none after cancellation", runner.count())
	}
	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want no canceled forward", registry.List())
	}
}

func TestShutdownDuringResolutionDoesNotLaunchAForward(t *testing.T) {
	root, cancelRoot := context.WithCancel(t.Context())
	runner := newStubRunner(45123)
	resolver := &gateResolver{entered: make(chan struct{}, 1), release: make(chan struct{})}
	registry := newRegistry(root, runner, resolver, nil, time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	done := make(chan error, 1)
	go func() {
		_, err := registry.Start(t.Context(), podTarget(), 8080)
		done <- err
	}()
	<-resolver.entered

	cancelRoot()
	close(resolver.release)

	select {
	case err := <-done:
		if !errors.Is(err, errStopped) {
			t.Fatalf("start error = %v, want stopped registry", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the start did not return after shutdown")
	}
	if runner.count() != 0 {
		t.Fatalf("runner calls = %d, want none after shutdown", runner.count())
	}
	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want no forward after shutdown", registry.List())
	}
}

func TestAResolutionStartedAfterShutdownIsAlreadyCanceled(t *testing.T) {
	root, cancelRoot := context.WithCancel(t.Context())
	cancelRoot()
	registry := &Registry{root: root}

	resolving, cancelResolve := registry.resolutionContext(t.Context())
	defer cancelResolve()

	if !errors.Is(resolving.Err(), context.Canceled) {
		t.Fatalf("resolution error = %v, want shutdown cancellation", resolving.Err())
	}
}

func TestShutdownWhileTheForwarderStartsHaltsIt(t *testing.T) {
	root, cancelRoot := context.WithCancel(t.Context())
	runner := newStubRunner(45123)
	runner.hang = true
	registry := newRegistry(
		root,
		runner,
		&stubResolver{pod: "web", podPort: 8080},
		nil,
		time.Second,
		time.Hour,
		time.Second,
	)
	t.Cleanup(registry.StopAll)
	done := make(chan error, 1)
	go func() {
		_, err := registry.Start(t.Context(), podTarget(), 8080)
		done <- err
	}()
	<-runner.entered

	cancelRoot()

	select {
	case err := <-done:
		if !errors.Is(err, errStopped) {
			t.Fatalf("start error = %v, want stopped registry", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the start did not return after shutdown")
	}
	select {
	case <-runner.finished:
	case <-time.After(time.Second):
		t.Fatal("the starting forwarder was not halted")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want no forward after shutdown", registry.List())
	}
}

func TestShutdownWakesAStartWaitingForAReservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, cancelRoot := context.WithCancel(t.Context())
		runner := newStubRunner(45123)
		registry := &Registry{
			root:     root,
			runner:   runner,
			resolver: &stubResolver{pod: "web", podPort: 8080},
			forwards: map[string]*record{},
			starting: map[startKey]chan struct{}{},
		}
		_, found, wait, stopped := registry.reserve(podTarget(), 8080)
		if found || wait != nil || stopped {
			t.Fatal("the fixture did not reserve the first start")
		}
		done := make(chan error, 1)
		go func() {
			_, err := registry.Start(t.Context(), podTarget(), 8080)
			done <- err
		}()
		synctest.Wait()

		cancelRoot()

		if err := <-done; !errors.Is(err, errStopped) {
			t.Fatalf("start error = %v, want stopped registry", err)
		}
		if runner.count() != 0 {
			t.Fatalf("runner calls = %d, want none while waiting", runner.count())
		}
	})
}

func TestAwaitingAStoppedForwardHasABoundedLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := &Registry{timeout: time.Hour}
		previous := newRun()
		done := make(chan struct{})
		started := time.Now()
		go func() {
			registry.awaitEnd(previous)
			close(done)
		}()

		<-done
		if elapsed := time.Since(started); elapsed != time.Hour {
			t.Fatalf("waited %v, want the configured timeout", elapsed)
		}
	})
}
