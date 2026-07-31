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
	cancel     func()
}

func (s *Subscription) Close() {
	s.cancel()
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

func NewManager(ctx context.Context, dyn dynamic.Interface, cs kubernetes.Interface, schemas *jsonschema.Client, forwards *portforward.Registry, shells *exec.Service, debugger *debugcontainer.Service, promClient *prom.Client, cats []api.Category, descs map[string]api.ResourceDescriptor) *Manager {
	return &Manager{
		rootCtx:  ctx,
		dyn:      dyn,
		cs:       cs,
		schemas:  schemas,
		charts:   charts.New(ctx, &http.Client{Timeout: chartFetchTimeout}, charts.DefaultTTL),
		forwards: forwards,
		shells:   shells,
		debugger: debugger,
		prom:     promClient,
		cats:     cats,
		descs:    descs,
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
	return inspect.Apply(ctx, m.dyn, ref, doc)
}

func (m *Manager) DeleteObject(ctx context.Context, ref api.ObjectRef) error {
	return inspect.Delete(ctx, m.dyn, ref)
}

func (m *Manager) Events(ctx context.Context, namespace, uid string) []api.Event {
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

func (m *Manager) DebugSupport(ctx context.Context, namespace string) api.DebugSupport {
	if m.debugger == nil {
		return api.DebugSupport{Namespace: namespace, Allowed: false, Reason: debugcontainer.ErrUnavailable.Error()}
	}
	return m.debugger.Allowed(ctx, namespace)
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
	return gitops.Build(ctx, m.dyn, m.descriptors())
}

func (m *Manager) Flux(ctx context.Context) api.FluxDashboard {
	return flux.Build(ctx, m.dyn, m.descriptors(), m.charts)
}

func (m *Manager) Metrics(ctx context.Context) api.Metrics {
	return metrics.Build(ctx, m.dyn)
}

type streamKey struct {
	gvr schema.GroupVersionResource
	ns  string
}

type stream struct {
	kind     string
	columns  []api.Column
	informer cache.SharedIndexInformer
	lister   cache.GenericLister
	cancel   context.CancelFunc
	mu       sync.Mutex
	subs     map[chan Event]struct{}
	refs     int
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

	st, err := m.streamFor(key, desc)
	if err != nil {
		return nil, err
	}

	ch := make(chan Event, 256)
	st.mu.Lock()
	st.subs[ch] = struct{}{}
	st.refs++
	st.mu.Unlock()

	rows := st.snapshot()

	cancel := func() {
		st.mu.Lock()
		_, present := st.subs[ch]
		if present {
			delete(st.subs, ch)
			close(ch)
			st.refs--
		}
		refs := st.refs
		st.mu.Unlock()
		if present && refs == 0 {
			m.dropStream(key)
		}
	}

	return &Subscription{
		Columns:    st.columns,
		Namespaced: desc.Namespaced,
		Rows:       rows,
		Events:     ch,
		cancel:     cancel,
	}, nil
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
		subs:     map[chan Event]struct{}{},
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

func syncFailure(key streamKey, timeout time.Duration, reason string) error {
	if reason == "" {
		return fmt.Errorf("%s did not sync within %s", key.gvr.String(), timeout)
	}
	return fmt.Errorf("%s did not sync within %s: %s", key.gvr.String(), timeout, reason)
}

func (m *Manager) dropStream(key streamKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.streams[key]
	if !ok {
		return
	}
	st.mu.Lock()
	refs := st.refs
	st.mu.Unlock()
	if refs > 0 {
		return
	}
	delete(m.streams, key)
	st.cancel()
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
	for ch := range st.subs {
		select {
		case ch <- ev:
		default:
		}
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
