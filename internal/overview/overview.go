package overview

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/metrics"
	"github.com/sophotechlabs/spinoza/internal/podcount"
	"github.com/sophotechlabs/spinoza/internal/safe"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	buildTimeout  = 20 * time.Second
	probeTimeout  = 5 * time.Second
	warningWindow = 200
	warningPages  = 20
	warningsShown = 25
)

var eventsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

const (
	nodesKey  = "/v1/nodes"
	podsKey   = "/v1/pods"
	eventsKey = "/v1/events"
)

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
}

type Versions interface {
	ServerVersion() (*version.Info, error)
}

func Build(
	ctx context.Context,
	dyn dynamic.Interface,
	meta metadata.Interface,
	lister Lister,
	versions Versions,
	descs map[string]api.ResourceDescriptor,
) api.ClusterOverview {
	bounded, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	failures := listerr.New()
	out := api.ClusterOverview{
		Version:  serverVersion(versions, failures),
		Warnings: []api.OverviewEvent{},
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go safe.Run("counting nodes", func() {
		defer wg.Done()
		out.Nodes = nodeSummary(bounded, dyn, lister, descs, failures)
	})
	go safe.Run("counting pod phases", func() {
		defer wg.Done()
		out.Pods = podSummary(bounded, meta, descs, failures)
	})
	go safe.Run("collecting warnings", func() {
		defer wg.Done()
		out.Warnings = warnings(bounded, dyn, descs, failures)
	})
	wg.Wait()

	out.Error = failures.Message()
	return out
}

func serverVersion(versions Versions, failures *listerr.Collector) string {
	if versions == nil {
		return ""
	}
	info, err := versions.ServerVersion()
	failures.Record("server version", err)
	if err != nil {
		return ""
	}
	return info.GitVersion
}

func nodeSummary(ctx context.Context, dyn dynamic.Interface, lister Lister, descs map[string]api.ResourceDescriptor, failures *listerr.Collector) api.NodeSummary {
	summary := api.NodeSummary{}
	desc, ok := descs[discovery.Key("", "v1", "nodes")]
	if !ok {
		return summary
	}
	nodes, err := lister.List(ctx, desc)
	failures.Record(nodesKey, err)
	if err != nil {
		return summary
	}
	for _, node := range nodes {
		summary.Total++
		if readyNode(node) {
			summary.Ready++
		}
		if unstr.Bool(node, "spec", "unschedulable") {
			summary.Unschedulable++
		}
		alloc, found := unstr.Map(node, "status", "allocatable")
		if !found {
			continue
		}
		summary.CPUAllocatableMilli += milliOf(alloc, "cpu")
		summary.MemAllocatableMi += mebiOf(alloc, "memory")
	}
	addUsage(ctx, dyn, &summary, failures)
	return summary
}

func addUsage(ctx context.Context, dyn dynamic.Interface, summary *api.NodeSummary, failures *listerr.Collector) {
	bounded, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	usage, err := metrics.NodeUsage(bounded, dyn)
	if err != nil {
		failures.Record("nodes.metrics.k8s.io", err)
		return
	}
	for _, use := range usage {
		summary.CPUUsedMilli += use.CPUMilli
		summary.MemUsedMi += use.MemoryMi
	}
	summary.UsageKnown = true
}

func readyNode(node *unstructured.Unstructured) bool {
	for _, raw := range unstr.Slice(node, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != "Ready" {
			continue
		}
		return unstr.At(entry, "status") == "True"
	}
	return false
}

type phaseProbe struct {
	label    string
	selector string
	into     *int
}

func podSummary(
	ctx context.Context,
	meta metadata.Interface,
	descs map[string]api.ResourceDescriptor,
	failures *listerr.Collector,
) api.PodSummary {
	if meta == nil {
		return api.PodSummary{}
	}
	summary := api.PodSummary{}
	_, ok := descs[discovery.Key("", "v1", "pods")]
	if !ok {
		return summary
	}
	probes := []phaseProbe{
		{label: "pods", selector: "", into: &summary.Total},
		{label: "running pods", selector: "status.phase=Running", into: &summary.Running},
		{label: "pending pods", selector: "status.phase=Pending", into: &summary.Pending},
		{label: "failed pods", selector: "status.phase=Failed", into: &summary.Failed},
		{label: "succeeded pods", selector: "status.phase=Succeeded", into: &summary.Succeeded},
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	broken := false
	partial := false
	for _, probe := range probes {
		wg.Add(1)
		go safe.Run("counting "+probe.label, func() {
			defer wg.Done()
			bounded, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			got, err := podcount.Count(bounded, meta, probe.selector)
			mu.Lock()
			defer mu.Unlock()
			failures.Record(podsKey+" "+probe.label, err)
			if err != nil {
				broken = true
				return
			}
			if !got.Complete {
				partial = true
			}
			*probe.into = got.Total
		})
	}
	wg.Wait()
	summary.Known = !broken
	if partial {
		failures.Record(podsKey, fmt.Errorf(
			"more than %d pods, so the tally stops there", podcount.Limit(),
		))
	}
	return summary
}

func warnings(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, failures *listerr.Collector) []api.OverviewEvent {
	out := []api.OverviewEvent{}
	_, ok := descs[discovery.Key("", "v1", "events")]
	if !ok {
		return out
	}
	bounded, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	opts := metav1.ListOptions{Limit: warningWindow, FieldSelector: "type=Warning"}
	for range warningPages {
		list, err := dyn.Resource(eventsGVR).List(bounded, opts)
		if err != nil {
			failures.Record(eventsKey, err)
			return out
		}
		for i := range list.Items {
			out = append(out, eventOf(&list.Items[i]))
		}
		out = newestFirst(out)
		if list.GetContinue() == "" {
			failures.Record(eventsKey, nil)
			return out
		}
		opts.Continue = list.GetContinue()
	}
	failures.Record(eventsKey, fmt.Errorf(
		"more than %d warning events, so the newest are taken from the first %d",
		warningWindow*warningPages, warningWindow*warningPages,
	))
	return out
}

func newestFirst(events []api.OverviewEvent) []api.OverviewEvent {
	slices.SortStableFunc(events, func(left, right api.OverviewEvent) int {
		return seenAt(right.LastSeen).Compare(seenAt(left.LastSeen))
	})
	if len(events) > warningsShown {
		return events[:warningsShown]
	}
	return events
}

func eventOf(obj *unstructured.Unstructured) api.OverviewEvent {
	return api.OverviewEvent{
		Namespace: obj.GetNamespace(),
		Object:    objectOf(obj),
		Reason:    unstr.String(obj, "reason"),
		Message:   unstr.String(obj, "message"),
		Count:     countOf(obj),
		LastSeen:  lastSeenOf(obj),
	}
}

func objectOf(obj *unstructured.Unstructured) string {
	kind := unstr.String(obj, "involvedObject", "kind")
	name := unstr.String(obj, "involvedObject", "name")
	if kind == "" {
		return name
	}
	if name == "" {
		return kind
	}
	return kind + "/" + name
}

func countOf(obj *unstructured.Unstructured) int64 {
	count := unstr.Int(obj, "count")
	if count > 0 {
		return count
	}
	series := unstr.Int(obj, "series", "count")
	if series > 0 {
		return series
	}
	return 1
}

func lastSeenOf(obj *unstructured.Unstructured) string {
	paths := [][]string{
		{"lastTimestamp"},
		{"series", "lastObservedTime"},
		{"eventTime"},
		{"firstTimestamp"},
	}
	for _, path := range paths {
		found := unstr.String(obj, path...)
		if found != "" {
			return found
		}
	}
	return ""
}

func seenAt(stamp string) time.Time {
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return time.Time{}
	}
	return at
}

func milliOf(quantities map[string]any, key string) int64 {
	q, ok := quantityOf(quantities, key)
	if !ok {
		return 0
	}
	return q.MilliValue()
}

func mebiOf(quantities map[string]any, key string) int64 {
	q, ok := quantityOf(quantities, key)
	if !ok {
		return 0
	}
	return q.Value() / (1024 * 1024)
}

func quantityOf(quantities map[string]any, key string) (resource.Quantity, bool) {
	raw, ok := quantities[key].(string)
	if !ok {
		return resource.Quantity{}, false
	}
	q, err := resource.ParseQuantity(raw)
	if err != nil {
		return resource.Quantity{}, false
	}
	return q, true
}
