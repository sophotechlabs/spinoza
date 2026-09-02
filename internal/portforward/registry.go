package portforward

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	KindPod     = "Pod"
	KindService = "Service"

	StateRunning = "running"
	StateFailed  = "failed"

	DefaultStartTimeout = 15 * time.Second
	DefaultReapInterval = 10 * time.Second
	DefaultProbeTimeout = 5 * time.Second
)

type startKey struct {
	kind      string
	namespace string
	name      string
	port      int32
}

type Target struct {
	Kind      string
	Namespace string
	Name      string
}

type Runner interface {
	Run(ctx context.Context, namespace, pod string, localPort, remotePort int32, ready chan<- int32, stop <-chan struct{}) error
}

type Resolver interface {
	Resolve(ctx context.Context, target Target, port int32) (pod string, podPort int32, err error)
}

type Prober interface {
	Alive(ctx context.Context, namespace, pod string) bool
}

type run struct {
	stop  chan struct{}
	ended chan struct{}
	once  sync.Once
}

func newRun() *run {
	return &run{stop: make(chan struct{}), ended: make(chan struct{})}
}

func (r *run) halt() {
	r.once.Do(func() {
		close(r.stop)
	})
}

type record struct {
	forward   api.PortForward
	current   *run
	actor     auth.Identity
	acting    bool
	replacing bool
}

type Registry struct {
	root         context.Context
	runner       Runner
	resolver     Resolver
	prober       Prober
	now          func() time.Time
	nextID       func() string
	timeout      time.Duration
	reapEvery    time.Duration
	probeTimeout time.Duration

	mu       sync.Mutex
	forwards map[string]*record
	starting map[startKey]chan struct{}
	sequence int
	stopped  bool
}

var errStopped = errors.New("port forwarding has stopped")

func NewRegistry(root context.Context, runner Runner, resolver Resolver, prober Prober) *Registry {
	return newRegistry(root, runner, resolver, prober, DefaultStartTimeout, DefaultReapInterval, DefaultProbeTimeout)
}

func newRegistry(
	root context.Context,
	runner Runner,
	resolver Resolver,
	prober Prober,
	timeout time.Duration,
	reapEvery time.Duration,
	probeTimeout time.Duration,
) *Registry {
	registry := &Registry{
		root:         root,
		runner:       runner,
		resolver:     resolver,
		prober:       prober,
		now:          time.Now,
		timeout:      timeout,
		reapEvery:    reapEvery,
		probeTimeout: probeTimeout,
		forwards:     map[string]*record{},
		starting:     map[startKey]chan struct{}{},
	}
	registry.nextID = registry.sequentialID
	safe.Go("stopping forwards on shutdown", registry.stopOnShutdown)
	safe.Go("reaping dead forwards", registry.reapLoop)
	return registry
}

func (r *Registry) reapLoop() {
	if r.prober == nil {
		return
	}
	ticker := time.NewTicker(r.reapEvery)
	defer ticker.Stop()
	for {
		select {
		case <-r.root.Done():
			return
		case <-ticker.C:
			r.Reap()
		}
	}
}

func (r *Registry) Reap() {
	r.reap(r.root)
}

func (r *Registry) reap(ctx context.Context) {
	if r.prober == nil {
		return
	}
	for _, forward := range r.List() {
		if forward.State != StateRunning {
			continue
		}
		if r.aliveWithin(ctx, forward.Namespace, forward.Pod) {
			continue
		}
		r.replace(ctx, forward)
	}
}

func (r *Registry) replace(ctx context.Context, forward api.PortForward) {
	who, acting, found := r.runIdentity(forward.ID)
	if !found {
		return
	}
	target := Target{Kind: forward.Kind, Namespace: forward.Namespace, Name: forward.Name}
	resolveAs := ctx
	runAs := ctx
	if acting {
		resolveAs = auth.WithIdentity(resolveAs, who)
		runAs = auth.WithIdentity(runAs, who)
	}
	pod, podPort, err := r.resolver.Resolve(resolveAs, target, forward.RemotePort)
	if err != nil {
		r.fail(forward.ID, fmt.Sprintf("pod %s/%s is gone and %s/%s no longer resolves: %v",
			forward.Namespace, forward.Pod, forward.Namespace, forward.Name, err))
		return
	}
	if pod == forward.Pod {
		r.fail(forward.ID, fmt.Sprintf("pod %s/%s is gone", forward.Namespace, forward.Pod))
		return
	}
	previous, taken := r.beginReplace(forward.ID)
	if !taken {
		return
	}
	previous.halt()
	r.awaitEnd(previous)
	r.restart(runAs, forward, pod, podPort)
}

func (r *Registry) runIdentity(id string) (auth.Identity, bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, present := r.forwards[id]
	if !present {
		return auth.Identity{}, false, false
	}
	return entry.actor, entry.acting, true
}

func (r *Registry) beginReplace(id string) (*run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil, false
	}
	entry, present := r.forwards[id]
	if !present {
		return nil, false
	}
	if entry.replacing {
		return nil, false
	}
	entry.replacing = true
	return entry.current, true
}

func (r *Registry) awaitEnd(previous *run) {
	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	select {
	case <-previous.ended:
	case <-timer.C:
	}
}

func (r *Registry) restart(ctx context.Context, forward api.PortForward, pod string, podPort int32) {
	active := newRun()
	ready := make(chan int32, 1)
	failed := make(chan error, 1)
	safe.Go("moving the forward to "+forward.Namespace+"/"+pod, func() {
		failed <- r.runner.Run(ctx, forward.Namespace, pod, forward.LocalPort, podPort, ready, active.stop)
	})

	select {
	case local := <-ready:
		r.adopt(forward.ID, pod, local, active, failed)
	case runErr := <-failed:
		active.halt()
		r.fail(forward.ID, restartFailure(forward, pod, runErr))
	case <-time.After(r.timeout):
		active.halt()
		r.fail(forward.ID, fmt.Sprintf("timed out moving the port forward to %s/%s", forward.Namespace, pod))
	}
}

func restartFailure(forward api.PortForward, pod string, err error) string {
	if err == nil {
		return fmt.Sprintf("the port forward to %s/%s ended before it was ready", forward.Namespace, pod)
	}
	return fmt.Sprintf("could not move the port forward to %s/%s: %v", forward.Namespace, pod, err)
}

func (r *Registry) adopt(id, pod string, local int32, active *run, failed <-chan error) {
	r.mu.Lock()
	entry, present := r.forwards[id]
	if !present || r.stopped {
		r.mu.Unlock()
		active.halt()
		return
	}
	entry.current = active
	entry.replacing = false
	entry.forward.Pod = pod
	entry.forward.LocalPort = local
	entry.forward.State = StateRunning
	entry.forward.Error = ""
	r.mu.Unlock()

	safe.Go("watching the forward "+id, func() { r.watch(id, active, failed) })
}

func (r *Registry) aliveWithin(ctx context.Context, namespace, pod string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, r.probeTimeout)
	defer cancel()
	return r.prober.Alive(probeCtx, namespace, pod)
}

func (r *Registry) fail(id, message string) {
	r.mu.Lock()
	entry, present := r.forwards[id]
	if present {
		entry.forward.State = StateFailed
		entry.forward.Error = message
		entry.replacing = false
	}
	r.mu.Unlock()
	if present {
		entry.current.halt()
	}
}

func (r *Registry) sequentialID() string {
	r.sequence++
	return fmt.Sprintf("pf-%d", r.sequence)
}

func (r *Registry) stopOnShutdown() {
	<-r.root.Done()
	r.StopAll()
}

func (r *Registry) Start(ctx context.Context, target Target, port int32) (api.PortForward, error) {
	existing, found, reserveErr := r.reserveStart(ctx, target, port)
	if reserveErr != nil {
		return api.PortForward{}, reserveErr
	}
	if found {
		return existing, nil
	}
	defer r.release(startKey{kind: target.Kind, namespace: target.Namespace, name: target.Name, port: port})

	resolving, cancelResolve := r.resolutionContext(ctx)
	defer cancelResolve()
	pod, podPort, err := r.resolver.Resolve(resolving, target, port)
	if err != nil {
		return api.PortForward{}, err
	}
	if ctx.Err() != nil {
		return api.PortForward{}, ctx.Err()
	}
	if r.root.Err() != nil {
		return api.PortForward{}, errStopped
	}

	active := newRun()
	ready := make(chan int32, 1)
	failed := make(chan error, 1)
	held := auth.Carry(ctx, r.root)
	actor, acting := auth.ActingAs(held)
	safe.Go("forwarding to "+target.Namespace+"/"+pod, func() {
		failed <- r.runner.Run(held, target.Namespace, pod, 0, podPort, ready, active.stop)
	})

	select {
	case local := <-ready:
		return r.register(target, port, pod, local, active, failed, actor, acting)
	case runErr := <-failed:
		active.halt()
		if runErr == nil {
			runErr = errors.New("the port forward ended before it was ready")
		}
		return api.PortForward{}, runErr
	case <-ctx.Done():
		active.halt()
		return api.PortForward{}, ctx.Err()
	case <-r.root.Done():
		active.halt()
		return api.PortForward{}, errStopped
	case <-time.After(r.timeout):
		active.halt()
		return api.PortForward{}, fmt.Errorf("timed out starting a port forward to %s/%s", target.Namespace, target.Name)
	}
}

func (r *Registry) reserveStart(
	ctx context.Context,
	target Target,
	port int32,
) (api.PortForward, bool, error) {
	for {
		existing, found, wait, stopped := r.reserve(target, port)
		if stopped {
			return api.PortForward{}, false, errStopped
		}
		if wait == nil {
			return existing, found, nil
		}
		select {
		case <-ctx.Done():
			return api.PortForward{}, false, ctx.Err()
		case <-r.root.Done():
			return api.PortForward{}, false, errStopped
		case <-wait:
		}
	}
}

func (r *Registry) resolutionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	resolving, cancel := context.WithCancel(ctx)
	if r.root.Err() != nil {
		cancel()
		return resolving, cancel
	}
	safe.Go("canceling a port-forward resolution on shutdown", func() {
		select {
		case <-r.root.Done():
			cancel()
		case <-resolving.Done():
		}
	})
	return resolving, cancel
}

func (r *Registry) register(
	target Target,
	port int32,
	pod string,
	local int32,
	active *run,
	failed <-chan error,
	actor auth.Identity,
	acting bool,
) (api.PortForward, error) {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		active.halt()
		return api.PortForward{}, fmt.Errorf("the connection to %s closed while the port forward was starting", target.Namespace)
	}
	forward := api.PortForward{
		ID:         r.nextID(),
		Kind:       target.Kind,
		Namespace:  target.Namespace,
		Name:       target.Name,
		Pod:        pod,
		RemotePort: port,
		LocalPort:  local,
		State:      StateRunning,
		StartedAt:  r.now().UTC().Format(time.RFC3339),
	}
	r.forwards[forward.ID] = &record{forward: forward, current: active, actor: actor, acting: acting}
	r.mu.Unlock()

	safe.Go("watching the forward "+forward.ID, func() { r.watch(forward.ID, active, failed) })
	return forward, nil
}

func (r *Registry) watch(id string, active *run, failed <-chan error) {
	err := <-failed
	close(active.ended)

	r.mu.Lock()
	defer r.mu.Unlock()
	entry, present := r.forwards[id]
	if !present {
		return
	}
	if entry.current != active {
		return
	}
	if entry.replacing {
		return
	}
	if entry.forward.State == StateFailed {
		return
	}
	if err == nil {
		delete(r.forwards, id)
		return
	}
	entry.forward.State = StateFailed
	entry.forward.Error = err.Error()
}

func (r *Registry) reserve(target Target, port int32) (api.PortForward, bool, <-chan struct{}, bool) {
	key := startKey{kind: target.Kind, namespace: target.Namespace, name: target.Name, port: port}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return api.PortForward{}, false, nil, true
	}
	forward, found := r.matchLocked(target, port)
	if found {
		return forward, true, nil, false
	}
	pending, busy := r.starting[key]
	if busy {
		return api.PortForward{}, false, pending, false
	}
	r.starting[key] = make(chan struct{})
	return api.PortForward{}, false, nil, false
}

func (r *Registry) release(key startKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, present := r.starting[key]
	if !present {
		return
	}
	delete(r.starting, key)
	close(pending)
}

func (r *Registry) matchLocked(target Target, port int32) (api.PortForward, bool) {
	for _, entry := range r.forwards {
		if entry.forward.State != StateRunning {
			continue
		}
		if entry.forward.Kind != target.Kind {
			continue
		}
		if entry.forward.Namespace != target.Namespace {
			continue
		}
		if entry.forward.Name != target.Name {
			continue
		}
		if entry.forward.RemotePort != port {
			continue
		}
		return entry.forward, true
	}
	return api.PortForward{}, false
}

func (r *Registry) List() []api.PortForward {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]api.PortForward, 0, len(r.forwards))
	for _, entry := range r.forwards {
		out = append(out, entry.forward)
	}
	slices.SortFunc(out, func(left, right api.PortForward) int {
		return strings.Compare(left.ID, right.ID)
	})
	return out
}

func (r *Registry) Stop(id string) error {
	r.mu.Lock()
	entry, present := r.forwards[id]
	if present {
		delete(r.forwards, id)
	}
	r.mu.Unlock()

	if !present {
		return fmt.Errorf("no port forward with id %q", id)
	}
	entry.current.halt()
	return nil
}

func (r *Registry) StopAll() {
	r.mu.Lock()
	r.stopped = true
	entries := make([]*record, 0, len(r.forwards))
	for _, entry := range r.forwards {
		entries = append(entries, entry)
	}
	r.forwards = map[string]*record{}
	pending := make([]chan struct{}, 0, len(r.starting))
	for _, wait := range r.starting {
		pending = append(pending, wait)
	}
	r.starting = map[startKey]chan struct{}{}
	r.mu.Unlock()

	for _, entry := range entries {
		entry.current.halt()
	}
	for _, wait := range pending {
		close(wait)
	}
}
