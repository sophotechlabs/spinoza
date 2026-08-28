package topology

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func build(t *testing.T, req Request) api.Graph {
	t.Helper()
	return Build(context.Background(), listerFor(newClient()), descs(), req)
}

func nodesByID(graph api.Graph) map[string]api.GraphNode {
	out := map[string]api.GraphNode{}
	for _, node := range graph.Nodes {
		out[node.ID] = node
	}
	return out
}

func edgeSet(graph api.Graph) map[string]bool {
	out := map[string]bool{}
	for _, edge := range graph.Edges {
		out[edge.From+"|"+edge.To+"|"+edge.Kind] = true
	}
	return out
}

func TestTheFoldPutsAWholeDeploymentOnOneNode(t *testing.T) {
	graph := build(t, Request{})
	nodes := nodesByID(graph)

	folded, ok := nodes["dep-api"]
	if !ok {
		t.Fatalf("the Deployment is missing from %v", nodes)
	}
	if folded.Contains != 3 {
		t.Fatalf("the Deployment folded %d objects, want 3 (one ReplicaSet, two pods)", folded.Contains)
	}
	if folded.Unhealthy != 1 {
		t.Fatalf("the Deployment reports %d unhealthy inside, want 1", folded.Unhealthy)
	}
	if folded.Ready != readyFalse {
		t.Fatalf("the Deployment reads %q, want False", folded.Ready)
	}
	for _, hidden := range []string{"rs-api", "pod-1", "pod-2"} {
		if _, shown := nodes[hidden]; shown {
			t.Fatalf("%q is still its own node", hidden)
		}
	}
}

func TestACompletedJobPodIsNotABrokenOne(t *testing.T) {
	nodes := nodesByID(build(t, Request{}))

	nightly, ok := nodes["cj-nightly"]
	if !ok {
		t.Fatal("the CronJob is missing")
	}
	if nightly.Contains != 2 {
		t.Fatalf("the CronJob folded %d objects, want 2 (one Job, one pod)", nightly.Contains)
	}
	if nightly.Unhealthy != 0 {
		t.Fatalf("the CronJob reports %d unhealthy inside, want 0", nightly.Unhealthy)
	}
}

func TestAnObjectWithNoUIDIsNotDrawn(t *testing.T) {
	for _, node := range build(t, Request{}).Nodes {
		if node.Name == "no-uid" {
			t.Fatal("an object with no uid became a node; the whole graph is keyed on uid")
		}
	}
}

func TestAnOwnerReferenceWithNoUIDOwnsNothing(t *testing.T) {
	graph := build(t, Request{})
	nodes := nodesByID(graph)

	orphan, ok := nodes["pod-10"]
	if !ok {
		t.Fatal("a pod whose owner reference carries no uid was dropped instead of drawn on its own")
	}
	if orphan.Contains != 0 {
		t.Fatalf("the pod folded %d objects", orphan.Contains)
	}
	for key := range edgeSet(graph) {
		if strings.HasSuffix(key, "|pod-10|owns") {
			t.Fatalf("an owns edge was drawn from an owner nothing identifies: %q", key)
		}
	}
}

func TestAnAutoscalerThatNamesNoTargetScalesNothing(t *testing.T) {
	for key := range edgeSet(build(t, Request{})) {
		if strings.HasPrefix(key, "hpa-aimless|") {
			t.Fatalf("an autoscaler with no target name grew an edge: %q", key)
		}
	}
}

func TestAContainerThatIsNotAMapConfiguresNothing(t *testing.T) {
	graph := build(t, Request{})

	if _, ok := nodesByID(graph)["job-3"]; !ok {
		t.Fatal("a Job whose container list holds junk was dropped instead of drawn")
	}
	for key := range edgeSet(graph) {
		if strings.HasSuffix(key, "|job-3|configures") {
			t.Fatalf("junk in a container list produced a config edge: %q", key)
		}
	}
}

func TestAPodItsOwnerDoesNotControlStaysOnTheGraph(t *testing.T) {
	graph := build(t, Request{})
	nodes := nodesByID(graph)

	if _, ok := nodes["pod-9"]; !ok {
		t.Fatal("a pod folded into an owner that does not control it")
	}
	if !edgeSet(graph)["dep-api|pod-9|owns"] {
		t.Fatal("the owns edge did not move to the node its folded owner sits in")
	}
}

func TestAPodNobodyOwnsStaysOnTheGraph(t *testing.T) {
	nodes := nodesByID(build(t, Request{}))

	debug, ok := nodes["pod-4"]
	if !ok {
		t.Fatal("the bare pod is missing")
	}
	if debug.Contains != 0 {
		t.Fatalf("the bare pod folded %d objects, want 0", debug.Contains)
	}
}

func TestAnOwnerNobodyListedStillGetsANode(t *testing.T) {
	nodes := nodesByID(build(t, Request{}))

	rollout, ok := nodes["rollout-1"]
	if !ok {
		t.Fatal("the Rollout is missing")
	}
	if rollout.Kind != "Rollout" {
		t.Fatalf("the owner is a %q, want Rollout", rollout.Kind)
	}
	if rollout.Resource != "rollouts" {
		t.Fatalf("the owner resolved to resource %q, want rollouts", rollout.Resource)
	}
	if rollout.Contains != 1 {
		t.Fatalf("the Rollout folded %d pods, want 1", rollout.Contains)
	}
}

func TestAClusterScopedOwnerKeepsNoNamespace(t *testing.T) {
	nodes := nodesByID(build(t, Request{}))

	fleet, ok := nodes["fleet-west"]
	if !ok {
		t.Fatal("the cluster-scoped owner is missing")
	}
	if fleet.Namespace != "" {
		t.Fatalf("the cluster-scoped owner landed in namespace %q", fleet.Namespace)
	}
}

func TestAnExpandRequestIgnoresAnEmptyID(t *testing.T) {
	nodes := nodesByID(build(t, Request{Expanded: []string{"", "dep-api"}}))

	if _, ok := nodes["rs-api"]; !ok {
		t.Fatal("an empty entry in the expand list swallowed the real one")
	}
}

func TestAServicePointsAtTheWorkloadItsPodsFoldInto(t *testing.T) {
	edges := edgeSet(build(t, Request{}))

	if !edges["svc-api|dep-api|routes"] {
		t.Fatalf("no routes edge from the Service to the Deployment: %v", edges)
	}
	if edges["svc-api|pod-1|routes"] {
		t.Fatal("the Service still points at a folded pod")
	}
}

func TestAServiceOnlyReachesPodsCarryingEveryPairItNames(t *testing.T) {
	graph := build(t, Request{})
	edges := edgeSet(graph)

	if !edges["svc-api|dep-api|routes"] {
		t.Fatal("the Service does not reach the workload whose pods carry its label")
	}
	for key := range edges {
		if strings.HasPrefix(key, "svc-api|cj-nightly") {
			t.Fatalf("the Service reached a workload whose pods carry no matching label: %q", key)
		}
		if strings.HasPrefix(key, "svc-api|pod-4") {
			t.Fatalf("the Service reached an unlabelled bare pod: %q", key)
		}
	}
}

func TestAServiceNamingALabelNoPodCarriesReachesNothing(t *testing.T) {
	for key := range edgeSet(build(t, Request{})) {
		if strings.HasPrefix(key, "svc-nomatch|") {
			t.Fatalf("a Service whose selector matches no pod grew an edge: %q", key)
		}
	}
}

func TestAServiceWithNoSelectorRoutesNowhere(t *testing.T) {
	for key := range edgeSet(build(t, Request{})) {
		if strings.HasPrefix(key, "svc-external|") {
			t.Fatalf("the ExternalName Service grew an edge: %q", key)
		}
	}
}

func TestAnIngressNamesTheServiceThatIsNotThere(t *testing.T) {
	graph := build(t, Request{})
	edges := edgeSet(graph)
	nodes := nodesByID(graph)

	if !edges["ing-web|svc-api|routes"] {
		t.Fatal("the Ingress does not reach the Service it names")
	}
	missing := "/Service/prod/gone"
	if !edges["ing-web|"+missing+"|routes"] {
		t.Fatal("the Ingress backend that is not there was dropped")
	}
	if nodes[missing].Ready != readyFalse {
		t.Fatalf("the missing Service reads %q, want False", nodes[missing].Ready)
	}
	if nodes[missing].Status != statusMissing {
		t.Fatalf("the missing Service says %q, want %q", nodes[missing].Status, statusMissing)
	}
}

func TestTheAutoscalerPointsAtWhatItScales(t *testing.T) {
	if !edgeSet(build(t, Request{}))["hpa-api|dep-api|scales"] {
		t.Fatal("no scales edge from the HorizontalPodAutoscaler to its target")
	}
}

func TestTheConfigAWorkloadMountsBecomesAnEdge(t *testing.T) {
	edges := edgeSet(build(t, Request{}))

	for _, want := range []string{
		"/ConfigMap/prod/api-config|dep-api|configures",
		"/Secret/prod/api-tls|dep-api|configures",
		"/Secret/prod/registry|dep-api|configures",
	} {
		if !edges[want] {
			t.Fatalf("missing edge %q", want)
		}
	}
}

func TestTheVolumeTheKubeletInjectsIsNotAnEdge(t *testing.T) {
	graph := build(t, Request{})
	nodes := nodesByID(graph)

	if _, drawn := nodes["/ConfigMap/prod/kube-root-ca.crt"]; drawn {
		t.Fatal("the ConfigMap from the injected token volume became a node")
	}
	for _, want := range []string{
		"/ConfigMap/prod/agent-config|ds-agent|configures",
		"/Secret/prod/agent-token|ds-agent|configures",
	} {
		if !edgeSet(graph)[want] {
			t.Fatalf("missing edge %q", want)
		}
	}
}

func TestExpandingADeploymentShowsTheReplicaSet(t *testing.T) {
	graph := build(t, Request{Expanded: []string{"dep-api"}})
	nodes := nodesByID(graph)

	replicas, ok := nodes["rs-api"]
	if !ok {
		t.Fatal("the ReplicaSet is still folded after its Deployment was expanded")
	}
	if replicas.Contains != 2 {
		t.Fatalf("the ReplicaSet folded %d pods, want 2", replicas.Contains)
	}
	if nodes["dep-api"].Contains != 0 {
		t.Fatalf("the expanded Deployment still claims %d hidden objects", nodes["dep-api"].Contains)
	}
	if !edgeSet(graph)["dep-api|rs-api|owns"] {
		t.Fatal("the owns edge between the Deployment and its ReplicaSet is missing")
	}
}

func TestExpandingTwiceReachesThePods(t *testing.T) {
	nodes := nodesByID(build(t, Request{Expanded: []string{"dep-api", "rs-api"}}))

	for _, id := range []string{"pod-1", "pod-2"} {
		if _, ok := nodes[id]; !ok {
			t.Fatalf("pod %q is still hidden with both owners expanded", id)
		}
	}
}

func TestANamespaceScopeLeavesTheRestOut(t *testing.T) {
	nodes := nodesByID(build(t, Request{Namespace: "other"}))

	if _, ok := nodes["dep-web"]; !ok {
		t.Fatal("the Deployment in the chosen namespace is missing")
	}
	if _, ok := nodes["dep-api"]; ok {
		t.Fatal("a Deployment from another namespace crossed the scope")
	}
}

func TestARootShowsWhatItTouches(t *testing.T) {
	graph := build(t, Request{Root: api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "api",
	}})
	nodes := nodesByID(graph)

	for _, want := range []string{"dep-api", "svc-api", "hpa-api", "/ConfigMap/prod/api-config", "ing-web"} {
		if _, ok := nodes[want]; !ok {
			t.Fatalf("%q is not in the neighborhood: %v", want, nodes)
		}
	}
	if _, ok := nodes["cj-nightly"]; ok {
		t.Fatal("the CronJob is unrelated to the root")
	}
	if _, ok := nodes["dep-web"]; ok {
		t.Fatal("a Deployment in another namespace reached the neighborhood")
	}
}

func TestARootNothingMatchesDrawsNothing(t *testing.T) {
	graph := build(t, Request{Root: api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "absent",
	}})
	if len(graph.Nodes) != 0 {
		t.Fatalf("a root that is not in the cluster drew %d nodes", len(graph.Nodes))
	}
}

func TestARootOfAnUnknownKindDrawsNothing(t *testing.T) {
	graph := build(t, Request{Root: api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "widgets",
		Namespace: "prod",
		Name:      "api",
	}})
	if len(graph.Nodes) != 0 {
		t.Fatalf("a root of a kind discovery never reported drew %d nodes", len(graph.Nodes))
	}
}

func TestARootThatIsFoldedResolvesToWhatSwallowedIt(t *testing.T) {
	graph := build(t, Request{Root: api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "replicasets",
		Namespace: "prod",
		Name:      "api-abc",
	}})
	if _, ok := nodesByID(graph)["dep-api"]; !ok {
		t.Fatal("a folded ReplicaSet root did not land on its Deployment")
	}
}

func TestAResourceTypeThatWillNotListIsReported(t *testing.T) {
	graph := build(t, Request{})

	want := "1 of 11 resource types could not be listed: statefulsets.apps (list statefulsets failed)"
	if graph.Error != want {
		t.Fatalf("error = %q, want %q", graph.Error, want)
	}
	if len(graph.Nodes) == 0 {
		t.Fatal("one resource type failing emptied the whole graph")
	}
}

func TestPodsBeingForbiddenLeavesTheWorkloadsStanding(t *testing.T) {
	dyn := newClient()
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})
	graph := Build(context.Background(), listerFor(dyn), descs(), Request{})
	nodes := nodesByID(graph)

	if !strings.Contains(graph.Error, "pods (pods is forbidden)") {
		t.Fatalf("the refused list is not reported: %q", graph.Error)
	}
	folded, ok := nodes["dep-api"]
	if !ok {
		t.Fatal("the Deployment went missing when its pods could not be read")
	}
	if folded.Contains != 1 {
		t.Fatalf("the Deployment folded %d objects, want just the ReplicaSet it can still see", folded.Contains)
	}
	if _, drawn := nodes["svc-api"]; !drawn {
		t.Fatal("the Service went missing when pods could not be read")
	}
	for key := range edgeSet(graph) {
		if strings.HasPrefix(key, "svc-api|") {
			t.Fatalf("a Service routed somewhere with no pods to match: %q", key)
		}
	}
}

func TestTheOutputIsSorted(t *testing.T) {
	graph := build(t, Request{})

	for i := 1; i < len(graph.Nodes); i++ {
		if graph.Nodes[i-1].ID > graph.Nodes[i].ID {
			t.Fatalf("nodes not sorted at %d: %q > %q", i, graph.Nodes[i-1].ID, graph.Nodes[i].ID)
		}
	}
	for i := 1; i < len(graph.Edges); i++ {
		previous := graph.Edges[i-1]
		current := graph.Edges[i]
		if previous.From > current.From {
			t.Fatalf("edges not sorted at %d: %q > %q", i, previous.From, current.From)
		}
		if previous.From == current.From && previous.To > current.To {
			t.Fatalf("edges not sorted at %d: %q > %q", i, previous.To, current.To)
		}
	}
}

func TestTheBuilderWarmsExactlyWhatItLists(t *testing.T) {
	spy := &warmSpy{dynLister: listerFor(newClient())}

	Build(context.Background(), spy, descs(), Request{})

	want := map[string]bool{
		"/v1/pods":                                true,
		"/v1/services":                            true,
		"/v1/replicationcontrollers":              true,
		"apps/v1/deployments":                     true,
		"apps/v1/replicasets":                     true,
		"apps/v1/statefulsets":                    true,
		"apps/v1/daemonsets":                      true,
		"batch/v1/jobs":                           true,
		"batch/v1/cronjobs":                       true,
		"networking.k8s.io/v1/ingresses":          true,
		"autoscaling/v2/horizontalpodautoscalers": true,
	}
	if len(spy.warmed) != len(want) {
		t.Fatalf("warmed %v, want the %d types the view reads", spy.warmed, len(want))
	}
	for _, key := range spy.warmed {
		if !want[key] {
			t.Fatalf("warmed %q, which this view never reads: every warm pins a cluster-wide informer", key)
		}
	}
	for key := range want {
		if !slices.Contains(spy.warmed, key) {
			t.Fatalf("%q was never warmed, so its first read waits on a cold cache", key)
		}
	}
}

func TestConfigMapsAndSecretsAreNeverListed(t *testing.T) {
	spy := &warmSpy{dynLister: listerFor(newClient())}

	Build(context.Background(), spy, descs(), Request{})

	for _, key := range spy.warmed {
		if strings.HasSuffix(key, "/configmaps") || strings.HasSuffix(key, "/secrets") {
			t.Fatalf("%q was warmed; opening this view would hold every secret in the cluster in memory", key)
		}
	}
}

func labelled(name, uid string, labels map[string]any) *unstructured.Unstructured {
	holder := meta(name, "prod", uid)
	holder["labels"] = labels
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   holder,
		"status": map[string]any{
			"phase":      "Running",
			"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
		},
	}}
}

func selecting(pairs map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   meta("picky", "prod", "svc-picky"),
		"spec":       map[string]any{"selector": pairs},
	}}
}

func TestASelectorNarrowsOnItsRarestLabelThenChecksTheRest(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		listKinds(),
		selecting(map[string]any{"app": "api", "role": "primary"}),
		labelled("both", "pod-both", map[string]any{"app": "api", "role": "primary"}),
		labelled("role-only", "pod-role", map[string]any{"role": "primary"}),
		labelled("app-only-a", "pod-app-a", map[string]any{"app": "api"}),
		labelled("app-only-b", "pod-app-b", map[string]any{"app": "api"}),
	)

	graph := Build(context.Background(), listerFor(dyn), descs(), Request{})
	edges := edgeSet(graph)

	if !edges["svc-picky|pod-both|routes"] {
		t.Fatal("the Service does not reach the one pod carrying every pair it names")
	}
	for _, missing := range []string{"pod-role", "pod-app-a", "pod-app-b"} {
		if edges["svc-picky|"+missing+"|routes"] {
			t.Fatalf("the Service reached %s, which carries only part of its selector", missing)
		}
	}
}
