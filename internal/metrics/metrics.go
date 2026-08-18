package metrics

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/safe"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var (
	podMetricsGVR  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	nodeGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
)

var buildTimeout = 20 * time.Second

type Nodes interface {
	List(ctx context.Context) ([]*unstructured.Unstructured, error)
}

type dynamicNodes struct {
	dyn dynamic.Interface
}

func FromCluster(dyn dynamic.Interface) Nodes {
	return dynamicNodes{dyn: dyn}
}

func (d dynamicNodes) List(ctx context.Context) ([]*unstructured.Unstructured, error) {
	list, err := d.dyn.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}

func Build(ctx context.Context, dyn dynamic.Interface, nodes Nodes) api.Metrics {
	bounded, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	ctx = bounded
	failures := listerr.New()
	var pods map[string]api.ResourceUsage
	var used map[string]api.ResourceUsage
	var group sync.WaitGroup
	group.Add(2)
	go safe.Run("reading pod metrics", func() {
		defer group.Done()
		pods = podUsage(ctx, dyn, failures)
	})
	go safe.Run("reading node metrics", func() {
		defer group.Done()
		used = nodeUsage(ctx, dyn, nodes, failures)
	})
	group.Wait()
	return api.Metrics{
		Pods:  pods,
		Nodes: used,
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

func NodeUsage(ctx context.Context, dyn dynamic.Interface) (map[string]api.ResourceUsage, error) {
	list, err := dyn.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := map[string]api.ResourceUsage{}
	for i := range list.Items {
		obj := &list.Items[i]
		usage, ok := unstr.Map(obj, "usage")
		if !ok {
			continue
		}
		out[obj.GetName()] = api.ResourceUsage{CPUMilli: milli(usage), MemoryMi: mebi(usage)}
	}
	return out, nil
}

func nodeUsage(
	ctx context.Context,
	dyn dynamic.Interface,
	nodes Nodes,
	failures *listerr.Collector,
) map[string]api.ResourceUsage {
	usage, err := NodeUsage(ctx, dyn)
	failures.Record("nodes.metrics.k8s.io", err)
	if err != nil {
		return map[string]api.ResourceUsage{}
	}
	allocatable := nodeAllocatable(ctx, nodes, failures)
	out := map[string]api.ResourceUsage{}
	for name, use := range usage {
		alloc, ok := allocatable[name]
		if ok {
			use.CPUPercent = percent(use.CPUMilli, alloc.CPUMilli)
			use.MemPercent = percent(use.MemoryMi, alloc.MemoryMi)
		}
		out[name] = use
	}
	return out
}

func nodeAllocatable(
	ctx context.Context,
	nodes Nodes,
	failures *listerr.Collector,
) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	found, err := nodes.List(ctx)
	failures.Record("nodes", err)
	if err != nil {
		return out
	}
	for _, u := range found {
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
