package portforward

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubRunner struct {
	mu         sync.Mutex
	calls      int
	local      int32
	asked      []int32
	pods       []string
	startErr   error
	lateErr    chan error
	entered    chan struct{}
	finished   chan struct{}
	gate       chan struct{}
	hang       bool
	neverReady bool
}

func newStubRunner(local int32) *stubRunner {
	return &stubRunner{
		local:    local,
		lateErr:  make(chan error, 1),
		entered:  make(chan struct{}, 8),
		finished: make(chan struct{}, 8),
	}
}

func (s *stubRunner) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubRunner) requested() []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int32{}, s.asked...)
}

func (s *stubRunner) forwarded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.pods...)
}

func (s *stubRunner) Run(_ context.Context, _, pod string, localPort, _ int32, ready chan<- int32, stop <-chan struct{}) error {
	s.mu.Lock()
	s.calls++
	s.asked = append(s.asked, localPort)
	s.pods = append(s.pods, pod)
	s.mu.Unlock()
	select {
	case s.entered <- struct{}{}:
	default:
	}
	defer func() {
		select {
		case s.finished <- struct{}{}:
		default:
		}
	}()

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
	if s.gate != nil {
		<-s.gate
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
	mu      sync.Mutex
	pod     string
	podPort int32
	err     error
}

func (s *stubResolver) Resolve(context.Context, Target, int32) (string, int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", 0, s.err
	}
	return s.pod, s.podPort, nil
}

func (s *stubResolver) moveTo(pod string, err error) {
	s.mu.Lock()
	s.pod = pod
	s.err = err
	s.mu.Unlock()
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

func replaceableRegistry(t *testing.T, runner Runner, resolver Resolver, prober Prober) *Registry {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, runner, resolver, prober, 2*time.Second, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)
	return registry
}

func webService() Target {
	return Target{Kind: KindService, Namespace: "flux-system", Name: "web"}
}

func TestAForwardFollowsItsServiceOntoAReplacementPod(t *testing.T) {
	runner := newStubRunner(45123)
	resolver := &stubResolver{pod: "web-abc", podPort: 8080}
	prober := &stubProber{alive: true}
	registry := replaceableRegistry(t, runner, resolver, prober)

	forward, err := registry.Start(context.Background(), webService(), 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	prober.kill()
	resolver.moveTo("web-def", nil)
	registry.Reap()

	list := registry.List()
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}
	if list[0].State != StateRunning {
		t.Fatalf("state = %q (%s), want the forward moved rather than failed", list[0].State, list[0].Error)
	}
	if list[0].Pod != "web-def" {
		t.Fatalf("pod = %q, want the replacement", list[0].Pod)
	}
	if list[0].ID != forward.ID || list[0].LocalPort != forward.LocalPort {
		t.Fatalf("forward = %+v, want the same id and local port kept", list[0])
	}
	if want := []int32{0, 45123}; !slices.Equal(runner.requested(), want) {
		t.Fatalf("local ports asked for = %v, want %v", runner.requested(), want)
	}
	if want := []string{"web-abc", "web-def"}; !slices.Equal(runner.forwarded(), want) {
		t.Fatalf("pods forwarded = %v, want %v", runner.forwarded(), want)
	}
}

func TestAForwardThatCannotBeResolvedAgainFails(t *testing.T) {
	resolver := &stubResolver{pod: "web-abc", podPort: 8080}
	prober := &stubProber{alive: true}
	registry := replaceableRegistry(t, newStubRunner(45123), resolver, prober)

	if _, err := registry.Start(context.Background(), webService(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}
	prober.kill()
	resolver.moveTo("", errors.New("no ready pod backs the service"))
	registry.Reap()

	list := registry.List()
	if list[0].State != StateFailed {
		t.Fatalf("state = %q, want failed", list[0].State)
	}
	if !strings.Contains(list[0].Error, "no ready pod") {
		t.Fatalf("error = %q, want the resolver reason", list[0].Error)
	}
}

func TestAReplacementThatWillNotStartFails(t *testing.T) {
	runner := newStubRunner(45123)
	resolver := &stubResolver{pod: "web-abc", podPort: 8080}
	prober := &stubProber{alive: true}
	registry := replaceableRegistry(t, runner, resolver, prober)

	if _, err := registry.Start(context.Background(), webService(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}
	prober.kill()
	resolver.moveTo("web-def", nil)
	runner.mu.Lock()
	runner.startErr = errors.New("pods/portforward is forbidden")
	runner.mu.Unlock()
	registry.Reap()

	list := registry.List()
	if list[0].State != StateFailed {
		t.Fatalf("state = %q, want failed", list[0].State)
	}
	if !strings.Contains(list[0].Error, "forbidden") {
		t.Fatalf("error = %q, want the run failure", list[0].Error)
	}
}

func TestAForwardThatArrivesAfterShutdownIsRefused(t *testing.T) {
	runner := newStubRunner(45123)
	runner.gate = make(chan struct{})
	registry := replaceableRegistry(t, runner, &stubResolver{pod: "web", podPort: 8080}, nil)

	failed := make(chan error, 1)
	go func() {
		_, err := registry.Start(context.Background(), podTarget(), 8080)
		failed <- err
	}()
	<-runner.entered
	registry.StopAll()
	close(runner.gate)

	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("a forward registered into a registry that had already shut down")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("start never returned")
	}
	if len(registry.List()) != 0 {
		t.Fatalf("list = %v, want the late forward left out", registry.List())
	}
	select {
	case <-runner.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the orphaned forwarder was never halted")
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

type failingGateResolver struct {
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	err     error
	calls   int
}

func (g *failingGateResolver) Resolve(context.Context, Target, int32) (string, int32, error) {
	g.mu.Lock()
	g.calls++
	call := g.calls
	g.mu.Unlock()
	if call == 1 {
		close(g.entered)
		<-g.release
		return "", 0, g.err
	}
	return "web", 8080, nil
}

func TestConcurrentStartsShareOneForward(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
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
	})
}

func TestAConcurrentStartRetriesAfterTheLeaderFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("the pod is not ready")
		resolver := &failingGateResolver{
			entered: make(chan struct{}),
			release: make(chan struct{}),
			err:     failure,
		}
		runner := newStubRunner(45123)
		registry := newTestRegistry(t, runner, resolver)

		var group sync.WaitGroup
		forwards := make([]api.PortForward, 2)
		errs := make([]error, 2)
		group.Go(func() {
			forwards[0], errs[0] = registry.Start(context.Background(), podTarget(), 8080)
		})
		<-resolver.entered
		group.Go(func() {
			forwards[1], errs[1] = registry.Start(context.Background(), podTarget(), 8080)
		})
		synctest.Wait()
		close(resolver.release)
		group.Wait()

		if !errors.Is(errs[0], failure) {
			t.Fatalf("leader error = %v", errs[0])
		}
		if errs[1] != nil {
			t.Fatalf("waiting start: %v", errs[1])
		}
		if forwards[1].ID == "" {
			t.Fatal("the waiting start did not create a forward")
		}
		if runner.count() != 1 {
			t.Fatalf("runner calls = %d, want only the successful retry", runner.count())
		}
		if len(registry.List()) != 1 {
			t.Fatalf("registry holds %d forwards, want the retry", len(registry.List()))
		}
	})
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

func TestReplaceIsRefusedOnceTheRegistryIsStopped(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{pod: "web", podPort: 8080})
	registry.StopAll()

	previous, taken := registry.beginReplace("pf-1")

	if taken || previous != nil {
		t.Fatalf("taken = %v, previous = %v; want a stopped registry to refuse", taken, previous)
	}
}

func TestReplaceIsRefusedForAForwardThatIsNotThere(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{pod: "web", podPort: 8080})

	previous, taken := registry.beginReplace("pf-missing")

	if taken || previous != nil {
		t.Fatalf("taken = %v; want an unknown forward to be refused", taken)
	}
}

func TestReplaceIsRefusedWhileOneIsAlreadyInFlight(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{pod: "web", podPort: 8080})
	registry.mu.Lock()
	registry.forwards["pf-1"] = &record{
		forward:   api.PortForward{ID: "pf-1"},
		current:   newRun(),
		replacing: true,
	}
	registry.mu.Unlock()

	_, taken := registry.beginReplace("pf-1")

	if taken {
		t.Fatal("taken = true; want the second replace to stand down")
	}
}

func TestRestartFailureNamesWhatWentWrong(t *testing.T) {
	forward := api.PortForward{Namespace: "prod", Name: "web"}

	silent := restartFailure(forward, "web-0", nil)
	if silent != "the port forward to prod/web-0 ended before it was ready" {
		t.Fatalf("message = %q", silent)
	}

	loud := restartFailure(forward, "web-0", errors.New("pod deleted"))
	if !strings.Contains(loud, "pod deleted") {
		t.Fatalf("message = %q, want it to carry the reason", loud)
	}
}

func TestAdoptingIsDroppedForAForwardThatIsGone(t *testing.T) {
	registry := newTestRegistry(t, newStubRunner(1), &stubResolver{pod: "web", podPort: 8080})
	failed := make(chan error)

	registry.adopt("pf-missing", "web-0", 45123, newRun(), failed)

	registry.mu.Lock()
	held := len(registry.forwards)
	registry.mu.Unlock()
	if held != 0 {
		t.Fatalf("forwards = %d, want nothing adopted for a forward that is gone", held)
	}
}

func TestAMoveThatNeverBecomesReadyFails(t *testing.T) {
	runner := newStubRunner(45123)
	resolver := &stubResolver{pod: "web-abc", podPort: 8080}
	prober := &stubProber{alive: true}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := newRegistry(ctx, runner, resolver, prober, 60*time.Millisecond, time.Hour, time.Second)
	t.Cleanup(registry.StopAll)

	if _, err := registry.Start(context.Background(), webService(), 8080); err != nil {
		t.Fatalf("start: %v", err)
	}
	runner.mu.Lock()
	runner.hang = true
	runner.mu.Unlock()
	prober.kill()
	resolver.moveTo("web-def", nil)
	registry.Reap()

	deadline := time.After(5 * time.Second)
	for registry.List()[0].State != StateFailed {
		select {
		case <-deadline:
			t.Fatalf("the forward never failed: %+v", registry.List()[0])
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !strings.Contains(registry.List()[0].Error, "timed out moving") {
		t.Fatalf("error = %q, want it to say the move timed out", registry.List()[0].Error)
	}
}
