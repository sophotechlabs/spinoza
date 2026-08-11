package overview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

type stubLister struct {
	nodes []*unstructured.Unstructured
	err   error
	calls int
}

func (s *stubLister) List(context.Context, api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.nodes, nil
}

type stubVersions struct {
	info *version.Info
	err  error
}

func (s *stubVersions) ServerVersion() (*version.Info, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.info, nil
}

func fullCatalog() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "nodes"):  {Version: "v1", Resource: "nodes", Kind: "Node"},
		discovery.Key("", "v1", "pods"):   {Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true},
		discovery.Key("", "v1", "events"): {Version: "v1", Resource: "events", Kind: "Event", Namespaced: true},
	}
}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "pods"}:                                "PodList",
		{Version: "v1", Resource: "events"}:                              "EventList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
		{Version: "v1", Resource: "nodes"}:                               "NodeList",
	}
}

func dynClient() *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds())
}

func seedNodeMetrics(t *testing.T, dyn *fake.FakeDynamicClient, objs ...*unstructured.Unstructured) {
	t.Helper()
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	for _, obj := range objs {
		_, err := dyn.Resource(gvr).Create(context.Background(), obj, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create node metrics %s: %v", obj.GetName(), err)
		}
	}
}

func seedEvents(t *testing.T, dyn *fake.FakeDynamicClient, objs ...*unstructured.Unstructured) {
	t.Helper()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	for _, obj := range objs {
		_, err := dyn.Resource(gvr).Namespace(obj.GetNamespace()).Create(context.Background(), obj, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create event %s: %v", obj.GetName(), err)
		}
	}
}

func node(name string, ready, cordoned bool, cpu, memory string) *unstructured.Unstructured {
	status := "False"
	if ready {
		status = "True"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"unschedulable": cordoned},
		"status": map[string]any{
			"allocatable": map[string]any{"cpu": cpu, "memory": memory},
			"conditions": []any{
				map[string]any{"type": "MemoryPressure", "status": "False"},
				map[string]any{"type": "Ready", "status": status},
			},
		},
	}}
}

func nodeMetric(name, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "NodeMetrics",
		"metadata":   map[string]any{"name": name},
		"usage":      map[string]any{"cpu": cpu, "memory": memory},
	}}
}

func warning(name, reason, object, seen string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion":     "v1",
		"kind":           "Event",
		"metadata":       map[string]any{"name": name, "namespace": "flux-system"},
		"type":           "Warning",
		"reason":         reason,
		"message":        reason + " happened",
		"count":          int64(3),
		"lastTimestamp":  seen,
		"involvedObject": map[string]any{"kind": "Pod", "name": object},
	}}
}

func answerPods(dyn *fake.FakeDynamicClient, counts map[string]int) {
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		selector := list.GetListRestrictions().Fields.String()
		total, known := counts[selector]
		if !known {
			return false, nil, nil
		}
		out := &unstructured.UnstructuredList{}
		remaining := int64(total)
		out.SetRemainingItemCount(&remaining)
		return true, out, nil
	})
}

func TestBuildReportsTheServerVersion(t *testing.T) {
	got := Build(context.Background(), dynClient(), &stubLister{}, &stubVersions{info: &version.Info{GitVersion: "v1.36.1"}}, map[string]api.ResourceDescriptor{})

	if got.Version != "v1.36.1" {
		t.Fatalf("version = %q, want v1.36.1", got.Version)
	}
}

func TestBuildSurvivesAClusterWithNoDiscoveryAtAll(t *testing.T) {
	got := Build(context.Background(), dynClient(), &stubLister{}, nil, map[string]api.ResourceDescriptor{})

	if got.Version != "" {
		t.Fatalf("version = %q, want it empty", got.Version)
	}
	if got.Nodes.Total != 0 {
		t.Fatalf("nodes = %d, want none", got.Nodes.Total)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", got.Warnings)
	}
}

func TestBuildSaysWhyTheVersionIsMissing(t *testing.T) {
	got := Build(context.Background(), dynClient(), &stubLister{}, &stubVersions{err: errors.New("apiserver is unreachable")}, map[string]api.ResourceDescriptor{})

	if !strings.Contains(got.Error, "apiserver is unreachable") {
		t.Fatalf("error = %q, want the apiserver failure in it", got.Error)
	}
}

func TestBuildCountsNodesByReadinessAndCapacity(t *testing.T) {
	lister := &stubLister{nodes: []*unstructured.Unstructured{
		node("a", true, false, "4", "8Gi"),
		node("b", true, true, "2", "4Gi"),
		node("c", false, false, "2", "4Gi"),
	}}

	got := Build(context.Background(), dynClient(), lister, nil, fullCatalog())

	if got.Nodes.Total != 3 {
		t.Fatalf("total = %d, want 3", got.Nodes.Total)
	}
	if got.Nodes.Ready != 2 {
		t.Fatalf("ready = %d, want 2", got.Nodes.Ready)
	}
	if got.Nodes.Unschedulable != 1 {
		t.Fatalf("unschedulable = %d, want 1", got.Nodes.Unschedulable)
	}
	if got.Nodes.CPUAllocatableMilli != 8000 {
		t.Fatalf("cpu = %d, want 8000", got.Nodes.CPUAllocatableMilli)
	}
	if got.Nodes.MemAllocatableMi != 16384 {
		t.Fatalf("memory = %d, want 16384", got.Nodes.MemAllocatableMi)
	}
}

func TestBuildAddsUpNodeUsage(t *testing.T) {
	lister := &stubLister{nodes: []*unstructured.Unstructured{node("a", true, false, "4", "8Gi")}}
	dyn := dynClient()
	seedNodeMetrics(t, dyn, nodeMetric("a", "1500m", "2Gi"), nodeMetric("b", "500m", "1Gi"))

	got := Build(context.Background(), dyn, lister, nil, fullCatalog())

	if got.Nodes.CPUUsedMilli != 2000 {
		t.Fatalf("cpu used = %d, want 2000", got.Nodes.CPUUsedMilli)
	}
	if got.Nodes.MemUsedMi != 3072 {
		t.Fatalf("memory used = %d, want 3072", got.Nodes.MemUsedMi)
	}
	if !got.Nodes.UsageKnown {
		t.Fatal("usage was reported as unknown even though metrics answered")
	}
}

func TestBuildMarksUsageUnknownWhenMetricsAreMissing(t *testing.T) {
	lister := &stubLister{nodes: []*unstructured.Unstructured{node("a", true, false, "4", "8Gi")}}
	dyn := dynClient()
	dyn.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the metrics api is not installed")
	})

	got := Build(context.Background(), dyn, lister, nil, fullCatalog())

	if got.Nodes.UsageKnown {
		t.Fatal("usage was reported as known with no metrics api")
	}
	if got.Nodes.Total != 1 {
		t.Fatalf("total = %d, want the node list to survive", got.Nodes.Total)
	}
	if !strings.Contains(got.Error, "metrics") {
		t.Fatalf("error = %q, want the metrics failure named", got.Error)
	}
}

func TestBuildSaysWhenTheNodeListFailed(t *testing.T) {
	lister := &stubLister{err: errors.New("nodes is forbidden")}

	got := Build(context.Background(), dynClient(), lister, nil, fullCatalog())

	if got.Nodes.Total != 0 {
		t.Fatalf("total = %d, want nothing", got.Nodes.Total)
	}
	if !strings.Contains(got.Error, "nodes is forbidden") {
		t.Fatalf("error = %q, want the node failure named", got.Error)
	}
}

func TestBuildCountsPodsByPhase(t *testing.T) {
	dyn := dynClient()
	answerPods(dyn, map[string]int{
		"":                       40,
		"status.phase=Running":   30,
		"status.phase=Pending":   4,
		"status.phase=Failed":    2,
		"status.phase=Succeeded": 4,
	})

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if got.Pods.Total != 40 {
		t.Fatalf("total = %d, want 40", got.Pods.Total)
	}
	if got.Pods.Running != 30 {
		t.Fatalf("running = %d, want 30", got.Pods.Running)
	}
	if got.Pods.Pending != 4 {
		t.Fatalf("pending = %d, want 4", got.Pods.Pending)
	}
	if got.Pods.Failed != 2 {
		t.Fatalf("failed = %d, want 2", got.Pods.Failed)
	}
	if got.Pods.Succeeded != 4 {
		t.Fatalf("succeeded = %d, want 4", got.Pods.Succeeded)
	}
	if !got.Pods.Known {
		t.Fatal("the pod tally was marked unknown even though every probe answered")
	}
}

func TestBuildMarksThePodTallyUnknownWhenAProbeFails(t *testing.T) {
	dyn := dynClient()
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if got.Pods.Known {
		t.Fatal("a refused probe still reported a known tally")
	}
	if !strings.Contains(got.Error, "pods is forbidden") {
		t.Fatalf("error = %q, want the pod failure named", got.Error)
	}
}

func TestBuildSkipsPodsThatDiscoveryNeverReported(t *testing.T) {
	dyn := dynClient()
	listed := 0
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		listed++
		return false, nil, nil
	})
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "nodes"): {Version: "v1", Resource: "nodes", Kind: "Node"},
	}

	got := Build(context.Background(), dyn, &stubLister{}, nil, descs)

	if listed != 0 {
		t.Fatalf("pods were listed %d times for a cluster that reports none", listed)
	}
	if got.Pods.Known {
		t.Fatal("a tally nobody could take was reported as known")
	}
}

func TestBuildReturnsTheNewestWarningsFirst(t *testing.T) {
	dyn := dynClient()
	seedEvents(
		t, dyn,
		warning("old", "BackOff", "web-1", "2026-08-11T10:00:00Z"),
		warning("new", "Unhealthy", "web-2", "2026-08-11T12:00:00Z"),
		warning("middle", "Failed", "web-3", "2026-08-11T11:00:00Z"),
	)

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if len(got.Warnings) != 3 {
		t.Fatalf("warnings = %d, want 3", len(got.Warnings))
	}
	if got.Warnings[0].Reason != "Unhealthy" {
		t.Fatalf("first = %q, want the newest", got.Warnings[0].Reason)
	}
	if got.Warnings[2].Reason != "BackOff" {
		t.Fatalf("last = %q, want the oldest", got.Warnings[2].Reason)
	}
	if got.Warnings[0].Object != "Pod/web-2" {
		t.Fatalf("object = %q, want Pod/web-2", got.Warnings[0].Object)
	}
	if got.Warnings[0].Namespace != "flux-system" {
		t.Fatalf("namespace = %q, want flux-system", got.Warnings[0].Namespace)
	}
	if got.Warnings[0].Count != 3 {
		t.Fatalf("count = %d, want 3", got.Warnings[0].Count)
	}
}

func TestBuildAsksTheApiserverOnlyForWarnings(t *testing.T) {
	dyn := dynClient()
	selectors := []string{}
	dyn.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if ok {
			selectors = append(selectors, list.GetListRestrictions().Fields.String())
		}
		return false, nil, nil
	})

	Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if len(selectors) != 1 {
		t.Fatalf("events were listed %d times, want once", len(selectors))
	}
	if selectors[0] != "type=Warning" {
		t.Fatalf("selector = %q, want type=Warning", selectors[0])
	}
}

func TestBuildCapsTheWarningsItReturns(t *testing.T) {
	dyn := dynClient()
	objs := make([]*unstructured.Unstructured, 0, warningsShown+5)
	for i := range warningsShown + 5 {
		objs = append(objs, warning(
			fmt.Sprintf("event-%d", i),
			"BackOff",
			"web",
			"2026-08-11T10:00:00Z",
		))
	}
	seedEvents(t, dyn, objs...)

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if len(got.Warnings) != warningsShown {
		t.Fatalf("warnings = %d, want the cap of %d", len(got.Warnings), warningsShown)
	}
}

func TestBuildSaysWhenEventsCouldNotBeRead(t *testing.T) {
	dyn := dynClient()
	dyn.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("events is forbidden")
	})

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", got.Warnings)
	}
	if !strings.Contains(got.Error, "events is forbidden") {
		t.Fatalf("error = %q, want the event failure named", got.Error)
	}
}

func TestBuildIsQuietWhenEverythingWorked(t *testing.T) {
	dyn := dynClient()
	seedNodeMetrics(t, dyn, nodeMetric("a", "1", "1Gi"))
	answerPods(dyn, map[string]int{
		"":                       1,
		"status.phase=Running":   1,
		"status.phase=Pending":   0,
		"status.phase=Failed":    0,
		"status.phase=Succeeded": 0,
	})
	lister := &stubLister{nodes: []*unstructured.Unstructured{node("a", true, false, "4", "8Gi")}}

	got := Build(context.Background(), dyn, lister, &stubVersions{info: &version.Info{GitVersion: "v1.36.1"}}, fullCatalog())

	if got.Error != "" {
		t.Fatalf("error = %q, want none", got.Error)
	}
}

func TestAnEventFallsBackThroughItsTimestamps(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata":       map[string]any{"namespace": "prod"},
		"involvedObject": map[string]any{"kind": "Node", "name": "worker-1"},
		"eventTime":      "2026-08-11T09:00:00Z",
		"series":         map[string]any{"count": int64(7)},
	}}

	got := eventOf(obj)

	if got.LastSeen != "2026-08-11T09:00:00Z" {
		t.Fatalf("lastSeen = %q, want the event time", got.LastSeen)
	}
	if got.Count != 7 {
		t.Fatalf("count = %d, want the series count", got.Count)
	}
	if got.Object != "Node/worker-1" {
		t.Fatalf("object = %q, want Node/worker-1", got.Object)
	}
}

func TestAnEventWithNothingToGoOnStillReads(t *testing.T) {
	got := eventOf(&unstructured.Unstructured{Object: map[string]any{}})

	if got.Count != 1 {
		t.Fatalf("count = %d, want 1", got.Count)
	}
	if got.LastSeen != "" {
		t.Fatalf("lastSeen = %q, want it empty", got.LastSeen)
	}
	if got.Object != "" {
		t.Fatalf("object = %q, want it empty", got.Object)
	}
}

func TestAnEventNamesWhicheverHalfOfTheObjectItHas(t *testing.T) {
	kindOnly := eventOf(&unstructured.Unstructured{Object: map[string]any{
		"involvedObject": map[string]any{"kind": "Pod"},
	}})
	nameOnly := eventOf(&unstructured.Unstructured{Object: map[string]any{
		"involvedObject": map[string]any{"name": "web"},
	}})

	if kindOnly.Object != "Pod" {
		t.Fatalf("object = %q, want Pod", kindOnly.Object)
	}
	if nameOnly.Object != "web" {
		t.Fatalf("object = %q, want web", nameOnly.Object)
	}
}

func TestANodeWithoutAReadyConditionIsNotReady(t *testing.T) {
	bare := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "a"},
		"status":   map[string]any{"conditions": []any{"not a condition"}},
	}}

	if readyNode(bare) {
		t.Fatal("a node with no Ready condition counted as ready")
	}
}

func TestUnreadableQuantitiesCountAsZero(t *testing.T) {
	lister := &stubLister{nodes: []*unstructured.Unstructured{{Object: map[string]any{
		"metadata": map[string]any{"name": "a"},
		"status":   map[string]any{"allocatable": map[string]any{"cpu": "four", "memory": "eight"}},
	}}}}

	got := Build(context.Background(), dynClient(), lister, nil, fullCatalog())

	if got.Nodes.CPUAllocatableMilli != 0 {
		t.Fatalf("cpu = %d, want 0", got.Nodes.CPUAllocatableMilli)
	}
	if got.Nodes.MemAllocatableMi != 0 {
		t.Fatalf("memory = %d, want 0", got.Nodes.MemAllocatableMi)
	}
	if got.Nodes.Total != 1 {
		t.Fatalf("total = %d, want the node itself to count", got.Nodes.Total)
	}
}

func TestANodeWithNoAllocatableAtAllStillCounts(t *testing.T) {
	lister := &stubLister{nodes: []*unstructured.Unstructured{{Object: map[string]any{
		"metadata": map[string]any{"name": "a"},
	}}}}

	got := Build(context.Background(), dynClient(), lister, nil, fullCatalog())

	if got.Nodes.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Nodes.Total)
	}
}

func TestBuildSaysWhenThereAreMorePodsThanItWillCount(t *testing.T) {
	dyn := dynClient()
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		out := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]any{"name": "one", "namespace": "default"},
		}}}}
		out.SetContinue("more")
		return true, out, nil
	})

	got := Build(context.Background(), dyn, &stubLister{}, nil, fullCatalog())

	if !strings.Contains(got.Error, "the tally stops there") {
		t.Fatalf("error = %q, want the truncation named", got.Error)
	}
	if !got.Pods.Known {
		t.Fatal("a truncated tally is still a tally; it should not read as unknown")
	}
}
