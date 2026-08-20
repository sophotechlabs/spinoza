package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubediscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/metrics"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/overview"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	chartFetchTimeout  = 30 * time.Second
	defaultSyncTimeout = 30 * time.Second
	eventBuffer        = 256
	attachAttempts     = 3
	warmConcurrency    = 8
	buildBackoff       = 5 * time.Second
	maxBuildBackoff    = 2 * time.Minute
	defaultIdleGrace   = 90 * time.Second
	defaultMetricsTTL  = 5 * time.Second
	defaultCountsTTL   = 10 * time.Second
)

type Event struct {
	Kind    string
	Row     api.Row
	UID     string
	Message string
}

type Subscription struct {
	Columns    []api.Column
	Namespaced bool
	Rows       []api.Row
	Total      int
	Events     <-chan Event
	Resync     <-chan struct{}
	stream     *stream
	entry      *subscriber
	namespace  string
	cancel     func()
}

func (s *Subscription) Close() {
	s.cancel()
}

func (s *Subscription) Limit() int {
	return int(s.entry.limit.Load())
}

func (s *Subscription) SetLimit(limit int) {
	s.entry.limit.Store(int64(limitFor(s.stream.kind, limit)))
	signalResync(s.entry)
}

func (s *Subscription) Snapshot() ([]api.Row, int, error) {
	return s.stream.snapshot(s.namespace, s.Limit())
}

type Manager struct {
	rootCtx     context.Context
	dyn         dynamic.Interface
	meta        metadata.Interface
	cs          kubernetes.Interface
	schemas     *jsonschema.Client
	charts      *charts.Cache
	forwards    *portforward.Registry
	shells      *exec.Service
	debugger    *debugcontainer.Service
	nodeShells  *nodeshell.Service
	helm        *helm.Service
	prom        *prom.Client
	disco       kubediscovery.CachedDiscoveryInterface
	limits      Limits
	catalog     sync.RWMutex
	cats        []api.Category
	descs       map[string]api.ResourceDescriptor
	discErr     string
	now         func() time.Time
	mu          sync.Mutex
	streams     map[streamKey]*stream
	building    map[streamKey]*buildGate
	failures    map[streamKey]buildFailure
	syncTimeout time.Duration
	usage       recent[api.Metrics]
	tally       recent[api.ResourceCounts]
}

type buildGate struct {
	mu   sync.Mutex
	refs int
}

type buildFailure struct {
	err  error
	at   time.Time
	wait time.Duration
}

type Limits struct {
	SyncTimeout     time.Duration
	IdleGrace       time.Duration
	MetricsTTL      time.Duration
	CountsTTL       time.Duration
	WarmConcurrency int
	Counts          CountLimits
	Search          CountLimits
}

func (l Limits) orDefaults() Limits {
	if l.SyncTimeout == 0 {
		l.SyncTimeout = defaultSyncTimeout
	}
	if l.IdleGrace == 0 {
		l.IdleGrace = defaultIdleGrace
	}
	if l.MetricsTTL == 0 {
		l.MetricsTTL = defaultMetricsTTL
	}
	if l.CountsTTL == 0 {
		l.CountsTTL = defaultCountsTTL
	}
	if l.WarmConcurrency == 0 {
		l.WarmConcurrency = warmConcurrency
	}
	l.Counts = l.Counts.orDefaults()
	l.Search = searchLimits(l.Search)
	return l
}

type Deps struct {
	Limits      Limits
	Dynamic     dynamic.Interface
	Metadata    metadata.Interface
	Clientset   kubernetes.Interface
	Schemas     *jsonschema.Client
	Forwards    *portforward.Registry
	Shells      *exec.Service
	Debugger    *debugcontainer.Service
	NodeShells  *nodeshell.Service
	Helm        *helm.Service
	Prometheus  *prom.Client
	Charts      *charts.Cache
	Categories  []api.Category
	Descriptors map[string]api.ResourceDescriptor
}

func NewManager(ctx context.Context, deps Deps) *Manager {
	limits := deps.Limits.orDefaults()
	return &Manager{
		limits:     limits,
		rootCtx:    ctx,
		dyn:        deps.Dynamic,
		meta:       deps.Metadata,
		cs:         deps.Clientset,
		schemas:    deps.Schemas,
		charts:     chartCache(ctx, deps.Charts),
		forwards:   deps.Forwards,
		shells:     deps.Shells,
		debugger:   deps.Debugger,
		nodeShells: deps.NodeShells,
		helm:       deps.Helm,
		prom:       deps.Prometheus,
		cats:       deps.Categories,
		descs:      deps.Descriptors,
		now:        time.Now,
		streams:    map[streamKey]*stream{},
		building:   map[streamKey]*buildGate{},
		failures:   map[streamKey]buildFailure{},

		syncTimeout: limits.SyncTimeout,
	}
}

func chartCache(ctx context.Context, provided *charts.Cache) *charts.Cache {
	if provided != nil {
		return provided
	}
	return charts.New(ctx, &http.Client{Timeout: chartFetchTimeout}, charts.DefaultTTL)
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
	if len(descs) == 0 {
		m.keepCatalog(emptyDiscovery(err))
		return m.Resources()
	}
	m.setCatalog(cats, descs, err)
	m.dropVanished(descs)
	return m.Resources()
}

func emptyDiscovery(err error) error {
	if err != nil {
		return fmt.Errorf("kept the resource types already known: discovery listed none: %w", err)
	}
	return errors.New("kept the resource types already known: discovery listed none")
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

func (m *Manager) keepCatalog(err error) {
	m.catalog.Lock()
	defer m.catalog.Unlock()
	m.discErr = err.Error()
}

func (m *Manager) dropVanished(descs map[string]api.ResourceDescriptor) {
	m.mu.Lock()
	gone := make([]*stream, 0, len(m.streams))
	for key, st := range m.streams {
		_, kept := descs[discovery.Key(key.gvr.Group, key.gvr.Version, key.gvr.Resource)]
		if kept {
			continue
		}
		delete(m.streams, key)
		gone = append(gone, st)
	}
	m.mu.Unlock()
	for _, st := range gone {
		st.shutdown()
	}
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

func (m *Manager) ArgoAction(ctx context.Context, ref api.ObjectRef, action argocd.Action) (api.ArgoActionResult, error) {
	return argocd.Do(ctx, m.dyn, ref, action)
}

func (m *Manager) Action(ctx context.Context, req actions.Request) (api.ActionResult, error) {
	return actions.New(m.dyn, m.cs).Do(ctx, req, time.Now())
}

func (m *Manager) StartForward(ctx context.Context, target portforward.Target, port int32) (api.PortForward, error) {
	if m.forwards == nil {
		return api.PortForward{}, fmt.Errorf("%w: port forwarding is not wired up", api.ErrInternal)
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
		return fmt.Errorf("%w: port forwarding is not wired up", api.ErrInternal)
	}
	return m.forwards.Stop(id)
}

func (m *Manager) ExecSupport(ctx context.Context, req exec.Request) (api.ExecSupport, error) {
	if m.shells == nil {
		return api.ExecSupport{}, fmt.Errorf("%w: exec is not wired up", api.ErrInternal)
	}
	return m.shells.Support(ctx, req)
}

func (m *Manager) StartExec(ctx context.Context, req exec.Request, stdout io.Writer) (*exec.Session, error) {
	if m.shells == nil {
		return nil, fmt.Errorf("%w: exec is not wired up", api.ErrInternal)
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

func (m *Manager) NodeShellSupport(ctx context.Context, node string) api.NodeShellSupport {
	if m.nodeShells == nil {
		return api.NodeShellSupport{Node: node, Reason: "node shells are not wired up"}
	}
	return m.nodeShells.Support(ctx, node)
}

func (m *Manager) StartNodeShell(ctx context.Context, node string) (api.NodeShellSession, error) {
	if m.nodeShells == nil {
		return api.NodeShellSession{}, fmt.Errorf("%w: node shells are not wired up", api.ErrInternal)
	}
	return m.nodeShells.Start(ctx, node)
}

func (m *Manager) RemoveNodeShell(ctx context.Context, pod string) {
	if m.nodeShells == nil {
		return
	}
	m.nodeShells.Remove(ctx, pod)
}

func (m *Manager) MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error) {
	if m.prom == nil {
		return api.MetricHistory{}, prom.ErrUnavailable
	}
	return m.prom.PodHistory(ctx, namespace, pod, span, time.Now())
}

func (m *Manager) Schema(ctx context.Context, gvk jsonschema.GVK) (json.RawMessage, error) {
	if m.schemas == nil {
		return nil, fmt.Errorf("%w: schemas are not wired up", api.ErrInternal)
	}
	return m.schemas.For(ctx, gvk)
}

func (m *Manager) Graph(ctx context.Context) api.Graph {
	return gitops.Build(ctx, m, m.descriptors())
}

func (m *Manager) Flux(ctx context.Context) api.FluxDashboard {
	return flux.Build(ctx, m, m.descriptors(), m.charts)
}

func (m *Manager) FluxOverview(ctx context.Context) api.FluxOverview {
	return flux.Overview(ctx, m.cs, m, m.descriptors(), flux.Cluster{
		Kubernetes: m.serverVersion(),
		Nodes:      m.nodeCount(ctx),
		Usage:      m.Metrics(ctx).Pods,
	})
}

func (m *Manager) serverVersion() string {
	if m.disco == nil {
		return ""
	}
	info, err := m.disco.ServerVersion()
	if err != nil {
		return ""
	}
	return info.GitVersion
}

func (m *Manager) nodeCount(ctx context.Context) int {
	desc, ok := m.descriptors()[discovery.Key("", "v1", "nodes")]
	if !ok {
		return 0
	}
	found, err := m.List(ctx, desc)
	if err != nil {
		return 0
	}
	return len(found)
}

func (m *Manager) Argo(ctx context.Context) api.ArgoDashboard {
	return argocd.Build(ctx, m, m.descriptors())
}

func (m *Manager) Counts(ctx context.Context) api.ResourceCounts {
	return withWatched(m.tallied(ctx), m.failingFromCaches())
}

func (m *Manager) tallied(ctx context.Context) api.ResourceCounts {
	empty := api.ResourceCounts{Counts: map[string]int{}}
	if m.meta == nil {
		return empty
	}
	descs := m.descriptors()
	flat := make([]api.ResourceDescriptor, 0, len(descs))
	for _, desc := range descs {
		flat = append(flat, desc)
	}
	out, ok := shared(ctx, &m.tally, m.now, m.limits.CountsTTL, func(ctx context.Context) (api.ResourceCounts, bool) {
		return Count(ctx, m.meta, flat, m.limits.Counts), true
	})
	if !ok {
		return empty
	}
	return out
}

func withWatched(out api.ResourceCounts, watched map[string]int) api.ResourceCounts {
	if len(watched) == 0 {
		return out
	}
	failing := make(map[string]int, len(out.Failing)+len(watched))
	maps.Copy(failing, out.Failing)
	for key, count := range watched {
		if _, taken := failing[key]; taken {
			continue
		}
		failing[key] = count
	}
	out.Failing = failing
	return out
}

func (m *Manager) Metrics(ctx context.Context) api.Metrics {
	value, ok := shared(ctx, &m.usage, m.now, m.limits.MetricsTTL, func(ctx context.Context) (api.Metrics, bool) {
		built := metrics.Build(ctx, m.dyn, m.nodeSource())
		return built, built.Error == ""
	})
	if !ok {
		return api.Metrics{Error: ctx.Err().Error()}
	}
	return value
}

func (m *Manager) nodeSource() metrics.Nodes {
	desc, ok := m.descriptors()[discovery.Key("", "v1", "nodes")]
	if !ok {
		return metrics.FromCluster(m.dyn)
	}
	return cachedNodes{mgr: m, desc: desc}
}

type cachedNodes struct {
	mgr  *Manager
	desc api.ResourceDescriptor
}

func (c cachedNodes) List(ctx context.Context) ([]*unstructured.Unstructured, error) {
	return c.mgr.List(ctx, c.desc)
}

func (m *Manager) Overview(ctx context.Context) api.ClusterOverview {
	return overview.Build(ctx, m.dyn, m.meta, m, m.versions(), m.descriptors())
}

func (m *Manager) versions() overview.Versions {
	if m.disco == nil {
		return nil
	}
	return m.disco
}

func (m *Manager) HelmReleases(ctx context.Context) (api.HelmReleases, error) {
	if m.helm == nil {
		return api.HelmReleases{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	list, err := m.helm.List(ctx)
	if err != nil {
		return list, err
	}
	decorateOwners(list.Releases, m.fluxOwners(ctx))
	return list, nil
}

func (m *Manager) HelmRelease(ctx context.Context, namespace, name string) (api.HelmReleaseDetail, error) {
	if m.helm == nil {
		return api.HelmReleaseDetail{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	detail, err := m.helm.Detail(ctx, namespace, name, m.resolveKind)
	if err != nil {
		return detail, err
	}
	detail.Release.FluxRef = ownerRef(m.fluxOwners(ctx), namespace, name)
	return detail, nil
}

func (m *Manager) HelmSupport() api.HelmSupport {
	if m.helm == nil {
		return api.HelmSupport{Reason: "helm is not wired up"}
	}
	return m.helm.Support()
}

func (m *Manager) HelmRollback(ctx context.Context, namespace, name string, revision int64) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Rollback(ctx, namespace, name, revision)
}

func (m *Manager) HelmUninstall(ctx context.Context, namespace, name string) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Uninstall(ctx, namespace, name)
}

func (m *Manager) HelmUpgrade(ctx context.Context, req helm.UpgradeRequest) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	owner := ownerRef(m.fluxOwners(ctx), req.Namespace, req.Name)
	if owner != nil {
		return api.HelmActionResult{}, fmt.Errorf(
			"%w: change the helmrelease object %s/%s in git instead",
			helm.ErrFluxManaged, owner.Namespace, owner.Name,
		)
	}
	return m.helm.Upgrade(ctx, req)
}

func (m *Manager) HelmVersions(ctx context.Context, chart string) (api.HelmChartVersions, error) {
	if m.helm == nil {
		return api.HelmChartVersions{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Versions(ctx, chart)
}

func (m *Manager) HelmChartSearch(ctx context.Context, query string) (api.HelmChartSearch, error) {
	if m.helm == nil {
		return api.HelmChartSearch{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.SearchCharts(ctx, query)
}

func (m *Manager) HelmChartValues(ctx context.Context, req helm.ValuesRequest) (api.HelmChartValues, error) {
	if m.helm == nil {
		return api.HelmChartValues{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.ChartValues(ctx, req)
}

func (m *Manager) HelmInstall(ctx context.Context, req helm.InstallRequest) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Install(ctx, req)
}

func (m *Manager) resolveKind(apiVersion, kind string) (helm.Kind, bool) {
	wantGroup, wantVersion := splitAPIVersion(apiVersion)
	for _, desc := range m.descriptors() {
		if desc.Kind != kind {
			continue
		}
		if desc.Group != wantGroup {
			continue
		}
		if wantVersion != "" && desc.Version != wantVersion {
			continue
		}
		return helm.Kind{
			Group:      desc.Group,
			Version:    desc.Version,
			Resource:   desc.Resource,
			Namespaced: desc.Namespaced,
		}, true
	}
	return helm.Kind{}, false
}

func splitAPIVersion(apiVersion string) (group, version string) {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

type streamKey struct {
	gvr schema.GroupVersionResource
}

type subscriber struct {
	events chan Event
	resync chan struct{}
	ns     string
	limit  atomic.Int64
}

func newSubscriber(ns string, limit int) *subscriber {
	sub := &subscriber{
		events: make(chan Event, eventBuffer),
		resync: make(chan struct{}, 1),
		ns:     ns,
	}
	sub.limit.Store(int64(limit))
	return sub
}

func (s *subscriber) wants(ev Event) bool {
	if s.ns == "" {
		return true
	}
	if ev.Kind == "added" || ev.Kind == "modified" {
		return ev.Row.Namespace == s.ns
	}
	return true
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
	broken   bool
	idle     *time.Timer
}

func (m *Manager) Subscribe(
	ctx context.Context,
	group, version, resource, namespace string,
	limit int,
) (*Subscription, error) {
	desc, ok := m.descriptors()[discovery.Key(group, version, resource)]
	if !ok {
		return nil, fmt.Errorf("unknown resource %s/%s/%s", group, version, resource)
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	effNs := namespace
	if !desc.Namespaced {
		effNs = ""
	}
	key := streamKey{gvr: gvr}

	st, entry, err := m.attach(ctx, key, desc, effNs, limitFor(desc.Kind, limit))
	if err != nil {
		return nil, err
	}
	rows, total, snapErr := st.snapshot(effNs, int(entry.limit.Load()))
	if snapErr != nil {
		m.detach(key, st, entry)
		return nil, snapErr
	}

	return &Subscription{
		Columns:    st.columns,
		Namespaced: desc.Namespaced,
		Rows:       rows,
		Total:      total,
		namespace:  effNs,
		Events:     entry.events,
		Resync:     entry.resync,
		stream:     st,
		entry:      entry,
		cancel: func() {
			m.detach(key, st, entry)
		},
	}, nil
}

func (m *Manager) attach(
	ctx context.Context,
	key streamKey,
	desc api.ResourceDescriptor,
	ns string,
	limit int,
) (*stream, *subscriber, error) {
	for range attachAttempts {
		st, err := m.streamFor(ctx, key, desc)
		if err != nil {
			return nil, nil, err
		}
		entry, ok := m.register(key, st, ns, limit)
		if ok {
			return st, entry, nil
		}
	}
	return nil, nil, fmt.Errorf("%w: %s kept being torn down while subscribing", api.ErrInternal, key.gvr.String())
}

func (m *Manager) List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	lister, err := m.pinnedLister(ctx, desc)
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

func (m *Manager) Warm(ctx context.Context, descs []api.ResourceDescriptor) {
	var group sync.WaitGroup
	slots := make(chan struct{}, m.limits.WarmConcurrency)
	for _, desc := range descs {
		group.Add(1)
		go safe.Run("warming "+desc.Kind, func() {
			defer group.Done()
			slots <- struct{}{}
			defer func() {
				<-slots
			}()
			_, _ = m.pinnedLister(ctx, desc)
		})
	}
	group.Wait()
}

func (m *Manager) pinnedLister(ctx context.Context, desc api.ResourceDescriptor) (cache.GenericLister, error) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	key := streamKey{gvr: gvr}
	for range attachAttempts {
		st, err := m.streamFor(ctx, key, desc)
		if err != nil {
			return nil, err
		}
		if m.pin(key, st) {
			return st.lister, nil
		}
	}
	return nil, fmt.Errorf("%w: %s kept being torn down while reading", api.ErrInternal, gvr.String())
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

func (m *Manager) Unpin(descs []api.ResourceDescriptor) {
	for _, desc := range descs {
		m.unpin(streamKey{gvr: schema.GroupVersionResource{
			Group:    desc.Group,
			Version:  desc.Version,
			Resource: desc.Resource,
		}})
	}
}

func (m *Manager) unpin(key streamKey) {
	m.mu.Lock()
	st, present := m.streams[key]
	if !present {
		m.mu.Unlock()
		return
	}
	st.mu.Lock()
	st.pinned = false
	idle := st.refs == 0
	st.mu.Unlock()
	if idle {
		delete(m.streams, key)
	}
	m.mu.Unlock()
	if idle {
		st.cancel()
	}
}

func (m *Manager) register(key streamKey, st *stream, ns string, limit int) (*subscriber, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams[key] != st {
		return nil, false
	}
	entry := newSubscriber(ns, limit)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.idle != nil {
		st.idle.Stop()
		st.idle = nil
	}
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
	idle := st.refs == 0 && !st.pinned && st.idle == nil
	if idle {
		st.idle = time.AfterFunc(m.limits.IdleGrace, func() {
			m.retire(key, st)
		})
	}
	st.mu.Unlock()
}

func (m *Manager) retire(key streamKey, st *stream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st.mu.Lock()
	st.idle = nil
	busy := st.refs > 0 || st.pinned
	st.mu.Unlock()
	if busy {
		return
	}
	if m.streams[key] != st {
		return
	}
	delete(m.streams, key)
	st.cancel()
}

func (m *Manager) streamFor(ctx context.Context, key streamKey, desc api.ResourceDescriptor) (*stream, error) {
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
	cooling, recent := m.coolingOff(key)
	if cooling {
		return nil, recent
	}

	created, err := m.newStream(ctx, key, desc)
	if err != nil {
		if ctx.Err() == nil {
			m.recordFailure(key, err)
		}
		return nil, err
	}

	m.mu.Lock()
	delete(m.failures, key)
	m.streams[key] = created
	m.mu.Unlock()
	return created, nil
}

func (m *Manager) coolingOff(key streamKey) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	failure, known := m.failures[key]
	if !known {
		return false, nil
	}
	if m.now().Sub(failure.at) >= failure.wait {
		return false, nil
	}
	return true, failure.err
}

func (m *Manager) recordFailure(key streamKey, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.failures[key]
	wait := max(previous.wait*2, buildBackoff)
	wait = min(wait, maxBuildBackoff)
	m.failures[key] = buildFailure{err: err, at: m.now(), wait: wait}
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

func (m *Manager) newStream(ctx context.Context, key streamKey, desc api.ResourceDescriptor) (*stream, error) {
	streamCtx, cancel := context.WithCancel(m.rootCtx)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(m.dyn, 0, metav1.NamespaceAll, nil)
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
		subs:     map[*subscriber]struct{}{},
	}

	var lastWatchErr atomic.Pointer[string]
	denied := make(chan error, 1)
	watchErr := informer.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
		reason := err.Error()
		lastWatchErr.Store(&reason)
		if apierrors.IsForbidden(err) {
			select {
			case denied <- err:
			default:
			}
		}
		st.watchBroke(reason)
	})
	if watchErr != nil {
		cancel()
		return nil, fmt.Errorf("set watch error handler: %w", watchErr)
	}

	_, handlerErr := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			safe.Run("an added "+desc.Kind, func() { st.publish("added", obj) })
		},
		UpdateFunc: func(_, obj any) {
			safe.Run("a changed "+desc.Kind, func() { st.publish("modified", obj) })
		},
		DeleteFunc: func(obj any) {
			safe.Run("a deleted "+desc.Kind, func() { st.publishDelete(obj) })
		},
	})
	if handlerErr != nil {
		cancel()
		return nil, fmt.Errorf("add event handler: %w", handlerErr)
	}

	factory.Start(streamCtx.Done())
	syncCtx, cancelSync := context.WithTimeout(ctx, m.syncTimeout)
	defer cancelSync()
	synced := make(chan bool, 1)
	go func() {
		synced <- cache.WaitForCacheSync(syncCtx.Done(), informer.HasSynced)
	}()
	select {
	case ok := <-synced:
		if ok {
			return st, nil
		}
		cancel()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s was still syncing when the request went away: %w", key.gvr.String(), ctx.Err())
		}
		return nil, syncFailure(key, m.syncTimeout, watchFailure(&lastWatchErr))
	case err := <-denied:
		cancel()
		return nil, err
	}
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
	st.watchRecovered()
	st.fanout(Event{Kind: kind, Row: toRow(u, st.kind)})
}

func (st *stream) publishDelete(obj any) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	st.watchRecovered()
	st.fanout(Event{Kind: "deleted", UID: string(u.GetUID())})
}

func (st *stream) fanout(ev Event) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for sub := range st.subs {
		if !sub.wants(ev) {
			continue
		}
		if sub.limit.Load() > 0 {
			signalResync(sub)
			continue
		}
		select {
		case sub.events <- ev:
		default:
			signalResync(sub)
		}
	}
}

func (st *stream) watchBroke(reason string) {
	st.mu.Lock()
	already := st.broken
	st.broken = true
	st.mu.Unlock()
	if already {
		return
	}
	st.fanout(Event{Kind: "error", Message: "the watch on " + st.kind + " broke: " + reason})
}

func (st *stream) watchRecovered() {
	st.mu.Lock()
	broken := st.broken
	st.broken = false
	subs := make([]*subscriber, 0, len(st.subs))
	for sub := range st.subs {
		subs = append(subs, sub)
	}
	st.mu.Unlock()
	if !broken {
		return
	}
	for _, sub := range subs {
		signalResync(sub)
	}
}

func (st *stream) shutdown() {
	st.mu.Lock()
	for sub := range st.subs {
		close(sub.events)
		close(sub.resync)
	}
	st.subs = map[*subscriber]struct{}{}
	st.refs = 0
	st.pinned = false
	st.mu.Unlock()
	st.cancel()
}

func signalResync(sub *subscriber) {
	select {
	case sub.resync <- struct{}{}:
	default:
	}
}

func (st *stream) snapshot(ns string, limit int) ([]api.Row, int, error) {
	objs, err := st.listFor(ns)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the cached %s: %w", st.kind, err)
	}
	held := make([]*unstructured.Unstructured, 0, len(objs))
	for _, o := range objs {
		u, ok := toUnstructured(o)
		if !ok {
			continue
		}
		held = append(held, u)
	}
	total := len(held)
	held = newestFirst(st.kind, held, limit)
	rows := make([]api.Row, 0, len(held))
	for _, u := range held {
		rows = append(rows, toRow(u, st.kind))
	}
	return rows, total, nil
}

func newestFirst(kind string, held []*unstructured.Unstructured, limit int) []*unstructured.Unstructured {
	if limit <= 0 {
		return held
	}
	slices.SortStableFunc(held, func(left, right *unstructured.Unstructured) int {
		return strings.Compare(sortKey(kind, right), sortKey(kind, left))
	})
	if len(held) <= limit {
		return held
	}
	return held[:limit]
}

func sortKey(kind string, obj *unstructured.Unstructured) string {
	if kind == eventKind {
		seen := eventLastSeen(obj)
		if seen != "" {
			return seen
		}
	}
	return obj.GetCreationTimestamp().UTC().Format(time.RFC3339)
}

func limitFor(kind string, asked int) int {
	if asked > 0 {
		return asked
	}
	if kind == eventKind {
		return defaultEventWindow
	}
	return 0
}

func (st *stream) listFor(ns string) ([]runtime.Object, error) {
	if ns == "" {
		return st.lister.List(labels.Everything())
	}
	return st.lister.ByNamespace(ns).List(labels.Everything())
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
