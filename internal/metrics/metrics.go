package metrics

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var (
	podMetricsGVR  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	nodeGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
)

func Build(ctx context.Context, dyn dynamic.Interface) api.Metrics {
	return api.Metrics{
		Pods:  podUsage(ctx, dyn),
		Nodes: nodeUsage(ctx, dyn),
	}
}

func podUsage(ctx context.Context, dyn dynamic.Interface) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(podMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for i := range list.Items {
		u := &list.Items[i]
		cpu, mem := containerTotals(u)
		out[u.GetNamespace()+"/"+u.GetName()] = api.ResourceUsage{CPUMilli: cpu, MemoryMi: mem}
	}
	return out
}

func containerTotals(u *unstructured.Unstructured) (cpuMilli, memMi int64) {
	for _, c := range nestedSlice(u, "containers") {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		usage, ok := m["usage"].(map[string]interface{})
		if !ok {
			continue
		}
		cpuMilli += milli(usage, "cpu")
		memMi += mebi(usage, "memory")
	}
	return cpuMilli, memMi
}

func nodeUsage(ctx context.Context, dyn dynamic.Interface) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	allocatable := nodeAllocatable(ctx, dyn)
	for i := range list.Items {
		u := &list.Items[i]
		usage, ok := nestedMap(u, "usage")
		if !ok {
			continue
		}
		cpu := milli(usage, "cpu")
		mem := mebi(usage, "memory")
		use := api.ResourceUsage{CPUMilli: cpu, MemoryMi: mem}
		alloc, ok := allocatable[u.GetName()]
		if ok {
			use.CPUPercent = percent(cpu, alloc.CPUMilli)
			use.MemPercent = percent(mem, alloc.MemoryMi)
		}
		out[u.GetName()] = use
	}
	return out
}

func nodeAllocatable(ctx context.Context, dyn dynamic.Interface) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for i := range list.Items {
		u := &list.Items[i]
		alloc, ok := nestedMap(u, "status", "allocatable")
		if !ok {
			continue
		}
		out[u.GetName()] = api.ResourceUsage{CPUMilli: milli(alloc, "cpu"), MemoryMi: mebi(alloc, "memory")}
	}
	return out
}

func percent(used, total int64) int64 {
	if total <= 0 {
		return 0
	}
	return used * 100 / total
}

func milli(m map[string]interface{}, key string) int64 {
	q, ok := quantity(m, key)
	if !ok {
		return 0
	}
	return q.MilliValue()
}

func mebi(m map[string]interface{}, key string) int64 {
	q, ok := quantity(m, key)
	if !ok {
		return 0
	}
	return q.Value() / (1024 * 1024)
}

func quantity(m map[string]interface{}, key string) (resource.Quantity, bool) {
	s, ok := m[key].(string)
	if !ok {
		return resource.Quantity{}, false
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.Quantity{}, false
	}
	return q, true
}

func nestedSlice(u *unstructured.Unstructured, fields ...string) []interface{} {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}

func nestedMap(u *unstructured.Unstructured, fields ...string) (map[string]interface{}, bool) {
	v, found, err := unstructured.NestedMap(u.Object, fields...)
	if !found || err != nil {
		return nil, false
	}
	return v, true
}
