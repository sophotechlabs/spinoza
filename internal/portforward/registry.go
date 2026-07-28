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
)

const (
	KindPod     = "Pod"
	KindService = "Service"

	StateRunning = "running"
	StateFailed  = "failed"

	DefaultStartTimeout = 15 * time.Second
	DefaultReapInterval = 10 * time.Second
)

type Target struct {
	Kind      string
	Namespace string
	Name      string
}

type Runner interface {
	Run(ctx context.Context, namespace, pod string, remotePort int32, ready chan<- int32, stop <-chan struct{}) error
}

type Resolver interface {
	Resolve(ctx context.Context, target Target, port int32) (pod string, podPort int32, err error)
}

type Prober interface {
	Alive(ctx context.Context, namespace, pod string) bool
}

type record struct {
	forward api.PortForward
	stop    chan struct{}
	once    sync.Once
}

func (r *record) halt() {
	r.once.Do(func() {
		close(r.stop)
	})
}

type Registry struct {
	root      context.Context
	runner    Runner
	resolver  Resolver
	prober    Prober
	now       func() time.Time
	nextID    func() string
	timeout   time.Duration
	reapEvery time.Duration

	mu       sync.Mutex
	forwards map[string]*record
	sequence int
}

func NewRegistry(root context.Context, runner Runner, resolver Resolver, prober Prober) *Registry {
	return newRegistry(root, runner, resolver, prober, DefaultStartTimeout, DefaultReapInterval)
}

func newRegistry(
	root context.Context,
	runner Runner,
	resolver Resolver,
	prober Prober,
	timeout time.Duration,
	reapEvery time.Duration,
) *Registry {
	registry := &Registry{
		root:      root,
		runner:    runner,
		resolver:  resolver,
		prober:    prober,
		now:       time.Now,
		timeout:   timeout,
		reapEvery: reapEvery,
		forwards:  map[string]*record{},
	}
	registry.nextID = registry.sequentialID
	go registry.stopOnShutdown()
	go registry.reapLoop()
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
	if r.prober == nil {
		return
	}
	for _, forward := range r.List() {
		if forward.State != StateRunning {
			continue
		}
		if r.prober.Alive(r.root, forward.Namespace, forward.Pod) {
			continue
		}
		r.fail(forward.ID, fmt.Sprintf("pod %s/%s is gone", forward.Namespace, forward.Pod))
	}
}

func (r *Registry) fail(id, message string) {
	r.mu.Lock()
	entry, present := r.forwards[id]
	if present {
		entry.forward.State = StateFailed
		entry.forward.Error = message
	}
	r.mu.Unlock()
	if present {
		entry.halt()
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
	existing, found := r.existing(target, port)
	if found {
		return existing, nil
	}

	pod, podPort, err := r.resolver.Resolve(ctx, target, port)
	if err != nil {
		return api.PortForward{}, err
	}

	stop := make(chan struct{})
	ready := make(chan int32, 1)
	failed := make(chan error, 1)
	go func() {
		failed <- r.runner.Run(r.root, target.Namespace, pod, podPort, ready, stop)
	}()

	select {
	case local := <-ready:
		return r.register(target, port, pod, local, stop, failed), nil
	case runErr := <-failed:
		close(stop)
		if runErr == nil {
			runErr = errors.New("the port forward ended before it was ready")
		}
		return api.PortForward{}, runErr
	case <-time.After(r.timeout):
		close(stop)
		return api.PortForward{}, fmt.Errorf("timed out starting a port forward to %s/%s", target.Namespace, target.Name)
	}
}

func (r *Registry) register(target Target, port int32, pod string, local int32, stop chan struct{}, failed <-chan error) api.PortForward {
	r.mu.Lock()
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
	r.forwards[forward.ID] = &record{forward: forward, stop: stop}
	r.mu.Unlock()

	go r.watch(forward.ID, failed)
	return forward
}

func (r *Registry) watch(id string, failed <-chan error) {
	err := <-failed

	r.mu.Lock()
	defer r.mu.Unlock()
	entry, present := r.forwards[id]
	if !present {
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

func (r *Registry) existing(target Target, port int32) (api.PortForward, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	slices.SortFunc(out, func(a, b api.PortForward) int {
		return strings.Compare(a.ID, b.ID)
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
	entry.halt()
	return nil
}

func (r *Registry) StopAll() {
	r.mu.Lock()
	entries := make([]*record, 0, len(r.forwards))
	for _, entry := range r.forwards {
		entries = append(entries, entry)
	}
	r.forwards = map[string]*record{}
	r.mu.Unlock()

	for _, entry := range entries {
		entry.halt()
	}
}
