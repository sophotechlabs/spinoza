package resources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/charts"

	kubediscovery "k8s.io/client-go/discovery"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

var depGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

var nodeGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

var eventGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		depGVR:   "DeploymentList",
		nodeGVR:  "NodeList",
		eventGVR: "EventList",
	}
}

func testDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): {
			Group:      "apps",
			Version:    "v1",
			Resource:   "deployments",
			Kind:       "Deployment",
			Namespaced: true,
			Category:   "Workloads",
		},
		discovery.Key("", "v1", "nodes"): {
			Group:      "",
			Version:    "v1",
			Resource:   "nodes",
			Kind:       "Node",
			Namespaced: false,
			Category:   "Cluster",
		},
	}
}

func newDeployment(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"uid":       "uid-" + name,
		},
		"spec": map[string]any{"replicas": int64(2)},
		"status": map[string]any{
			"readyReplicas":     int64(2),
			"updatedReplicas":   int64(2),
			"availableReplicas": int64(2),
		},
	}}
}

func newNode(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]any{
			"name": name,
			"uid":  "uid-" + name,
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
			"nodeInfo": map[string]any{"kubeletVersion": "v1.31.0"},
		},
	}}
}

func newClient(t *testing.T, objs ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	return fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds(), objs...)
}

func newManager(t *testing.T, dyn dynamic.Interface) (*Manager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(ctx, Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Categories: []api.Category{{Name: "Workloads"}}, Descriptors: testDescs()})
	return mgr, cancel
}

func recvEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func expectNoEvent(t *testing.T, ch <-chan Event) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func streamCount(m *Manager) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.streams)
}

func TestManagerGraph(t *testing.T) {
	gitRepoGVR := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{gitRepoGVR: "GitRepositoryList"}
	repo := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata": map[string]any{
			"name":      "app-repo",
			"namespace": "flux-system",
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, repo)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group:      "source.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "gitrepositories",
			Kind:       "GitRepository",
			Namespaced: true,
			Category:   "Custom Resources",
		},
	}
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})

	graph := mgr.Graph(ctx)
	if len(graph.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(graph.Nodes))
	}
	node := graph.Nodes[0]
	if node.ID != "source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo" {
		t.Fatalf("id = %q, want the GitRepository node", node.ID)
	}
	if node.Category != "source" {
		t.Fatalf("category = %q, want source", node.Category)
	}
	if node.Status != "Ready" {
		t.Fatalf("status = %q, want Ready", node.Status)
	}
}

func TestManagerFlux(t *testing.T) {
	gitRepoGVR := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{gitRepoGVR: "GitRepositoryList"}
	repo := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata": map[string]any{
			"name":      "app-repo",
			"namespace": "flux-system",
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, repo)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group:      "source.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "gitrepositories",
			Kind:       "GitRepository",
			Namespaced: true,
			Category:   "Custom Resources",
		},
	}
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})

	dash := mgr.Flux(ctx)
	if len(dash.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(dash.Groups))
	}
	if dash.Groups[0].Name != "Sources" {
		t.Fatalf("group = %q, want Sources", dash.Groups[0].Name)
	}
	if dash.Groups[0].Ready != 1 {
		t.Fatalf("ready = %d, want 1", dash.Groups[0].Ready)
	}
}

func TestManagerMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		{Group: "", Version: "v1", Resource: "nodes"}:                    "NodeList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds)
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset()})

	m := mgr.Metrics(ctx)
	if len(m.Pods) != 0 {
		t.Fatalf("pods = %d, want 0", len(m.Pods))
	}
	if len(m.Nodes) != 0 {
		t.Fatalf("nodes = %d, want 0", len(m.Nodes))
	}
}

func TestResources(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	catalog := mgr.Resources()
	if len(catalog.Categories) != 1 {
		t.Fatalf("categories = %d, want 1", len(catalog.Categories))
	}
	if catalog.Categories[0].Name != "Workloads" {
		t.Fatalf("category = %q, want Workloads", catalog.Categories[0].Name)
	}
	if catalog.Error != "" {
		t.Fatalf("error = %q, want empty", catalog.Error)
	}
}

func TestSubscribeSnapshot(t *testing.T) {
	dyn := newClient(t, newDeployment("default", "web"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if !sub.Namespaced {
		t.Fatal("Namespaced = false, want true")
	}
	if len(sub.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(sub.Columns))
	}
	if sub.Columns[0].Name != "Ready" {
		t.Fatalf("first column = %q, want Ready", sub.Columns[0].Name)
	}
	if len(sub.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sub.Rows))
	}
	row := sub.Rows[0]
	if row.Name != "web" {
		t.Fatalf("name = %q, want web", row.Name)
	}
	if row.UID != "uid-web" {
		t.Fatalf("uid = %q, want uid-web", row.UID)
	}
	if row.Namespace != "default" {
		t.Fatalf("namespace = %q, want default", row.Namespace)
	}
	if len(row.Cells) != 3 || row.Cells[0] != "2/2" {
		t.Fatalf("cells = %v, want [2/2 2 2]", row.Cells)
	}
}

func TestSubscribeUnknownResource(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	_, err := mgr.Subscribe(context.Background(), "apps", "v1", "statefulsets", "default")
	if err == nil {
		t.Fatal("Subscribe returned nil error for unknown resource")
	}
}

func TestSubscribeCacheSyncFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := NewManager(ctx, Deps{Dynamic: newClient(t), Clientset: k8sfake.NewClientset(), Descriptors: testDescs()})
	_, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err == nil {
		t.Fatal("Subscribe returned nil error when cache sync could not complete")
	}
}

func TestSubscribeDeliversEvents(t *testing.T) {
	ctx := context.Background()
	dyn := newClient(t)
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	if len(sub.Rows) != 0 {
		t.Fatalf("initial rows = %d, want 0", len(sub.Rows))
	}

	_, err = dyn.Resource(depGVR).Namespace("default").Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	added := recvEvent(t, sub.Events)
	if added.Kind != "added" {
		t.Fatalf("kind = %q, want added", added.Kind)
	}
	if added.Row.Name != "api" {
		t.Fatalf("name = %q, want api", added.Row.Name)
	}

	cur, err := dyn.Resource(depGVR).Namespace("default").Get(ctx, "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	setErr := unstructured.SetNestedField(cur.Object, int64(5), "spec", "replicas")
	if setErr != nil {
		t.Fatalf("set replicas: %v", setErr)
	}
	_, err = dyn.Resource(depGVR).Namespace("default").Update(ctx, cur, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	modified := recvEvent(t, sub.Events)
	if modified.Kind != "modified" {
		t.Fatalf("kind = %q, want modified", modified.Kind)
	}
	if modified.Row.Cells[0] != "2/5" {
		t.Fatalf("cells[0] = %q, want 2/5", modified.Row.Cells[0])
	}

	err = dyn.Resource(depGVR).Namespace("default").Delete(ctx, "api", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	deleted := recvEvent(t, sub.Events)
	if deleted.Kind != "deleted" {
		t.Fatalf("kind = %q, want deleted", deleted.Kind)
	}
	if deleted.UID != "uid-api" {
		t.Fatalf("uid = %q, want uid-api", deleted.UID)
	}
}

func TestSubscribeSharedStream(t *testing.T) {
	ctx := context.Background()
	dyn := newClient(t)
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub1, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe 1: %v", err)
	}
	sub2, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	if streamCount(mgr) != 1 {
		t.Fatalf("streams = %d, want 1 (shared)", streamCount(mgr))
	}

	_, err = dyn.Resource(depGVR).Namespace("default").Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ev1 := recvEvent(t, sub1.Events)
	ev2 := recvEvent(t, sub2.Events)
	if ev1.Kind != "added" || ev2.Kind != "added" {
		t.Fatalf("both subscribers should receive added, got %q and %q", ev1.Kind, ev2.Kind)
	}

	sub1.Close()
	if streamCount(mgr) != 1 {
		t.Fatalf("streams after first close = %d, want 1", streamCount(mgr))
	}

	_, err = dyn.Resource(depGVR).Namespace("default").Create(ctx, newDeployment("default", "api2"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	ev := recvEvent(t, sub2.Events)
	if ev.Kind != "added" {
		t.Fatalf("kind = %q, want added", ev.Kind)
	}

	sub2.Close()
	if streamCount(mgr) != 0 {
		t.Fatalf("streams after second close = %d, want 0", streamCount(mgr))
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	dyn := newClient(t)
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	sub.Close()
	sub.Close()
	if streamCount(mgr) != 0 {
		t.Fatalf("streams = %d, want 0", streamCount(mgr))
	}
}

func TestSubscribeClusterScoped(t *testing.T) {
	ctx := context.Background()
	dyn := newClient(t, newNode("node-a"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "", "v1", "nodes", "ignored-namespace")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if sub.Namespaced {
		t.Fatal("Namespaced = true, want false")
	}
	if len(sub.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sub.Rows))
	}
	if sub.Rows[0].Cells[0] != "Ready" {
		t.Fatalf("node status = %q, want Ready", sub.Rows[0].Cells[0])
	}

	_, err = dyn.Resource(nodeGVR).Create(ctx, newNode("node-b"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	ev := recvEvent(t, sub.Events)
	if ev.Kind != "added" {
		t.Fatalf("kind = %q, want added", ev.Kind)
	}
	if ev.Row.Name != "node-b" {
		t.Fatalf("name = %q, want node-b", ev.Row.Name)
	}
}

func TestSubscribeNamespaceIsolation(t *testing.T) {
	ctx := context.Background()
	dyn := newClient(t)
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "team-a")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	_, err = dyn.Resource(depGVR).Namespace("team-b").Create(ctx, newDeployment("team-b", "other"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	expectNoEvent(t, sub.Events)

	_, err = dyn.Resource(depGVR).Namespace("team-a").Create(ctx, newDeployment("team-a", "mine"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ev := recvEvent(t, sub.Events)
	if ev.Row.Name != "mine" {
		t.Fatalf("name = %q, want mine", ev.Row.Name)
	}
}

func TestStripManagedFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
	}}
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "kubectl"}})
	obj.SetAnnotations(map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "{}",
		"keep": "yes",
	})

	out, err := stripManagedFields(obj)
	if err != nil {
		t.Fatalf("stripManagedFields: %v", err)
	}
	result, ok := out.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("out type = %T, want *unstructured.Unstructured", out)
	}
	if result.GetManagedFields() != nil {
		t.Fatal("managed fields not cleared")
	}
	annotations := result.GetAnnotations()
	if _, present := annotations["kubectl.kubernetes.io/last-applied-configuration"]; present {
		t.Fatal("last-applied-configuration annotation not removed")
	}
	if annotations["keep"] != "yes" {
		t.Fatal("keep annotation removed")
	}
}

func TestStripManagedFieldsNoAnnotations(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
	}}
	out, err := stripManagedFields(obj)
	if err != nil {
		t.Fatalf("stripManagedFields: %v", err)
	}
	if out != obj {
		t.Fatal("expected same object back")
	}
}

func TestStripManagedFieldsNonUnstructured(t *testing.T) {
	out, err := stripManagedFields("not-an-object")
	if err != nil {
		t.Fatalf("stripManagedFields: %v", err)
	}
	if out != "not-an-object" {
		t.Fatalf("out = %v, want the input unchanged", out)
	}
}

func TestToUnstructured(t *testing.T) {
	obj := newDeployment("default", "web")
	got, ok := toUnstructured(obj)
	if !ok {
		t.Fatal("toUnstructured(*Unstructured) ok = false, want true")
	}
	if got != obj {
		t.Fatal("toUnstructured returned a different object")
	}
}

func TestToUnstructuredTombstone(t *testing.T) {
	obj := newDeployment("default", "web")
	tomb := cache.DeletedFinalStateUnknown{Key: "default/web", Obj: obj}
	got, ok := toUnstructured(tomb)
	if !ok {
		t.Fatal("toUnstructured(tombstone) ok = false, want true")
	}
	if got != obj {
		t.Fatal("toUnstructured returned a different object")
	}
}

func TestToUnstructuredTombstoneWrongInner(t *testing.T) {
	tomb := cache.DeletedFinalStateUnknown{Key: "default/web", Obj: "not-an-object"}
	_, ok := toUnstructured(tomb)
	if ok {
		t.Fatal("toUnstructured(bad tombstone) ok = true, want false")
	}
}

func TestToUnstructuredUnknownType(t *testing.T) {
	_, ok := toUnstructured(42)
	if ok {
		t.Fatal("toUnstructured(int) ok = true, want false")
	}
}

type discoveryResult struct {
	lists []*metav1.APIResourceList
	err   error
}

type stubDiscovery struct {
	kubediscovery.CachedDiscoveryInterface

	invalidated int
	results     []discoveryResult
	calls       int
}

func (s *stubDiscovery) Invalidate() {
	s.invalidated++
}

func (s *stubDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	if len(s.results) == 0 {
		return nil, nil
	}
	index := s.calls
	s.calls++
	if index >= len(s.results) {
		index = len(s.results) - 1
	}
	return s.results[index].lists, s.results[index].err
}

func podList() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list", "watch"}},
			},
		},
	}
}

func TestResourcesReportsTheDiscoveryError(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	mgr.UseDiscovery(&stubDiscovery{}, errors.New("connection refused"))

	catalog := mgr.Resources()
	if catalog.Error != "connection refused" {
		t.Fatalf("error = %q", catalog.Error)
	}
}

func TestRefreshResourcesRediscovers(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	disco := &stubDiscovery{results: []discoveryResult{{lists: podList()}}}
	mgr.UseDiscovery(disco, errors.New("connection refused"))

	if mgr.Resources().Error == "" {
		t.Fatal("expected the startup error to be reported")
	}

	catalog := mgr.RefreshResources()
	if disco.invalidated != 1 {
		t.Fatalf("invalidated %d times, want 1", disco.invalidated)
	}
	if catalog.Error != "" {
		t.Fatalf("error = %q, want it cleared", catalog.Error)
	}
	if len(catalog.Categories) == 0 {
		t.Fatal("expected categories after a successful refresh")
	}
}

func TestRefreshResourcesKeepsReportingAFailure(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	disco := &stubDiscovery{results: []discoveryResult{{err: errors.New("still down")}}}
	mgr.UseDiscovery(disco, errors.New("still down"))

	catalog := mgr.RefreshResources()
	if !strings.Contains(catalog.Error, "still down") {
		t.Fatalf("error = %q", catalog.Error)
	}
}

func TestRefreshKeepsAGoodCatalogThroughABlip(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	before := mgr.Resources()
	disco := &stubDiscovery{results: []discoveryResult{{err: errors.New("connection refused")}}}
	mgr.UseDiscovery(disco, nil)

	catalog := mgr.RefreshResources()

	if len(catalog.Categories) != len(before.Categories) {
		t.Fatalf("categories = %d, want the %d already known kept", len(catalog.Categories), len(before.Categories))
	}
	if !strings.Contains(catalog.Error, "connection refused") {
		t.Fatalf("error = %q, want the blip reported alongside the kept catalog", catalog.Error)
	}
	if _, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default"); err != nil {
		t.Fatalf("subscribe: %v, want the kept catalog still usable", err)
	}
}

func TestRefreshResourcesWithoutADiscoveryClient(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	catalog := mgr.RefreshResources()
	if len(catalog.Categories) != 1 {
		t.Fatalf("categories = %d, want the startup catalog", len(catalog.Categories))
	}
}

func stuckManager(t *testing.T, stuck string) (*Manager, *fake.FakeDynamicClient) {
	t.Helper()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds())
	client.PrependReactor("list", stuck, func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: stuck},
			"",
			errors.New(`User "spinoza" cannot list resource "`+stuck+`"`),
		)
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := NewManager(ctx, Deps{Dynamic: client, Clientset: k8sfake.NewClientset(), Descriptors: testDescs()})
	mgr.syncTimeout = 300 * time.Millisecond
	return mgr, client
}

func TestSubscribeGivesUpWhenTheCacheNeverSyncs(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")

	start := time.Now()
	_, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a resource that never syncs to fail the subscribe")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("subscribe took %s, want it bounded by the sync timeout", elapsed)
	}
	if !strings.Contains(err.Error(), "did not sync") {
		t.Fatalf("err = %v", err)
	}
}

func TestSubscribeReportsWhyTheWatchFailed(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")

	_, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")

	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot list resource") {
		t.Fatalf("err = %v, want the forbidden reason from the reflector", err)
	}
}

func TestASecondSubscribeWaitsOutTheFailureInsteadOfRelisting(t *testing.T) {
	mgr, client := stuckManager(t, "deployments")
	lists := 0
	client.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		lists++
		return false, nil, nil
	})

	first, firstErr := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if firstErr == nil {
		first.Close()
		t.Fatal("expected the stuck resource to fail")
	}
	after := lists
	_, secondErr := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")

	if secondErr == nil {
		t.Fatal("expected the cached failure to be handed back")
	}
	if !strings.Contains(secondErr.Error(), "did not sync") {
		t.Fatalf("err = %v, want the leader's failure", secondErr)
	}
	if lists != after {
		t.Fatalf("lists = %d, want no fresh LIST inside the backoff window (was %d)", lists, after)
	}
}

func TestTheBackoffLetsARecoveredResourceThrough(t *testing.T) {
	mgr, client := stuckManager(t, "deployments")
	if _, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default"); err == nil {
		t.Fatal("expected the stuck resource to fail")
	}

	client.ReactionChain = client.ReactionChain[1:]
	mgr.now = func() time.Time {
		return time.Now().Add(2 * buildBackoff)
	}

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v, want the retry allowed once the backoff elapsed", err)
	}
	sub.Close()
}

func TestABackoffGrowsWithRepeatedFailures(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	key := streamKey{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, ns: "default"}

	mgr.recordFailure(key, errors.New("first"))
	first := mgr.failures[key].wait
	mgr.recordFailure(key, errors.New("second"))
	second := mgr.failures[key].wait

	if first != buildBackoff {
		t.Fatalf("first wait = %s, want %s", first, buildBackoff)
	}
	if second != 2*buildBackoff {
		t.Fatalf("second wait = %s, want it doubled", second)
	}
}

func TestAWatchThatBreaksAfterSyncReachesTheSubscriber(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	st, ok := mgr.lookupStream(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		ns:  "default",
	})
	if !ok {
		t.Fatal("no stream was registered")
	}

	st.watchBroke("etcdserver: request timed out")

	ev := recvEvent(t, sub.Events)
	if ev.Kind != "error" {
		t.Fatalf("kind = %q, want the broken watch reported rather than a frozen table", ev.Kind)
	}
	if !strings.Contains(ev.Message, "request timed out") {
		t.Fatalf("message = %q", ev.Message)
	}
}

func TestABrokenWatchIsReportedOnlyOnce(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	st, _ := mgr.lookupStream(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		ns:  "default",
	})

	st.watchBroke("etcdserver: request timed out")
	recvEvent(t, sub.Events)
	st.watchBroke("etcdserver: request timed out")

	expectNoEvent(t, sub.Events)
}

func TestAWatchThatComesBackAsksForAFreshSnapshot(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	st, _ := mgr.lookupStream(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		ns:  "default",
	})

	st.watchBroke("etcdserver: request timed out")
	recvEvent(t, sub.Events)
	st.publish("modified", newDeployment("default", "web"))

	select {
	case <-sub.Resync:
	case <-time.After(3 * time.Second):
		t.Fatal("a watch that recovered never told the client to re-read the table")
	}
}

func TestARefreshStopsAStreamWhoseResourceTypeIsGone(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	disco := &stubDiscovery{results: []discoveryResult{{lists: podList()}}}
	mgr.UseDiscovery(disco, nil)

	mgr.RefreshResources()

	select {
	case _, open := <-sub.Events:
		if open {
			t.Fatal("the subscriber kept receiving from a resource type that no longer exists")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a stream whose resource type vanished was left list-watching forever")
	}
	if _, present := mgr.lookupStream(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		ns:  "default",
	}); present {
		t.Fatal("the vanished stream is still registered")
	}
}

func TestUnpinLetsAWarmedInformerGo(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	mgr.Warm(context.Background(), []api.ResourceDescriptor{desc})
	if streamCount(mgr) != 1 {
		t.Fatalf("streams = %d, want the warmed one", streamCount(mgr))
	}

	mgr.Unpin([]api.ResourceDescriptor{desc})

	if streamCount(mgr) != 0 {
		t.Fatalf("streams = %d, want the informer released once nothing needs it", streamCount(mgr))
	}
}

func TestUnpinKeepsAnInformerASubscriberStillNeeds(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]
	mgr.Warm(context.Background(), []api.ResourceDescriptor{desc})
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	mgr.Unpin([]api.ResourceDescriptor{desc})

	if streamCount(mgr) != 1 {
		t.Fatalf("streams = %d, want the subscriber's informer kept", streamCount(mgr))
	}
}

func TestUnpinIgnoresAResourceItNeverWarmed(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	mgr.Unpin([]api.ResourceDescriptor{testDescs()[discovery.Key("apps", "v1", "deployments")]})

	if streamCount(mgr) != 0 {
		t.Fatalf("streams = %d", streamCount(mgr))
	}
}

func TestACancelledRequestStopsWaitingForTheCache(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	mgr.syncTimeout = 30 * time.Second
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := mgr.Subscribe(ctx, "apps", "v1", "deployments", "default")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the abandoned subscribe to fail")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("subscribe took %s, want it to stop when the request went away", elapsed)
	}
	cooling, _ := mgr.coolingOff(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		ns:  "default",
	})
	if cooling {
		t.Fatal("a caller giving up put the resource into backoff for everyone else")
	}
}

func TestAStuckSubscribeDoesNotBlockOtherResources(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	mgr.syncTimeout = 20 * time.Second

	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	}()
	<-started
	time.Sleep(200 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		sub, err := mgr.Subscribe(context.Background(), "", "v1", "nodes", "")
		if sub != nil {
			sub.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("subscribe to a healthy resource: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a stuck resource blocked every other subscribe")
	}
}

func TestCloseIsNotBlockedByAStuckSubscribe(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	mgr.syncTimeout = 20 * time.Second

	sub, err := mgr.Subscribe(context.Background(), "", "v1", "nodes", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	}()
	<-started
	time.Sleep(200 * time.Millisecond)

	closed := make(chan struct{})
	go func() {
		sub.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked behind a stuck subscribe; the session would leak its goroutines")
	}
}

func TestConcurrentSubscribersShareOneStream(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)

	var group sync.WaitGroup
	subs := make([]*Subscription, 8)
	for i := range subs {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
			if err != nil {
				return
			}
			subs[index] = sub
		}(i)
	}
	group.Wait()

	mgr.mu.Lock()
	count := len(mgr.streams)
	mgr.mu.Unlock()
	if count != 1 {
		t.Fatalf("built %d streams, want them shared", count)
	}
	for _, sub := range subs {
		if sub == nil {
			t.Fatal("a concurrent subscribe failed")
		}
		sub.Close()
	}
}

func TestTheBuildGateIsReleased(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	sub.Close()

	mgr.mu.Lock()
	gates := len(mgr.building)
	mgr.mu.Unlock()
	if gates != 0 {
		t.Fatalf("%d build gates left behind", gates)
	}
}

const raceRounds = 40

func deploymentKey() streamKey {
	return streamKey{gvr: depGVR, ns: "default"}
}

func TestRegisterRefusesAStreamThatWasAlreadyDropped(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	key := deploymentKey()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	st, err := mgr.streamFor(context.Background(), key, desc)
	if err != nil {
		t.Fatalf("streamFor: %v", err)
	}

	mgr.mu.Lock()
	delete(mgr.streams, key)
	mgr.mu.Unlock()
	st.cancel()

	_, ok := mgr.register(key, st)

	if ok {
		t.Fatal("registered against a torn-down stream; the subscriber would never see an event")
	}
}

func TestAttachRebuildsAfterTheStreamIsDropped(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	key := deploymentKey()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	first, err := mgr.streamFor(context.Background(), key, desc)
	if err != nil {
		t.Fatalf("streamFor: %v", err)
	}
	mgr.mu.Lock()
	delete(mgr.streams, key)
	mgr.mu.Unlock()
	first.cancel()

	st, entry, attachErr := mgr.attach(context.Background(), key, desc)
	if attachErr != nil {
		t.Fatalf("attach: %v", attachErr)
	}
	if st == first {
		t.Fatal("attach handed back the dead stream")
	}

	st.fanout(Event{Kind: "added", Row: api.Row{UID: "u"}})
	select {
	case <-entry.events:
	default:
		t.Fatal("the rebuilt stream does not reach the subscriber")
	}
}

func TestDetachLeavesAStreamThatWasAlreadyReplaced(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	key := deploymentKey()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	old, entry, err := mgr.attach(context.Background(), key, desc)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	mgr.mu.Lock()
	delete(mgr.streams, key)
	mgr.mu.Unlock()

	replacement, replacementErr := mgr.streamFor(context.Background(), key, desc)
	if replacementErr != nil {
		t.Fatalf("streamFor: %v", replacementErr)
	}

	mgr.detach(key, old, entry)

	mgr.mu.Lock()
	live := mgr.streams[key]
	mgr.mu.Unlock()
	if live != replacement {
		t.Fatal("detaching an old subscriber removed the replacement stream")
	}
}

func TestASubscribeRacingTheLastCloseGetsALiveStream(t *testing.T) {
	client := newClient(t, newDeployment("default", "web"))
	mgr, cancel := newManager(t, client)
	t.Cleanup(cancel)

	for round := range raceRounds {
		first, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
		if err != nil {
			t.Fatalf("round %d: subscribe: %v", round, err)
		}

		var group sync.WaitGroup
		group.Add(2)
		var second *Subscription
		var secondErr error
		go func() {
			defer group.Done()
			first.Close()
		}()
		go func() {
			defer group.Done()
			second, secondErr = mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
		}()
		group.Wait()

		if secondErr != nil {
			t.Fatalf("round %d: racing subscribe: %v", round, secondErr)
		}
		mgr.mu.Lock()
		live := len(mgr.streams)
		mgr.mu.Unlock()
		if live != 1 {
			t.Fatalf("round %d: %d streams alive, want the newcomer's stream kept", round, live)
		}

		_, createErr := client.Resource(depGVR).Namespace("default").
			Create(context.Background(), newDeployment("default", "probe"), metav1.CreateOptions{})
		if createErr != nil {
			t.Fatalf("round %d: create: %v", round, createErr)
		}
		recvEvent(t, second.Events)
		second.Close()

		delErr := client.Resource(depGVR).Namespace("default").
			Delete(context.Background(), "probe", metav1.DeleteOptions{})
		if delErr != nil {
			t.Fatalf("round %d: delete: %v", round, delErr)
		}
	}
}

func TestAFullBufferAsksForAResync(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	stream := sub.stream
	for range eventBuffer + 10 {
		stream.fanout(Event{Kind: "added", Row: api.Row{UID: "u"}})
	}

	select {
	case <-sub.Resync:
	default:
		t.Fatal("events were dropped without asking the session to resync")
	}
}

func TestAResyncIsOnlySignalledOnce(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	for range eventBuffer * 3 {
		sub.stream.fanout(Event{Kind: "added", Row: api.Row{UID: "u"}})
	}

	<-sub.Resync
	select {
	case <-sub.Resync:
		t.Fatal("resync signals piled up; one pending request is enough")
	default:
	}
}

func TestAHealthySubscriberIsNotAskedToResync(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	for range eventBuffer {
		sub.stream.fanout(Event{Kind: "added", Row: api.Row{UID: "u"}})
	}

	select {
	case <-sub.Resync:
		t.Fatal("a subscriber that kept up was asked to resync")
	default:
	}
}

func TestSnapshotReReadsTheCache(t *testing.T) {
	client := newClient(t, newDeployment("default", "web"))
	mgr, cancel := newManager(t, client)
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	if len(sub.Rows) != 1 {
		t.Fatalf("initial rows = %d, want 1", len(sub.Rows))
	}

	_, createErr := client.Resource(depGVR).Namespace("default").
		Create(context.Background(), newDeployment("default", "api"), metav1.CreateOptions{})
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}
	recvEvent(t, sub.Events)

	rows, snapErr := sub.Snapshot()
	if snapErr != nil {
		t.Fatalf("snapshot: %v", snapErr)
	}
	if len(rows) != 2 {
		t.Fatalf("resync snapshot = %d rows, want 2", len(rows))
	}
}

func TestResyncChannelClosesWithTheSubscription(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	t.Cleanup(cancel)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	sub.Close()

	select {
	case _, ok := <-sub.Resync:
		if ok {
			t.Fatal("resync delivered a value after close")
		}
	case <-time.After(time.Second):
		t.Fatal("resync channel was left open; the relay would leak")
	}
}

func TestListReadsTheInformerCacheNotTheApiserver(t *testing.T) {
	client := newClient(t, newDeployment("default", "web"))
	lists := 0
	var listMu sync.Mutex
	client.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		listMu.Lock()
		lists++
		listMu.Unlock()
		return false, nil, nil
	})
	mgr, cancel := newManager(t, client)
	t.Cleanup(cancel)
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	for range 5 {
		items, err := mgr.List(context.Background(), desc)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("items = %d, want 1", len(items))
		}
	}

	listMu.Lock()
	defer listMu.Unlock()
	if lists > 1 {
		t.Fatalf("hit the apiserver %d times for 5 reads; the informer cache should serve them", lists)
	}
}

func TestListSeesObjectsTheInformerPicksUp(t *testing.T) {
	client := newClient(t)
	mgr, cancel := newManager(t, client)
	t.Cleanup(cancel)
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	before, err := mgr.List(context.Background(), desc)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("items = %d, want none", len(before))
	}

	_, createErr := client.Resource(depGVR).Namespace("default").
		Create(context.Background(), newDeployment("default", "api"), metav1.CreateOptions{})
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, listErr := mgr.List(context.Background(), desc)
		if listErr != nil {
			t.Fatalf("list: %v", listErr)
		}
		if len(items) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the informer never delivered the new object to the lister")
}

func TestAPinnedStreamOutlivesItsSubscribers(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	_, listErr := mgr.List(context.Background(), desc)
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	sub.Close()

	mgr.mu.Lock()
	live := len(mgr.streams)
	mgr.mu.Unlock()
	if live != 1 {
		t.Fatalf("streams = %d; closing the last subscriber tore down a stream a reader still needs", live)
	}
	items, afterErr := mgr.List(context.Background(), desc)
	if afterErr != nil {
		t.Fatalf("list after close: %v", afterErr)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d after the subscriber left", len(items))
	}
}

func TestListReportsAResourceThatNeverSyncs(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	_, err := mgr.List(context.Background(), desc)

	if err == nil {
		t.Fatal("a forbidden resource was reported as an empty list")
	}
}

func TestWarmLeavesTheListersUsable(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	mgr.Warm(context.Background(), []api.ResourceDescriptor{desc})

	items, err := mgr.List(context.Background(), desc)
	if err != nil {
		t.Fatalf("list after warm: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}

func TestWarmSurvivesAResourceThatNeverSyncs(t *testing.T) {
	mgr, _ := stuckManager(t, "deployments")
	descs := []api.ResourceDescriptor{
		testDescs()[discovery.Key("apps", "v1", "deployments")],
		testDescs()[discovery.Key("", "v1", "nodes")],
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Warm(context.Background(), descs)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("one unsyncable resource stalled the whole warm")
	}

	_, err := mgr.List(context.Background(), testDescs()[discovery.Key("", "v1", "nodes")])
	if err != nil {
		t.Fatalf("a healthy resource was unusable after a failed sibling: %v", err)
	}
}

func TestManagerUsesAnInjectedChartCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	provided := charts.New(ctx, &http.Client{}, time.Minute)

	mgr := NewManager(ctx, Deps{Charts: provided})

	if mgr.charts != provided {
		t.Fatal("the manager built its own chart cache instead of using the one it was given")
	}
}

func TestManagerBuildsAChartCacheWhenGivenNone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mgr := NewManager(ctx, Deps{})

	if mgr.charts == nil {
		t.Fatal("the manager has no chart cache and would panic on the flux dashboard")
	}
}
