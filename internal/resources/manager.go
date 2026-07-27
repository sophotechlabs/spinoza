package resources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/metrics"
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
	rootCtx context.Context
	dyn     dynamic.Interface
	cs      kubernetes.Interface
	cats    []api.Category
	descs   map[string]api.ResourceDescriptor
	mu      sync.Mutex
	streams map[streamKey]*stream
}

func NewManager(ctx context.Context, dyn dynamic.Interface, cs kubernetes.Interface, cats []api.Category, descs map[string]api.ResourceDescriptor) *Manager {
	return &Manager{
		rootCtx: ctx,
		dyn:     dyn,
		cs:      cs,
		cats:    cats,
		descs:   descs,
		streams: map[streamKey]*stream{},
	}
}

func (m *Manager) Resources() []api.Category {
	return m.cats
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

func (m *Manager) Graph(ctx context.Context) api.Graph {
	return gitops.Build(ctx, m.dyn, m.descs)
}

func (m *Manager) Flux(ctx context.Context) api.FluxDashboard {
	return flux.Build(ctx, m.dyn, m.descs)
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
	desc, ok := m.descs[discovery.Key(group, version, resource)]
	if !ok {
		return nil, fmt.Errorf("unknown resource %s/%s/%s", group, version, resource)
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	effNs := namespace
	if !desc.Namespaced {
		effNs = ""
	}
	key := streamKey{gvr: gvr, ns: effNs}

	m.mu.Lock()
	st, ok := m.streams[key]
	if !ok {
		created, err := m.newStream(key, desc)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		st = created
		m.streams[key] = st
	}
	m.mu.Unlock()

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

	st := &stream{
		kind:     desc.Kind,
		columns:  columnsFor(desc.Kind),
		informer: informer,
		lister:   gi.Lister(),
		cancel:   cancel,
		subs:     map[chan Event]struct{}{},
	}

	_, handlerErr := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			st.publish("added", obj)
		},
		UpdateFunc: func(_, obj interface{}) {
			st.publish("modified", obj)
		},
		DeleteFunc: func(obj interface{}) {
			st.publishDelete(obj)
		},
	})
	if handlerErr != nil {
		cancel()
		return nil, fmt.Errorf("add event handler: %w", handlerErr)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		cancel()
		return nil, fmt.Errorf("cache sync failed for %s", key.gvr.String())
	}
	return st, nil
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

func (st *stream) publish(kind string, obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	st.fanout(Event{Kind: kind, Row: toRow(u, st.kind)})
}

func (st *stream) publishDelete(obj interface{}) {
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

func toUnstructured(obj interface{}) (*unstructured.Unstructured, bool) {
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

func toRow(u *unstructured.Unstructured, kind string) api.Row {
	return api.Row{
		UID:        string(u.GetUID()),
		Name:       u.GetName(),
		Namespace:  u.GetNamespace(),
		CreatedAt:  u.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
		Cells:      cellsFor(u, kind),
		Containers: containersFor(u, kind),
	}
}

func stripManagedFields(obj interface{}) (interface{}, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	u.SetManagedFields(nil)
	annotations := u.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		u.SetAnnotations(annotations)
	}
	return u, nil
}
