package metrics

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var (
	podMetricsGVR  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	nodeGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
)

var buildTimeout = 20 * time.Second

func Build(ctx context.Context, dyn dynamic.Interface) api.Metrics {
	bounded, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	ctx = bounded
	failures := listerr.New()
	return api.Metrics{
		Pods:  podUsage(ctx, dyn, failures),
		Nodes: nodeUsage(ctx, dyn, failures),
		Error: failures.Message(),
	}
}

func podUsage(ctx context.Context, dyn dynamic.Interface, failures *listerr.Collector) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(podMetricsGVR).List(ctx, metav1.ListOptions{})
	failures.Record("pods.metrics.k8s.io", err)
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
	for _, c := range unstr.Slice(u, "containers") {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		usage, ok := m["usage"].(map[string]any)
		if !ok {
			continue
		}
		cpuMilli += milli(usage)
		memMi += mebi(usage)
	}
	return cpuMilli, memMi
}

func nodeUsage(ctx context.Context, dyn dynamic.Interface, failures *listerr.Collector) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	failures.Record("nodes.metrics.k8s.io", err)
	if err != nil {
		return out
	}
	allocatable := nodeAllocatable(ctx, dyn, failures)
	for i := range list.Items {
		obj := &list.Items[i]
		usage, ok := unstr.Map(obj, "usage")
		if !ok {
			continue
		}
		cpu := milli(usage)
		mem := mebi(usage)
		use := api.ResourceUsage{CPUMilli: cpu, MemoryMi: mem}
		alloc, ok := allocatable[obj.GetName()]
		if ok {
			use.CPUPercent = percent(cpu, alloc.CPUMilli)
			use.MemPercent = percent(mem, alloc.MemoryMi)
		}
		out[obj.GetName()] = use
	}
	return out
}

func nodeAllocatable(ctx context.Context, dyn dynamic.Interface, failures *listerr.Collector) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	list, err := dyn.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	failures.Record("nodes", err)
	if err != nil {
		return out
	}
	for i := range list.Items {
		u := &list.Items[i]
		alloc, ok := unstr.Map(u, "status", "allocatable")
		if !ok {
			continue
		}
		out[u.GetName()] = api.ResourceUsage{CPUMilli: milli(alloc), MemoryMi: mebi(alloc)}
	}
	return out
}

func percent(used, total int64) int64 {
	if total <= 0 {
		return 0
	}
	return used * 100 / total
}

func milli(m map[string]any) int64 {
	q, ok := quantity(m, "cpu")
	if !ok {
		return 0
	}
	return q.MilliValue()
}

func mebi(m map[string]any) int64 {
	q, ok := quantity(m, "memory")
	if !ok {
		return 0
	}
	return q.Value() / (1024 * 1024)
}

func quantity(m map[string]any, key string) (resource.Quantity, bool) {
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
