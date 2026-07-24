package resources

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

var depGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

var nodeGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		depGVR:  "DeploymentList",
		nodeGVR: "NodeList",
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
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       "uid-" + name,
		},
		"spec": map[string]interface{}{"replicas": int64(2)},
		"status": map[string]interface{}{
			"readyReplicas":     int64(2),
			"updatedReplicas":   int64(2),
			"availableReplicas": int64(2),
		},
	}}
}

func newNode(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]interface{}{
			"name": name,
			"uid":  "uid-" + name,
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
			"nodeInfo": map[string]interface{}{"kubeletVersion": "v1.31.0"},
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
	mgr := NewManager(ctx, dyn, []api.Category{{Name: "Workloads"}}, testDescs())
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
	repo := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata": map[string]interface{}{
			"name":      "app-repo",
			"namespace": "flux-system",
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := NewManager(ctx, dyn, nil, descs)

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

func TestResources(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	cats := mgr.Resources()
	if len(cats) != 1 {
		t.Fatalf("categories = %d, want 1", len(cats))
	}
	if cats[0].Name != "Workloads" {
		t.Fatalf("category = %q, want Workloads", cats[0].Name)
	}
}

func TestSubscribeSnapshot(t *testing.T) {
	dyn := newClient(t, newDeployment("default", "web"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
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
	_, err := mgr.Subscribe("apps", "v1", "statefulsets", "default")
	if err == nil {
		t.Fatal("Subscribe returned nil error for unknown resource")
	}
}

func TestSubscribeCacheSyncFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := NewManager(ctx, newClient(t), nil, testDescs())
	_, err := mgr.Subscribe("apps", "v1", "deployments", "default")
	if err == nil {
		t.Fatal("Subscribe returned nil error when cache sync could not complete")
	}
}

func TestSubscribeDeliversEvents(t *testing.T) {
	ctx := context.Background()
	dyn := newClient(t)
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
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
	if err := unstructured.SetNestedField(cur.Object, int64(5), "spec", "replicas"); err != nil {
		t.Fatalf("set replicas: %v", err)
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

	sub1, err := mgr.Subscribe("apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("Subscribe 1: %v", err)
	}
	sub2, err := mgr.Subscribe("apps", "v1", "deployments", "default")
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

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
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

	sub, err := mgr.Subscribe("", "v1", "nodes", "ignored-namespace")
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

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "team-a")
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
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
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
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
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
