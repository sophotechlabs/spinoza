package portforward

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type stubRunner struct {
	mu         sync.Mutex
	calls      int
	local      int32
	startErr   error
	lateErr    chan error
	entered    chan struct{}
	hang       bool
	neverReady bool
}

func newStubRunner(local int32) *stubRunner {
	return &stubRunner{local: local, lateErr: make(chan error, 1), entered: make(chan struct{}, 8)}
}

func (s *stubRunner) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubRunner) Run(_ context.Context, _, _ string, _ int32, ready chan<- int32, stop <-chan struct{}) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	select {
	case s.entered <- struct{}{}:
	default:
	}

	if s.startErr != nil {
		return s.startErr
	}
	if s.neverReady {
		return nil
	}
	if s.hang {
		<-stop
		return nil
	}
	ready <- s.local
	select {
	case err := <-s.lateErr:
		return err
	case <-stop:
		return nil
	}
}

type stubResolver struct {
	pod     string
	podPort int32
	err     error
}

func (s *stubResolver) Resolve(context.Context, Target, int32) (string, int32, error) {
	if s.err != nil {
		return "", 0, s.err
	}
	return s.pod, s.podPort, nil
}

func podTarget() Target {
	return Target{Kind: KindPod, Namespace: "flux-system", Name: "web"}
}

type stubProber struct {
	mu    sync.Mutex
	alive bool
}

func (s *stubProber) Alive(context.Context, string, string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

func (s *stubProber) kill() {
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
}

func newTestRegistry(t *testing.T, runner Runner, resolver Resolver) *Registry {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, runner, resolver, &stubProber{alive: true}, 2*time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	return registry
}

func TestStartRegistersARunningForward(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	forward, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if forward.LocalPort != 45123 {
		t.Fatalf("localPort = %d, want 45123", forward.LocalPort)
	}
	if forward.RemotePort != 8080 {
		t.Fatalf("remotePort = %d", forward.RemotePort)
	}
	if forward.State != StateRunning {
		t.Fatalf("state = %q", forward.State)
	}
	if forward.Pod != "web" {
		t.Fatalf("pod = %q", forward.Pod)
	}
	if forward.ID == "" || forward.StartedAt == "" {
		t.Fatalf("forward is missing an id or timestamp: %+v", forward)
	}
	if len(registry.List()) != 1 {
		t.Fatalf("list = %v", registry.List())
	}
}

func TestStartRecordsTheResolvedPodForAService(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(3000), &stubResolver{pod: "prom-0", podPort: 9090})
	target := Target{Kind: KindService, Namespace: "monitoring", Name: "prometheus"}

	forward, err := registry.Start(context.Background(), target, 9090)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if forward.Kind != KindService {
		t.Fatalf("kind = %q", forward.Kind)
	}
	if forward.Pod != "prom-0" {
		t.Fatalf("pod = %q, want the resolved backing pod", forward.Pod)
	}
}

func TestStartReusesAnIdenticalRunningForward(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	first, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	second, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("ids %q and %q differ, want the existing forward reused", first.ID, second.ID)
	}
	if runner.count() != 1 {
		t.Fatalf("runner called %d times, want 1", runner.count())
	}
}

func TestStartTreatsADifferentPortAsANewForward(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	first, _ := registry.Start(context.Background(), podTarget(), 8080)
	second, err := registry.Start(context.Background(), podTarget(), 9090)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("a different remote port must get its own forward")
	}
	if len(registry.List()) != 2 {
		t.Fatalf("list = %v", registry.List())
	}
}

func TestStartSurfacesAResolverFailure(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{err: errors.New("no ready pod")})

	_, err := registry.Start(context.Background(), podTarget(), 8080)

	if err == nil {
		t.Fatalf("expected the resolver failure to surface")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("a failed start must not be registered")
	}
}

func TestStartSurfacesAnImmediateRunFailure(t *testing.T) {
	runner := newStubRunner(0)
	runner.startErr = errors.New("pods/portforward is forbidden")
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	_, err := registry.Start(context.Background(), podTarget(), 8080)

	if err == nil {
		t.Fatalf("expected the run failure to surface")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("a failed start must not be registered")
	}
}

func TestStartReportsARunThatEndsBeforeReady(t *testing.T) {
	runner := newStubRunner(0)
	runner.neverReady = true
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	_, err := registry.Start(context.Background(), podTarget(), 8080)

	if err == nil {
		t.Fatalf("expected an error when the run ends without becoming ready")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("nothing should be registered")
	}
}

func TestStartTreatsADifferentTargetAsANewForward(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	first, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	others := []Target{
		{Kind: KindService, Namespace: "flux-system", Name: "web"},
		{Kind: KindPod, Namespace: "other", Name: "web"},
		{Kind: KindPod, Namespace: "flux-system", Name: "api"},
	}
	for _, target := range others {
		next, startErr := registry.Start(context.Background(), target, 8080)
		if startErr != nil {
			t.Fatalf("start %+v: %v", target, startErr)
		}
		if next.ID == first.ID {
			t.Fatalf("target %+v reused the forward for %+v", target, podTarget())
		}
	}
	if len(registry.List()) != 4 {
		t.Fatalf("list = %v, want four distinct forwards", registry.List())
	}
}

func TestStartTimesOut(t *testing.T) {
	runner := newStubRunner(0)
	runner.hang = true
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, runner, &stubResolver{pod: "web", podPort: 8080}, nil, 50*time.Millisecond, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	_, err := registry.Start(context.Background(), podTarget(), 8080)

	if err == nil {
		t.Fatalf("expected a timeout")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("a timed-out start must not be registered")
	}
}

func TestALateFailureMarksTheForwardFailed(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})
	forward, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	runner.lateErr <- errors.New("pod deleted")

	deadline := time.After(5 * time.Second)
	for {
		list := registry.List()
		if len(list) == 1 && list[0].State == StateFailed {
			if list[0].Error != "pod deleted" {
				t.Fatalf("error = %q", list[0].Error)
			}
			if list[0].ID != forward.ID {
				t.Fatalf("id changed on failure")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("forward never became failed: %v", list)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestAFailedForwardIsNotReused(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})
	first, _ := registry.Start(context.Background(), podTarget(), 8080)
	runner.lateErr <- errors.New("pod deleted")

	deadline := time.After(5 * time.Second)
	for registry.List()[0].State != StateFailed {
		select {
		case <-deadline:
			t.Fatalf("forward never failed")
		case <-time.After(10 * time.Millisecond):
		}
	}

	second, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("a failed forward must not be reused")
	}
	if runner.count() != 2 {
		t.Fatalf("runner called %d times, want a fresh run", runner.count())
	}
}

func TestACleanExitRemovesTheForward(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}

	runner.lateErr <- nil

	deadline := time.After(5 * time.Second)
	for len(registry.List()) != 0 {
		select {
		case <-deadline:
			t.Fatalf("forward was not removed after a clean exit")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestStopHaltsAndRemoves(t *testing.T) {
	runner := newStubRunner(45123)
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})
	forward, _ := registry.Start(context.Background(), podTarget(), 8080)

	if err := registry.Stop(forward.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want empty", registry.List())
	}
}

func TestStopIsIdempotentlySafe(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080})
	forward, _ := registry.Start(context.Background(), podTarget(), 8080)

	if err := registry.Stop(forward.ID); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := registry.Stop(forward.ID); err == nil {
		t.Fatalf("stopping an unknown id must report an error")
	}
}

func TestStopUnknownID(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{pod: "web", podPort: 8080})

	if err := registry.Stop("pf-nope"); err == nil {
		t.Fatalf("expected an error for an unknown id")
	}
}

func TestStopAllClearsEverything(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080})
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := registry.Start(context.Background(), podTarget(), 9090); err != nil {
		t.Fatalf("start: %v", err)
	}

	registry.StopAll()

	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want empty", registry.List())
	}
}

func TestListIsSortedByID(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080})
	for _, port := range []int32{8080, 9090, 7070} {
		if _, err := registry.Start(context.Background(), podTarget(), port); err != nil {
			t.Fatalf("start %d: %v", port, err)
		}
	}

	list := registry.List()

	if len(list) != 3 {
		t.Fatalf("list = %v", list)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ID >= list[i].ID {
			t.Fatalf("list is not sorted by id: %v", list)
		}
	}
}

func TestShutdownStopsEveryForward(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	registry := newRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, nil, 2*time.Second, time.Hour, time.Second)
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()

	deadline := time.After(5 * time.Second)
	for len(registry.List()) != 0 {
		select {
		case <-deadline:
			t.Fatalf("forwards survived shutdown: %v", registry.List())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestReapMarksAForwardWhosePodIsGone(t *testing.T) {
	runner := newStubRunner(45123)
	prober := &stubProber{alive: true}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, runner, &stubResolver{pod: "web", podPort: 8080}, prober, 2*time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)

	forward, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	registry.Reap()
	if registry.List()[0].State != StateRunning {
		t.Fatalf("a live pod must keep its forward running")
	}

	prober.kill()
	registry.Reap()

	list := registry.List()
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}
	if list[0].State != StateFailed {
		t.Fatalf("state = %q, want failed", list[0].State)
	}
	if list[0].Error == "" {
		t.Fatalf("a reaped forward must explain itself")
	}
	if list[0].ID != forward.ID {
		t.Fatalf("id changed during reaping")
	}
}

func TestReapLeavesFailedForwardsAlone(t *testing.T) {
	prober := &stubProber{alive: false}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, prober, 2*time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}

	registry.Reap()
	first := registry.List()[0].Error
	registry.Reap()

	if registry.List()[0].Error != first {
		t.Fatalf("reaping twice rewrote the error")
	}
}

func TestReapIsANoOpWithoutAProber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, nil, 2*time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}

	registry.Reap()

	if registry.List()[0].State != StateRunning {
		t.Fatalf("reaping without a prober must change nothing")
	}
}

func TestReapLoopRunsOnItsInterval(t *testing.T) {
	prober := &stubProber{alive: true}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, prober, 2*time.Second, 10*time.Millisecond, time.Second)
	t.Cleanup(registry.StopAll)
	if _, err := registry.Start(context.Background(), podTarget(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}

	prober.kill()

	deadline := time.After(5 * time.Second)
	for registry.List()[0].State != StateFailed {
		select {
		case <-deadline:
			t.Fatalf("the reap loop never noticed the dead pod")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestFailIgnoresAnUnknownID(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080})

	registry.fail("pf-nope", "gone")

	if len(registry.List()) != 0 {
		t.Fatalf("list = %v", registry.List())
	}
}

func TestNewRegistryUsesTheDefaultTimings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	registry := NewRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, &stubProber{alive: true})
	t.Cleanup(registry.StopAll)

	if registry.timeout != DefaultStartTimeout {
		t.Fatalf("timeout = %v", registry.timeout)
	}
	if registry.reapEvery != DefaultReapInterval {
		t.Fatalf("reapEvery = %v", registry.reapEvery)
	}
}

type gateResolver struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gateResolver) Resolve(context.Context, Target, int32) (string, int32, error) {
	g.entered <- struct{}{}
	<-g.release
	return "web", 8080, nil
}

func TestConcurrentStartsShareOneForward(t *testing.T) {
	runner := newStubRunner(45123)
	resolver := &gateResolver{entered: make(chan struct{}, 2), release: make(chan struct{})}
	registry := newTestRegistry(t, runner, resolver)

	var group sync.WaitGroup
	ids := make([]string, 2)
	errs := make([]error, 2)
	group.Go(func() {
		forward, err := registry.Start(context.Background(), podTarget(), 8080)
		ids[0] = forward.ID
		errs[0] = err
	})
	<-resolver.entered

	group.Go(func() {
		forward, err := registry.Start(context.Background(), podTarget(), 8080)
		ids[1] = forward.ID
		errs[1] = err
	})
	time.Sleep(50 * time.Millisecond)
	close(resolver.release)
	group.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	if runner.count() != 1 {
		t.Fatalf("ran %d forwards for one target, want the second call to reuse the first", runner.count())
	}
	if ids[0] != ids[1] {
		t.Fatalf("ids = %q and %q, want both callers to get the same forward", ids[0], ids[1])
	}
	if len(registry.List()) != 1 {
		t.Fatalf("registry holds %d forwards, want 1", len(registry.List()))
	}
}

func TestAFailedStartReleasesTheReservation(t *testing.T) {
	runner := newStubRunner(45123)
	runner.startErr = errors.New("upgrade refused")
	registry := newTestRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080})

	_, first := registry.Start(context.Background(), podTarget(), 8080)
	if first == nil {
		t.Fatal("expected the first start to fail")
	}

	runner.startErr = nil
	forward, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("a retry after a failed start was blocked: %v", err)
	}
	if forward.LocalPort != 45123 {
		t.Fatalf("localPort = %d", forward.LocalPort)
	}
}

func TestAResolveFailureReleasesTheReservation(t *testing.T) {
	resolver := &stubResolver{err: errors.New("no such service")}
	registry := newTestRegistry(t, newStubRunner(45123), resolver)

	_, first := registry.Start(context.Background(), podTarget(), 8080)
	if first == nil {
		t.Fatal("expected the resolve failure to surface")
	}

	resolver.err = nil
	resolver.pod = "web"
	resolver.podPort = 8080
	_, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("a retry after a resolve failure was blocked: %v", err)
	}
}

func TestReapGivesEachProbeADeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	prober := &deadlineProber{seen: make(chan bool, 4)}
	registry := newRegistry(ctx, newStubRunner(45123), &stubResolver{pod: "web", podPort: 8080}, prober, 2*time.Second, time.Hour, 25*time.Millisecond)
	t.Cleanup(registry.StopAll)
	_, err := registry.Start(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	registry.Reap()

	select {
	case bounded := <-prober.seen:
		if !bounded {
			t.Fatal("the reap probe was handed a context with no deadline; one hung apiserver call stalls every probe")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reap never probed")
	}
}

type deadlineProber struct {
	seen chan bool
}

func (d *deadlineProber) Alive(ctx context.Context, _, _ string) bool {
	_, hasDeadline := ctx.Deadline()
	select {
	case d.seen <- hasDeadline:
	default:
	}
	<-ctx.Done()
	return true
}
