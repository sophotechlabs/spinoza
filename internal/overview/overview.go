package overview

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	safe.Go("counting nodes", func() {
		defer wg.Done()
		defer func() {
			failures.RecordPanic(nodesKey, "counting nodes", recover())
		}()
		out.Nodes = nodeSummary(bounded, dyn, lister, descs, failures)
	})
	safe.Go("counting pod phases", func() {
		defer wg.Done()
		defer func() {
			failures.RecordPanic(podsKey, "counting pod phases", recover())
		}()
		out.Pods = podSummary(bounded, meta, descs, failures)
	})
	safe.Go("collecting warnings", func() {
		defer wg.Done()
		defer func() {
			failures.RecordPanic(eventsKey, "collecting warnings", recover())
		}()
		out.Warnings, out.WarningCount = warnings(bounded, dyn, descs, failures)
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
	field    string
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
		{label: "pods", field: "total", selector: "", into: &summary.Total},
		{label: "running pods", field: "running", selector: "status.phase=Running", into: &summary.Running},
		{label: "pending pods", field: "pending", selector: "status.phase=Pending", into: &summary.Pending},
		{label: "failed pods", field: "failed", selector: "status.phase=Failed", into: &summary.Failed},
		{label: "succeeded pods", field: "succeeded", selector: "status.phase=Succeeded", into: &summary.Succeeded},
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	broken := false
	capped := map[string]int{}
	for _, probe := range probes {
		what := "counting " + probe.label
		resourceKey := podsKey + " " + probe.label
		wg.Add(1)
		safe.Go(what, func() {
			defer wg.Done()
			defer func() {
				caught := recover()
				if caught == nil {
					return
				}
				safe.Log(what, caught)
				mu.Lock()
				failures.Record(resourceKey, errors.New("spinoza could not finish counting this pod phase"))
				broken = true
				mu.Unlock()
			}()
			bounded, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			got, err := podcount.Count(bounded, meta, probe.selector)
			mu.Lock()
			defer mu.Unlock()
			failures.Record(resourceKey, err)
			if err != nil {
				broken = true
				return
			}
			if !got.Complete {
				capped[probe.field] = got.Total
			}
			*probe.into = got.Total
		})
	}
	wg.Wait()
	summary.Known = !broken
	summary.Capped = cappedFields(probes, capped)
	if len(summary.Capped) > 0 {
		failures.Record(podsKey, fmt.Errorf(
			"more than %d pods in %s, so the tally stops there",
			lowestCap(capped), strings.Join(summary.Capped, ", "),
		))
	}
	return summary
}

func cappedFields(probes []phaseProbe, capped map[string]int) []string {
	out := []string{}
	for _, probe := range probes {
		_, stopped := capped[probe.field]
		if stopped {
			out = append(out, probe.field)
		}
	}
	return out
}

func lowestCap(capped map[string]int) int {
	lowest := 0
	for _, at := range capped {
		if lowest == 0 || at < lowest {
			lowest = at
		}
	}
	return lowest
}

func warnings(
	ctx context.Context, dyn dynamic.Interface,
	descs map[string]api.ResourceDescriptor, failures *listerr.Collector,
) (shown []api.OverviewEvent, found int) {
	out := []api.OverviewEvent{}
	_, ok := descs[discovery.Key("", "v1", "events")]
	if !ok {
		return out, len(out)
	}
	bounded, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	opts := metav1.ListOptions{Limit: warningWindow, FieldSelector: "type=Warning"}
	// The list is cut every page, so what was read has to be counted as it
	// arrives rather than measured off the end.
	seen := 0
	for range warningPages {
		list, err := dyn.Resource(eventsGVR).List(bounded, opts)
		if err != nil {
			failures.Record(eventsKey, err)
			return out, seen
		}
		for i := range list.Items {
			out = append(out, eventOf(&list.Items[i]))
			seen++
		}
		out = newestFirst(out)
		if list.GetContinue() == "" {
			failures.Record(eventsKey, nil)
			return out, seen
		}
		opts.Continue = list.GetContinue()
	}
	failures.Record(eventsKey, fmt.Errorf(
		"more than %d warning events, so the newest are taken from the first %d",
		warningWindow*warningPages, warningWindow*warningPages,
	))
	return out, seen
}

func newestFirst(events []api.OverviewEvent) []api.OverviewEvent {
	slices.SortStableFunc(events, func(left, right api.OverviewEvent) int {
		if when := seenAt(right.LastSeen).Compare(seenAt(left.LastSeen)); when != 0 {
			return when
		}
		return strings.Compare(overviewEventKey(left), overviewEventKey(right))
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

func overviewEventKey(one api.OverviewEvent) string {
	return one.Namespace + "\x00" + one.Object + "\x00" + one.Reason + "\x00" + one.Message
}
