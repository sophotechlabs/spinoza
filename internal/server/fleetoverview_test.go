package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type surveying struct {
	notStubbed

	overview api.ClusterOverview
	counts   api.ResourceCounts
	pods     []*unstructured.Unstructured
	listErr  error
}

func (s *surveying) Overview(context.Context) api.ClusterOverview {
	return s.overview
}

func (s *surveying) Counts(context.Context) api.ResourceCounts {
	return s.counts
}

func (s *surveying) ListKind(context.Context, api.ObjectRef) ([]*unstructured.Unstructured, error) {
	return s.pods, s.listErr
}

func overviewOf(version string, nodes, ready, pods, running int) api.ClusterOverview {
	return api.ClusterOverview{
		Version: version,
		Nodes:   api.NodeSummary{Total: nodes, Ready: ready, CPUAllocatableMilli: 1000},
		Pods:    api.PodSummary{Total: pods, Running: running, Known: true},
	}
}

func TestTheFleetOverviewHasALinePerCluster(t *testing.T) {
	ts := listServer(t,
		&surveying{overview: overviewOf("v1.34.1", 3, 3, 40, 39)},
		&surveying{overview: overviewOf("v1.33.0", 2, 1, 20, 18)})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if len(got.Clusters) != 2 {
		t.Fatalf("clusters = %d", len(got.Clusters))
	}
	if got.Clusters[0].Context != "p-mk1" {
		t.Fatalf("the first line was %+v", got.Clusters[0])
	}
}

func TestTheFleetOverviewTotalsWhatEveryClusterReported(t *testing.T) {
	ts := listServer(t,
		&surveying{overview: overviewOf("v1.34.1", 3, 3, 40, 39)},
		&surveying{overview: overviewOf("v1.33.0", 2, 1, 20, 18)})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if got.Nodes.Total != 5 || got.Nodes.Ready != 4 {
		t.Fatalf("nodes = %+v", got.Nodes)
	}
	if got.Pods.Total != 60 || got.Pods.Running != 57 {
		t.Fatalf("pods = %+v", got.Pods)
	}
}

func TestFleetUsageIsUnknownWhenOneClusterHasNoMetrics(t *testing.T) {
	known := overviewOf("v1.34.1", 3, 3, 40, 39)
	known.Nodes.CPUUsedMilli = 500
	known.Nodes.MemAllocatableMi = 2048
	known.Nodes.MemUsedMi = 1024
	known.Nodes.UsageKnown = true
	unknown := overviewOf("v1.33.0", 2, 2, 20, 20)
	unknown.Nodes.MemAllocatableMi = 4096
	ts := listServer(t, &surveying{overview: known}, &surveying{overview: unknown})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if got.Nodes.UsageKnown {
		t.Fatalf("nodes = %+v, want fleet usage to be unknown", got.Nodes)
	}
	if got.Nodes.CPUUsedMilli != 0 || got.Nodes.MemUsedMi != 0 {
		t.Fatalf("nodes = %+v, want no partial fleet usage", got.Nodes)
	}
	if got.Nodes.CPUAllocatableMilli != 2000 || got.Nodes.MemAllocatableMi != 6144 {
		t.Fatalf("nodes = %+v, want capacity from every cluster", got.Nodes)
	}
}

func TestFleetUsageTotalsWhenEveryClusterHasMetrics(t *testing.T) {
	first := overviewOf("v1.34.1", 3, 3, 40, 39)
	first.Nodes.CPUUsedMilli = 500
	first.Nodes.MemAllocatableMi = 2048
	first.Nodes.MemUsedMi = 1024
	first.Nodes.UsageKnown = true
	second := overviewOf("v1.33.0", 2, 2, 20, 20)
	second.Nodes.CPUUsedMilli = 250
	second.Nodes.MemAllocatableMi = 4096
	second.Nodes.MemUsedMi = 2048
	second.Nodes.UsageKnown = true
	ts := listServer(t, &surveying{overview: first}, &surveying{overview: second})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if !got.Nodes.UsageKnown {
		t.Fatalf("nodes = %+v, want fleet usage to be known", got.Nodes)
	}
	if got.Nodes.CPUUsedMilli != 750 || got.Nodes.MemUsedMi != 3072 {
		t.Fatalf("nodes = %+v, want usage from every cluster", got.Nodes)
	}
}

func TestAClusterThatCouldNotCountItsPodsIsNotCountedAsZero(t *testing.T) {
	quiet := overviewOf("v1.33.0", 2, 2, 0, 0)
	quiet.Pods.Known = false
	ts := listServer(t, &surveying{overview: overviewOf("v1.34.1", 3, 3, 40, 39)}, &surveying{overview: quiet})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if got.Pods.Total != 40 {
		t.Fatalf("pods = %+v, want only what was reported", got.Pods)
	}
	if !got.Pods.Known {
		t.Fatal("the total says nothing is known when one cluster answered")
	}
}

func TestAClusterThatCouldNotBeSurveyedSaysWhyOnItsOwnLine(t *testing.T) {
	broken := overviewOf("", 0, 0, 0, 0)
	broken.Error = "the apiserver refused"
	ts := listServer(t, &surveying{overview: overviewOf("v1.34.1", 3, 3, 40, 39)}, &surveying{overview: broken})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if got.Clusters[1].Reason != "the apiserver refused" {
		t.Fatalf("the line read %+v", got.Clusters[1])
	}
	if got.Error != "p-mk2: the apiserver refused" {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestAPanickingClusterIsNamedInTheFleetOverview(t *testing.T) {
	got := mergeOverviews([]clusterAnswer[api.ClusterOverview]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  overviewOf("v1.34.1", 3, 3, 40, 39),
		},
		{
			cluster: "mk2",
			context: "p-mk2",
			failure: "panicked: informer cache changed",
		},
	})

	if got.Clusters[1].Reason != "panicked: informer cache changed" {
		t.Fatalf("cluster = %+v, want the recovered failure", got.Clusters[1])
	}
	if got.Error != "p-mk2: panicked: informer cache changed" {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestFleetOverviewKeepsEveryPodStateAndUniqueCap(t *testing.T) {
	first := overviewOf("v1.34.1", 3, 3, 40, 30)
	first.Pods.Pending = 2
	first.Pods.Failed = 3
	first.Pods.Succeeded = 5
	first.Pods.Capped = []string{"pods", "events"}
	second := overviewOf("v1.33.0", 2, 2, 20, 10)
	second.Pods.Pending = 4
	second.Pods.Failed = 2
	second.Pods.Succeeded = 4
	second.Pods.Capped = []string{"events", "jobs"}

	got := mergeOverviews([]clusterAnswer[api.ClusterOverview]{
		{cluster: "mk1", context: "p-mk1", answer: first},
		{cluster: "mk2", context: "p-mk2", answer: second},
	})

	if got.Pods.Pending != 6 || got.Pods.Failed != 5 || got.Pods.Succeeded != 9 {
		t.Fatalf("pods = %+v", got.Pods)
	}
	wantCaps := []string{"pods", "events", "jobs"}
	if !slices.Equal(got.Pods.Capped, wantCaps) {
		t.Fatalf("caps = %v, want %v", got.Pods.Capped, wantCaps)
	}
}

func TestTheInventoryKeepsThePerClusterSplit(t *testing.T) {
	ts := listServer(t,
		&surveying{counts: api.ResourceCounts{Counts: map[string]int{"/v1/pods": 40}}},
		&surveying{counts: api.ResourceCounts{Counts: map[string]int{"/v1/pods": 20}}})

	var got api.FleetInventory
	readFleet(t, ts, "/api/resources/fleet", &got)

	if len(got.Kinds) != 1 || got.Kinds[0].Total != 60 {
		t.Fatalf("kinds = %+v", got.Kinds)
	}
	if got.Kinds[0].PerCluster[mk1] != 40 || got.Kinds[0].PerCluster[mk2] != 20 {
		t.Fatalf("the split was %+v", got.Kinds[0].PerCluster)
	}
}

func TestTheBiggestKindComesFirst(t *testing.T) {
	ts := listServer(t,
		&surveying{counts: api.ResourceCounts{Counts: map[string]int{"/v1/pods": 40, "/v1/services": 5}}},
		&surveying{counts: api.ResourceCounts{}})

	var got api.FleetInventory
	readFleet(t, ts, "/api/resources/fleet", &got)

	if got.Kinds[0].Key != "/v1/pods" {
		t.Fatalf("the first kind was %q", got.Kinds[0].Key)
	}
}

func TestWhatIsUnwellIsCountedAcrossTheFleet(t *testing.T) {
	ts := listServer(t,
		&surveying{counts: api.ResourceCounts{
			Counts:  map[string]int{"/v1/pods": 40},
			Failing: map[string]int{"/v1/pods": 2},
		}},
		&surveying{counts: api.ResourceCounts{
			Counts:  map[string]int{"/v1/pods": 20},
			Failing: map[string]int{"/v1/pods": 1},
		}})

	var got api.FleetInventory
	readFleet(t, ts, "/api/resources/fleet", &got)

	if got.Kinds[0].Failing != 3 {
		t.Fatalf("failing = %d", got.Kinds[0].Failing)
	}
}

func TestAClusterFailureIsNamedInTheFleetInventory(t *testing.T) {
	got := mergeCounts([]clusterAnswer[api.ResourceCounts]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  api.ResourceCounts{Counts: map[string]int{"/v1/pods": 20}},
		},
		{cluster: "mk2", context: "p-mk2", failure: "panicked: informer cache changed"},
	})

	if !strings.Contains(got.Error, "p-mk2") || !strings.Contains(got.Error, "panicked") {
		t.Fatalf("error = %q, want the cluster and its failure", got.Error)
	}
	if len(got.Kinds) != 1 || got.Kinds[0].Total != 20 {
		t.Fatalf("kinds = %+v, want the cluster that answered preserved", got.Kinds)
	}
}

func TestFleetInventoryErrorsAndEqualTotalsAreStable(t *testing.T) {
	got := mergeCounts([]clusterAnswer[api.ResourceCounts]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer: api.ResourceCounts{
				Counts: map[string]int{
					"/v1/services": 2,
					"/v1/pods":     2,
				},
				Errors: map[string]string{
					"services": "forbidden",
					"pods":     "timed out",
				},
			},
		},
	})

	if len(got.Kinds) != 2 || got.Kinds[0].Key != "/v1/pods" || got.Kinds[1].Key != "/v1/services" {
		t.Fatalf("kinds = %+v, want equal totals ordered by key", got.Kinds)
	}
	if got.Error != "p-mk1 pods: timed out · p-mk1 services: forbidden" {
		t.Fatalf("error = %q", got.Error)
	}
}

func podWith(images ...string) *unstructured.Unstructured {
	containers := make([]any, 0, len(images))
	for _, image := range images {
		containers = append(containers, map[string]any{"name": "app", "image": image})
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "web", "namespace": "default"},
		"spec":       map[string]any{"containers": containers},
	}}
}

func TestAnImageOnTwoClustersSaysBoth(t *testing.T) {
	ts := listServer(t,
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}})

	var got api.FleetImages
	readFleet(t, ts, "/api/images/fleet", &got)

	if len(got.Images) != 1 {
		t.Fatalf("images = %+v", got.Images)
	}
	if len(got.Images[0].Clusters) != 2 || got.Images[0].Pods != 2 {
		t.Fatalf("the image came back as %+v", got.Images[0])
	}
}

func TestTwoTagsOfOneRepoAreMarkedAsSkew(t *testing.T) {
	ts := listServer(t,
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.25")}})

	var got api.FleetImages
	readFleet(t, ts, "/api/images/fleet", &got)

	for _, one := range got.Images {
		if len(one.Skew) != 2 {
			t.Fatalf("skew = %+v on %s", one.Skew, one.Image)
		}
	}
}

func TestOneTagEverywhereIsNotSkew(t *testing.T) {
	ts := listServer(t,
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}})

	var got api.FleetImages
	readFleet(t, ts, "/api/images/fleet", &got)

	if len(got.Images[0].Skew) != 0 {
		t.Fatalf("skew = %+v where every cluster agrees", got.Images[0].Skew)
	}
}

func TestAnInitContainersImageCounts(t *testing.T) {
	pod := podWith("nginx:1.27")
	spec, ok := pod.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("the pod has no spec")
	}
	spec["initContainers"] = []any{map[string]any{"name": "wait", "image": "busybox:1.37"}}
	ts := listServer(t, &surveying{pods: []*unstructured.Unstructured{pod}}, &surveying{})

	var got api.FleetImages
	readFleet(t, ts, "/api/images/fleet", &got)

	if len(got.Images) != 2 {
		t.Fatalf("images = %+v", got.Images)
	}
}

func TestAClusterWhosePodsCouldNotBeReadIsNamed(t *testing.T) {
	ts := listServer(t,
		&surveying{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		&surveying{listErr: errors.New("forbidden")})

	var got api.FleetImages
	readFleet(t, ts, "/api/images/fleet", &got)

	if got.Error != "p-mk2: forbidden" {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestAClusterFailureIsNamedInTheFleetImages(t *testing.T) {
	got := mergeImages([]clusterAnswer[imageAnswer]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  imageAnswer{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		},
		{cluster: "mk2", context: "p-mk2", failure: "panicked: informer cache changed"},
	})

	if !strings.Contains(got.Error, "p-mk2") || !strings.Contains(got.Error, "panicked") {
		t.Fatalf("error = %q, want the cluster and its failure", got.Error)
	}
	if len(got.Images) != 1 {
		t.Fatalf("images = %+v, want the cluster that answered preserved", got.Images)
	}
}

func TestAFleetImageListThatStoppedAtTheCapSaysSo(t *testing.T) {
	images := make([]string, 0, fleetImageCap+1)
	for at := range fleetImageCap + 1 {
		images = append(images, fmt.Sprintf("ghcr.io/acme/app-%04d:1", at))
	}
	got := mergeImages([]clusterAnswer[imageAnswer]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  imageAnswer{pods: []*unstructured.Unstructured{podWith(images...)}},
		},
	})

	if len(got.Images) != fleetImageCap {
		t.Fatalf("images = %d, want the cap of %d", len(got.Images), fleetImageCap)
	}
	if !got.Truncated {
		t.Fatal("a list cut at the cap came back marked complete")
	}
	if got.Total != fleetImageCap+1 {
		t.Fatalf("total = %d, want %d", got.Total, fleetImageCap+1)
	}
}

func TestAFleetImageListInsideTheCapIsNotMarkedCut(t *testing.T) {
	got := mergeImages([]clusterAnswer[imageAnswer]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  imageAnswer{pods: []*unstructured.Unstructured{podWith("nginx:1.27")}},
		},
	})

	if got.Truncated {
		t.Fatal("a list inside the cap was marked cut")
	}
	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
}

func TestFleetImagesAreOrderedByReachThenUseThenName(t *testing.T) {
	got := mergeImages([]clusterAnswer[imageAnswer]{
		{
			cluster: "mk1",
			answer: imageAnswer{
				pods: []*unstructured.Unstructured{
					podWith("z:1", "busy:1", "alpha:1", "beta:1"),
					podWith("busy:1"),
				},
			},
		},
		{
			cluster: "mk2",
			answer:  imageAnswer{pods: []*unstructured.Unstructured{podWith("z:1")}},
		},
	})

	want := []string{"z:1", "busy:1", "alpha:1", "beta:1"}
	if len(got.Images) != len(want) {
		t.Fatalf("images = %+v", got.Images)
	}
	for at, image := range got.Images {
		if image.Image != want[at] {
			t.Fatalf("image %d = %q, want %q", at, image.Image, want[at])
		}
	}
}

func TestMalformedContainerImagesAreIgnored(t *testing.T) {
	pod := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"containers": []any{
				"not a container",
				map[string]any{"image": int64(42)},
				map[string]any{"image": ""},
				map[string]any{"image": "nginx:1.27"},
			},
			"initContainers": map[string]any{"image": "not a list"},
		},
	}}

	got := imagesIn(pod)

	if len(got) != 1 || got[0] != "nginx:1.27" {
		t.Fatalf("images = %v", got)
	}
}

func TestADigestIsNotReadAsATag(t *testing.T) {
	repo, tag := splitImage("ghcr.io/sophotechlabs/spinoza@sha256:abc")

	if repo != "ghcr.io/sophotechlabs/spinoza" || tag != "sha256:abc" {
		t.Fatalf("repo = %q tag = %q", repo, tag)
	}
}

func TestAPortInARegistryHostIsNotReadAsATag(t *testing.T) {
	repo, tag := splitImage("registry:5000/team/app")

	if repo != "registry:5000/team/app" || tag != "" {
		t.Fatalf("repo = %q tag = %q", repo, tag)
	}
}

func TestAnImageWithNoTagIsItsOwnRepo(t *testing.T) {
	repo, tag := splitImage("nginx")

	if repo != "nginx" || tag != "" {
		t.Fatalf("repo = %q tag = %q", repo, tag)
	}
}

func TestSpinozaSaysWhatItIsHolding(t *testing.T) {
	ts := listServer(t, &surveying{}, &surveying{})

	var got api.Memory
	readFleet(t, ts, "/api/memory", &got)

	if got.HeapMi < 0 || got.SysMi <= 0 {
		t.Fatalf("memory = %+v", got)
	}
}

func TestTheWarningColumnSaysHowManyThereWereNotHowManyWereKept(t *testing.T) {
	held := overviewOf("v1.34.1", 3, 3, 40, 39)
	held.Warnings = make([]api.OverviewEvent, 25)
	held.WarningCount = 1219
	ts := listServer(t, &surveying{overview: held}, &surveying{})

	var got api.FleetOverview
	readFleet(t, ts, "/api/overview/fleet", &got)

	if got.Clusters[0].Warnings != 1219 {
		t.Fatalf("warnings = %d, want what the cluster had rather than what fits", got.Clusters[0].Warnings)
	}
}

func TestOneClusterThatNeverAnswersDoesNotHoldTheFleet(t *testing.T) {
	held := &fleet{
		held:     []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active:   mk1,
		backends: map[string]Backend{mk1: &surveying{}},
	}
	srv := New(held, testAssets(), testToken)
	release := make(chan struct{})
	stopped := make(chan struct{})
	found := eachOpenClusterWithin(t.Context(), srv, 20*time.Millisecond, func(
		context.Context,
		api.OpenCluster,
		Backend,
	) string {
		defer close(stopped)
		<-release
		return "late"
	})
	close(release)
	<-stopped

	if len(found) != 1 || !strings.Contains(found[0].failure, "fleet deadline") {
		t.Fatalf("the non-cooperative cluster did not time out: %+v", found)
	}
}
