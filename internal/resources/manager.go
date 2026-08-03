package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubediscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/metrics"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

const (
	chartFetchTimeout  = 30 * time.Second
	defaultSyncTimeout = 30 * time.Second
	eventBuffer        = 256
	attachAttempts     = 3
)

type Event struct {
	Kind string
	Row  api.Row
	UID  string
}

type Subscription struct {
	Columns    []api.Column
	Namespaced bool
	Rows       []api.Row
	Events     <-chan Event
	Resync     <-chan struct{}
	stream     *stream
	cancel     func()
}

func (s *Subscription) Close() {
	s.cancel()
}

func (s *Subscription) Snapshot() []api.Row {
	return s.stream.snapshot()
}

type Manager struct {
	rootCtx     context.Context
	dyn         dynamic.Interface
	cs          kubernetes.Interface
	schemas     *jsonschema.Client
	charts      *charts.Cache
	forwards    *portforward.Registry
	shells      *exec.Service
	debugger    *debugcontainer.Service
	prom        *prom.Client
	disco       kubediscovery.CachedDiscoveryInterface
	catalog     sync.RWMutex
	cats        []api.Category
	descs       map[string]api.ResourceDescriptor
	discErr     string
	mu          sync.Mutex
	streams     map[streamKey]*stream
	building    map[streamKey]*buildGate
	syncTimeout time.Duration
}

type buildGate struct {
	mu   sync.Mutex
	refs int
}

type Deps struct {
	Dynamic     dynamic.Interface
	Clientset   kubernetes.Interface
	Schemas     *jsonschema.Client
	Forwards    *portforward.Registry
	Shells      *exec.Service
	Debugger    *debugcontainer.Service
	Prometheus  *prom.Client
	Categories  []api.Category
	Descriptors map[string]api.ResourceDescriptor
}

func NewManager(ctx context.Context, deps Deps) *Manager {
	return &Manager{
		rootCtx:  ctx,
		dyn:      deps.Dynamic,
		cs:       deps.Clientset,
		schemas:  deps.Schemas,
		charts:   charts.New(ctx, &http.Client{Timeout: chartFetchTimeout}, charts.DefaultTTL),
		forwards: deps.Forwards,
		shells:   deps.Shells,
		debugger: deps.Debugger,
		prom:     deps.Prometheus,
		cats:     deps.Categories,
		descs:    deps.Descriptors,
		streams:  map[streamKey]*stream{},
		building: map[streamKey]*buildGate{},

		syncTimeout: defaultSyncTimeout,
	}
}

func (m *Manager) UseDiscovery(disco kubediscovery.CachedDiscoveryInterface, discErr error) {
	m.disco = disco
	m.setCatalog(m.cats, m.descs, discErr)
}

func (m *Manager) Resources() api.ResourceCatalog {
	m.catalog.RLock()
	defer m.catalog.RUnlock()
	return api.ResourceCatalog{Categories: m.cats, Error: m.discErr}
}

func (m *Manager) RefreshResources() api.ResourceCatalog {
	if m.disco == nil {
		return m.Resources()
	}
	m.disco.Invalidate()
	if m.schemas != nil {
		m.schemas.Refresh()
	}
	cats, descs, err := discovery.List(m.disco)
	m.setCatalog(cats, descs, err)
	return m.Resources()
}

func (m *Manager) setCatalog(cats []api.Category, descs map[string]api.ResourceDescriptor, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	m.catalog.Lock()
	defer m.catalog.Unlock()
	m.cats = cats
	m.descs = descs
	m.discErr = message
}

func (m *Manager) descriptors() map[string]api.ResourceDescriptor {
	m.catalog.RLock()
	defer m.catalog.RUnlock()
	return m.descs
}

func (m *Manager) Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error) {
	return inspect.Get(ctx, m.dyn, ref)
}

func (m *Manager) ApplyObject(ctx context.Context, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error) {
	return inspect.Apply(ctx, m.dyn, ref, m.kindFor(ref), doc)
}

func (m *Manager) kindFor(ref api.ObjectRef) string {
	desc, ok := m.descriptors()[discovery.Key(ref.Group, ref.Version, ref.Resource)]
	if !ok {
		return ""
	}
	return desc.Kind
}

func (m *Manager) DeleteObject(ctx context.Context, ref api.ObjectRef) error {
	return inspect.Delete(ctx, m.dyn, ref)
}

func (m *Manager) Events(ctx context.Context, namespace, uid string) ([]api.Event, error) {
	return inspect.Events(ctx, m.dyn, namespace, uid)
}

func (m *Manager) Logs(ctx context.Context, req logs.Request) (*logs.Stream, error) {
	return logs.Open(ctx, m.cs, req)
}

func (m *Manager) FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error) {
	return flux.Do(ctx, m.dyn, ref, action, time.Now())
}

func (m *Manager) Action(ctx context.Context, req actions.Request) (api.ActionResult, error) {
	return actions.New(m.dyn, m.cs).Do(ctx, req, time.Now())
}

func (m *Manager) StartForward(ctx context.Context, target portforward.Target, port int32) (api.PortForward, error) {
	if m.forwards == nil {
		return api.PortForward{}, errors.New("port forwarding is unavailable")
	}
	return m.forwards.Start(ctx, target, port)
}

func (m *Manager) Forwards() []api.PortForward {
	if m.forwards == nil {
		return []api.PortForward{}
	}
	return m.forwards.List()
}

func (m *Manager) StopForward(id string) error {
	if m.forwards == nil {
		return errors.New("port forwarding is unavailable")
	}
	return m.forwards.Stop(id)
}

func (m *Manager) ExecSupport(ctx context.Context, req exec.Request) (api.ExecSupport, error) {
	if m.shells == nil {
		return api.ExecSupport{}, errors.New("exec is unavailable")
	}
	return m.shells.Support(ctx, req)
}

func (m *Manager) StartExec(ctx context.Context, req exec.Request, stdout io.Writer) (*exec.Session, error) {
	if m.shells == nil {
		return nil, errors.New("exec is unavailable")
	}
	return m.shells.Start(ctx, req, stdout)
}

func (m *Manager) DebugSupport(ctx context.Context, namespace, pod string) api.DebugSupport {
	if m.debugger == nil {
		return api.DebugSupport{Namespace: namespace, Pod: pod, Allowed: false, Reason: debugcontainer.ErrUnavailable.Error()}
	}
	return m.debugger.Allowed(ctx, namespace, pod)
}

func (m *Manager) StartDebug(ctx context.Context, req debugcontainer.Request) (api.DebugSession, error) {
	if m.debugger == nil {
		return api.DebugSession{}, debugcontainer.ErrUnavailable
	}
	return m.debugger.Ensure(ctx, req)
}

func (m *Manager) MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error) {
	if m.prom == nil {
		return api.MetricHistory{}, prom.ErrUnavailable
	}
	return m.prom.PodHistory(ctx, namespace, pod, span, time.Now())
}

func (m *Manager) Schema(gvk jsonschema.GVK) (json.RawMessage, error) {
	if m.schemas == nil {
		return nil, errors.New("schemas unavailable")
	}
	return m.schemas.For(gvk)
}

func (m *Manager) Graph(ctx context.Context) api.Graph {
	return gitops.Build(ctx, m, m.descriptors())
}

func (m *Manager) Flux(ctx context.Context) api.FluxDashboard {
	return flux.Build(ctx, m, m.descriptors(), m.charts)
}

func (m *Manager) Metrics(ctx context.Context) api.Metrics {
	return metrics.Build(ctx, m.dyn)
}

type streamKey struct {
	gvr schema.GroupVersionResource
	ns  string
}

type subscriber struct {
	events chan Event
	resync chan struct{}
}

func newSubscriber() *subscriber {
	return &subscriber{
		events: make(chan Event, eventBuffer),
		resync: make(chan struct{}, 1),
	}
}

type stream struct {
	kind     string
	columns  []api.Column
	informer cache.SharedIndexInformer
	lister   cache.GenericLister
	cancel   context.CancelFunc
	mu       sync.Mutex
	subs     map[*subscriber]struct{}
	refs     int
	pinned   bool
}

func (m *Manager) Subscribe(group, version, resource, namespace string) (*Subscription, error) {
	desc, ok := m.descriptors()[discovery.Key(group, version, resource)]
	if !ok {
		return nil, fmt.Errorf("unknown resource %s/%s/%s", group, version, resource)
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	effNs := namespace
	if !desc.Namespaced {
		effNs = ""
	}
	key := streamKey{gvr: gvr, ns: effNs}

	st, entry, err := m.attach(key, desc)
	if err != nil {
		return nil, err
	}

	return &Subscription{
		Columns:    st.columns,
		Namespaced: desc.Namespaced,
		Rows:       st.snapshot(),
		Events:     entry.events,
		Resync:     entry.resync,
		stream:     st,
		cancel: func() {
			m.detach(key, st, entry)
		},
	}, nil
}

func (m *Manager) attach(key streamKey, desc api.ResourceDescriptor) (*stream, *subscriber, error) {
	for range attachAttempts {
		st, err := m.streamFor(key, desc)
		if err != nil {
			return nil, nil, err
		}
		entry, ok := m.register(key, st)
		if ok {
			return st, entry, nil
		}
	}
	return nil, nil, fmt.Errorf("%s kept being torn down while subscribing", key.gvr.String())
}

func (m *Manager) List(desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	lister, err := m.pinnedLister(desc)
	if err != nil {
		return nil, err
	}
	objs, listErr := lister.List(labels.Everything())
	if listErr != nil {
		return nil, listErr
	}
	out := make([]*unstructured.Unstructured, 0, len(objs))
	for _, obj := range objs {
		item, ok := toUnstructured(obj)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *Manager) pinnedLister(desc api.ResourceDescriptor) (cache.GenericLister, error) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	key := streamKey{gvr: gvr}
	for range attachAttempts {
		st, err := m.streamFor(key, desc)
		if err != nil {
			return nil, err
		}
		if m.pin(key, st) {
			return st.lister, nil
		}
	}
	return nil, fmt.Errorf("%s kept being torn down while reading", gvr.String())
}

func (m *Manager) pin(key streamKey, st *stream) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams[key] != st {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.pinned = true
	return true
}

func (m *Manager) register(key streamKey, st *stream) (*subscriber, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams[key] != st {
		return nil, false
	}
	entry := newSubscriber()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.subs[entry] = struct{}{}
	st.refs++
	return entry, true
}

func (m *Manager) detach(key streamKey, st *stream, entry *subscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st.mu.Lock()
	_, present := st.subs[entry]
	if present {
		delete(st.subs, entry)
		close(entry.events)
		close(entry.resync)
		st.refs--
	}
	idle := st.refs == 0 && !st.pinned
	st.mu.Unlock()
	if !present || !idle {
		return
	}
	if m.streams[key] != st {
		return
	}
	delete(m.streams, key)
	st.cancel()
}

func (m *Manager) streamFor(key streamKey, desc api.ResourceDescriptor) (*stream, error) {
	existing, gate := m.acquireGate(key)
	if existing != nil {
		return existing, nil
	}
	defer m.releaseGate(key)

	gate.Lock()
	defer gate.Unlock()

	built, ok := m.lookupStream(key)
	if ok {
		return built, nil
	}

	created, err := m.newStream(key, desc)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.streams[key] = created
	m.mu.Unlock()
	return created, nil
}

func (m *Manager) lookupStream(key streamKey) (*stream, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.streams[key]
	return st, ok
}

func (m *Manager) acquireGate(key streamKey) (*stream, *sync.Mutex) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.streams[key]
	if ok {
		return st, nil
	}
	gate, ok := m.building[key]
	if !ok {
		gate = &buildGate{}
		m.building[key] = gate
	}
	gate.refs++
	return nil, &gate.mu
}

func (m *Manager) releaseGate(key streamKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gate, ok := m.building[key]
	if !ok {
		return
	}
	gate.refs--
	if gate.refs == 0 {
		delete(m.building, key)
	}
}

func (m *Manager) newStream(key streamKey, desc api.ResourceDescriptor) (*stream, error) {
	ctx, cancel := context.WithCancel(m.rootCtx)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(m.dyn, 0, key.ns, nil)
	gi := factory.ForResource(key.gvr)
	informer := gi.Informer()

	transformErr := informer.SetTransform(stripManagedFields)
	if transformErr != nil {
		cancel()
		return nil, fmt.Errorf("set transform: %w", transformErr)
	}

	var lastWatchErr atomic.Pointer[string]
	watchErr := informer.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
		reason := err.Error()
		lastWatchErr.Store(&reason)
	})
	if watchErr != nil {
		cancel()
		return nil, fmt.Errorf("set watch error handler: %w", watchErr)
	}

	st := &stream{
		kind:     desc.Kind,
		columns:  columnsFor(desc.Kind),
		informer: informer,
		lister:   gi.Lister(),
		cancel:   cancel,
		subs:     map[*subscriber]struct{}{},
	}

	_, handlerErr := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			st.publish("added", obj)
		},
		UpdateFunc: func(_, obj any) {
			st.publish("modified", obj)
		},
		DeleteFunc: func(obj any) {
			st.publishDelete(obj)
		},
	})
	if handlerErr != nil {
		cancel()
		return nil, fmt.Errorf("add event handler: %w", handlerErr)
	}

	factory.Start(ctx.Done())
	syncCtx, cancelSync := context.WithTimeout(ctx, m.syncTimeout)
	defer cancelSync()
	if !cache.WaitForCacheSync(syncCtx.Done(), informer.HasSynced) {
		cancel()
		return nil, syncFailure(key, m.syncTimeout, watchFailure(&lastWatchErr))
	}
	return st, nil
}

func watchFailure(holder *atomic.Pointer[string]) string {
	reason := holder.Load()
	if reason == nil {
		return ""
	}
	return *reason
}

var ErrNotSynced = errors.New("the cluster did not answer in time")

func syncFailure(key streamKey, timeout time.Duration, reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: %s did not sync within %s", ErrNotSynced, key.gvr.String(), timeout)
	}
	return fmt.Errorf("%w: %s did not sync within %s: %s", ErrNotSynced, key.gvr.String(), timeout, reason)
}

func (st *stream) publish(kind string, obj any) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	st.fanout(Event{Kind: kind, Row: toRow(u, st.kind)})
}

func (st *stream) publishDelete(obj any) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	st.fanout(Event{Kind: "deleted", UID: string(u.GetUID())})
}

func (st *stream) fanout(ev Event) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for sub := range st.subs {
		select {
		case sub.events <- ev:
		default:
			signalResync(sub)
		}
	}
}

func signalResync(sub *subscriber) {
	select {
	case sub.resync <- struct{}{}:
	default:
	}
}

func (st *stream) snapshot() []api.Row {
	objs, err := st.lister.List(labels.Everything())
	if err != nil {
		return []api.Row{}
	}
	rows := make([]api.Row, 0, len(objs))
	for _, o := range objs {
		u, ok := toUnstructured(o)
		if !ok {
			continue
		}
		rows = append(rows, toRow(u, st.kind))
	}
	return rows
}

func toUnstructured(obj any) (*unstructured.Unstructured, bool) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u, true
	}
	tomb, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, false
	}
	u, ok := tomb.Obj.(*unstructured.Unstructured)
	if !ok {
		return nil, false
	}
	return u, true
}

func toRow(obj *unstructured.Unstructured, kind string) api.Row {
	return api.Row{
		UID:        string(obj.GetUID()),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		CreatedAt:  obj.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
		Cells:      cellsFor(obj, kind),
		Containers: containersFor(obj, kind),
	}
}

func stripManagedFields(value any) (any, error) {
	obj, ok := value.(*unstructured.Unstructured)
	if !ok {
		return value, nil
	}
	obj.SetManagedFields(nil)
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		obj.SetAnnotations(annotations)
	}
	return obj, nil
}
