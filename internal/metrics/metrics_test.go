package metrics

import (
	"context"
	"errors"
	"maps"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		podMetricsGVR:  "PodMetricsList",
		nodeMetricsGVR: "NodeMetricsList",
		nodeGVR:        "NodeList",
	}
}

func obj(apiVersion, kind, name, namespace string, extra map[string]any) *unstructured.Unstructured {
	object := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}
	maps.Copy(object, extra)
	return &unstructured.Unstructured{Object: object}
}

func create(t *testing.T, dyn *fake.FakeDynamicClient, gvr schema.GroupVersionResource, ns string, object *unstructured.Unstructured) {
	t.Helper()
	var err error
	if ns == "" {
		_, err = dyn.Resource(gvr).Create(context.Background(), object, metav1.CreateOptions{})
	} else {
		_, err = dyn.Resource(gvr).Namespace(ns).Create(context.Background(), object, metav1.CreateOptions{})
	}
	if err != nil {
		t.Fatalf("create %s/%s: %v", gvr.Resource, object.GetName(), err)
	}
}

func seed(t *testing.T) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds())
	create(t, dyn, podMetricsGVR, "prod", obj("metrics.k8s.io/v1beta1", "PodMetrics", "web", "prod", map[string]any{
		"containers": []any{
			"not-a-map",
			map[string]any{"name": "no-usage"},
			map[string]any{"name": "a", "usage": map[string]any{"cpu": "100m", "memory": "128Mi"}},
			map[string]any{"name": "b", "usage": map[string]any{"cpu": "50m", "memory": "64Mi"}},
		},
	}))
	create(t, dyn, nodeMetricsGVR, "", obj("metrics.k8s.io/v1beta1", "NodeMetrics", "n1", "", map[string]any{
		"usage": map[string]any{"cpu": "1500m", "memory": "2048Mi"},
	}))
	create(t, dyn, nodeMetricsGVR, "", obj("metrics.k8s.io/v1beta1", "NodeMetrics", "n2", "", map[string]any{
		"usage": map[string]any{"cpu": "500m", "memory": "512Mi"},
	}))
	create(t, dyn, nodeMetricsGVR, "", obj("metrics.k8s.io/v1beta1", "NodeMetrics", "n-nousage", "", map[string]any{}))
	create(t, dyn, nodeGVR, "", obj("v1", "Node", "n1", "", map[string]any{
		"status": map[string]any{"allocatable": map[string]any{"cpu": "4", "memory": "8192Mi"}},
	}))
	create(t, dyn, nodeGVR, "", obj("v1", "Node", "n3", "", map[string]any{
		"status": map[string]any{},
	}))
	return dyn
}

func TestBuild(t *testing.T) {
	dyn := seed(t)

	metrics := Build(context.Background(), dyn, FromCluster(dyn))

	web := metrics.Pods["prod/web"]
	if web.CPUMilli != 150 || web.MemoryMi != 192 {
		t.Fatalf("pod web = %+v, want cpu 150 mem 192", web)
	}

	if len(metrics.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2 (n-nousage skipped)", len(metrics.Nodes))
	}
	n1 := metrics.Nodes["n1"]
	if n1.CPUMilli != 1500 || n1.MemoryMi != 2048 || n1.CPUPercent != 37 || n1.MemPercent != 25 {
		t.Fatalf("node n1 = %+v", n1)
	}
	n2 := metrics.Nodes["n2"]
	if n2.CPUMilli != 500 || n2.CPUPercent != 0 || n2.MemPercent != 0 {
		t.Fatalf("node n2 = %+v, want no percent", n2)
	}
	if n1.CPUAllocatableMilli != 4000 || n1.MemAllocatableMi != 8192 {
		t.Fatalf("node n1 = %+v, want what the node has to give", n1)
	}
	if n2.CPUAllocatableMilli != 0 || n2.MemAllocatableMi != 0 {
		t.Fatalf("node n2 = %+v, want nothing claimed about a ceiling", n2)
	}
}

func TestBuildListErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds())
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods list failed")
	})
	dyn.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("nodes list failed")
	})
	m := Build(context.Background(), dyn, FromCluster(dyn))
	if len(m.Pods) != 0 {
		t.Fatalf("pods = %d, want 0", len(m.Pods))
	}
	if len(m.Nodes) != 0 {
		t.Fatalf("nodes = %d, want 0", len(m.Nodes))
	}
}

func TestNodeAllocatableListError(t *testing.T) {
	dyn := seed(t)
	dyn.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Group == "" {
			return true, nil, errors.New("core nodes list failed")
		}
		return false, nil, nil
	})
	m := Build(context.Background(), dyn, FromCluster(dyn))
	n1 := m.Nodes["n1"]
	if n1.CPUMilli != 1500 || n1.CPUPercent != 0 {
		t.Fatalf("node n1 = %+v, want usage without percent", n1)
	}
}

func TestQuantityHelpers(t *testing.T) {
	if got := milli(map[string]any{"cpu": "250m"}); got != 250 {
		t.Fatalf("milli valid = %d, want 250", got)
	}
	if got := mebi(map[string]any{"memory": "512Mi"}); got != 512 {
		t.Fatalf("mebi valid = %d, want 512", got)
	}
	if got := milli(map[string]any{"cpu": 5}); got != 0 {
		t.Fatalf("milli non-string = %d, want 0", got)
	}
	if got := mebi(map[string]any{"memory": "bad"}); got != 0 {
		t.Fatalf("mebi parse error = %d, want 0", got)
	}
	if got := milli(map[string]any{}); got != 0 {
		t.Fatalf("milli missing = %d, want 0", got)
	}
}

func TestPercent(t *testing.T) {
	if got := percent(50, 200); got != 25 {
		t.Fatalf("percent(50,200) = %d, want 25", got)
	}
	if got := percent(5, 0); got != 0 {
		t.Fatalf("percent(5,0) = %d, want 0", got)
	}
}

type stubNodes struct {
	items []*unstructured.Unstructured
	err   error
}

func (s stubNodes) List(context.Context) ([]*unstructured.Unstructured, error) {
	return s.items, s.err
}

func TestAllocatableComesFromTheGivenSource(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds())
	create(t, dyn, nodeMetricsGVR, "", obj("metrics.k8s.io/v1beta1", "NodeMetrics", "n1", "", map[string]any{
		"usage": map[string]any{"cpu": "1000m", "memory": "1024Mi"},
	}))
	cached := stubNodes{items: []*unstructured.Unstructured{
		obj("v1", "Node", "n1", "", map[string]any{
			"status": map[string]any{"allocatable": map[string]any{"cpu": "4", "memory": "4096Mi"}},
		}),
	}}

	built := Build(context.Background(), dyn, cached)

	if built.Nodes["n1"].CPUPercent != 25 || built.Nodes["n1"].MemPercent != 25 {
		t.Fatalf("node n1 = %+v, want a quarter of the cached allocatable", built.Nodes["n1"])
	}
}

func TestANodeSourceThatFailsIsReported(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds())
	create(t, dyn, nodeMetricsGVR, "", obj("metrics.k8s.io/v1beta1", "NodeMetrics", "n1", "", map[string]any{
		"usage": map[string]any{"cpu": "1000m", "memory": "1024Mi"},
	}))

	built := Build(context.Background(), dyn, stubNodes{err: errors.New("nodes is forbidden")})

	if built.Error == "" {
		t.Fatal("a refused node listing was swallowed")
	}
	if built.Nodes["n1"].CPUMilli != 1000 {
		t.Fatalf("node n1 = %+v, want the usage without percentages", built.Nodes["n1"])
	}
	if built.Nodes["n1"].CPUPercent != 0 {
		t.Fatalf("cpu percent = %d, want none without allocatable", built.Nodes["n1"].CPUPercent)
	}
}
