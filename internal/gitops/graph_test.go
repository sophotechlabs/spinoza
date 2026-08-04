package gitops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var (
	gitRepoGVR = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	bucketGVR  = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "buckets"}
	kustGVR    = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	helmGVR    = schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	appGVR     = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
)

func graphListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		gitRepoGVR: "GitRepositoryList",
		bucketGVR:  "BucketList",
		kustGVR:    "KustomizationList",
		helmGVR:    "HelmReleaseList",
		appGVR:     "ApplicationList",
	}
}

func desc(group, version, resource, kind string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      group,
		Version:    version,
		Resource:   resource,
		Kind:       kind,
		Namespaced: true,
		Category:   "Custom Resources",
	}
}

func graphDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"):   desc("source.toolkit.fluxcd.io", "v1", "gitrepositories", "GitRepository"),
		discovery.Key("source.toolkit.fluxcd.io", "v1", "buckets"):           desc("source.toolkit.fluxcd.io", "v1", "buckets", "Bucket"),
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"): desc("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization"),
		discovery.Key("helm.toolkit.fluxcd.io", "v2", "helmreleases"):        desc("helm.toolkit.fluxcd.io", "v2", "helmreleases", "HelmRelease"),
		discovery.Key("argoproj.io", "v1alpha1", "applications"):             desc("argoproj.io", "v1alpha1", "applications", "Application"),
		discovery.Key("apps", "v1", "deployments"):                           desc("apps", "v1", "deployments", "Deployment"),
	}
}

func gitRepository() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata": map[string]any{
			"name":      "app-repo",
			"namespace": "flux-system",
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}
}

func kustomizationApps() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]any{
			"sourceRef": map[string]any{
				"kind":      "GitRepository",
				"name":      "app-repo",
				"namespace": "flux-system",
			},
			"dependsOn": []any{
				map[string]any{"name": "infra"},
				map[string]any{"name": "db", "namespace": "data"},
				map[string]any{"namespace": "x"},
				"not-a-map",
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
			"inventory": map[string]any{
				"entries": []any{
					map[string]any{"id": "default_web__Service", "v": "v1"},
					map[string]any{"id": "prod_api_apps_Deployment", "v": "v1"},
					map[string]any{"id": "prod_api_apps_Deployment", "v": "v1"},
					map[string]any{"id": "flux-system_app-repo_source.toolkit.fluxcd.io_GitRepository", "v": "v1"},
					map[string]any{"id": "bad-id", "v": "v1"},
					"not-a-map",
				},
			},
		},
	}}
}

func kustomizationOrphan() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":      "orphan",
			"namespace": "flux-system",
		},
	}}
}

func helmRelease() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata": map[string]any{
			"name":      "podinfo",
			"namespace": "flux-system",
		},
		"spec": map[string]any{
			"chart": map[string]any{
				"spec": map[string]any{
					"sourceRef": map[string]any{
						"kind": "HelmRepository",
						"name": "podinfo-charts",
					},
				},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Stalled", "status": "False"},
				map[string]any{"type": "Ready", "status": "False", "reason": "InstallFailed"},
			},
			"inventory": map[string]any{
				"entries": []any{
					map[string]any{"id": "flux-system_podinfo__ConfigMap", "v": "v1"},
				},
			},
		},
	}}
}

func argoApplication() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "guestbook",
			"namespace": "argocd",
		},
		"status": map[string]any{
			"health": map[string]any{"status": "Healthy"},
			"sync":   map[string]any{"status": "Synced"},
			"resources": []any{
				map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "namespace": "prod", "name": "api"},
				map[string]any{"group": "", "version": "v1", "kind": "Service", "namespace": "default", "name": "web"},
				map[string]any{"kind": "ConfigMap", "name": "settings", "namespace": "argocd"},
				map[string]any{"name": "no-kind"},
				map[string]any{"kind": "NoName"},
				"not-a-map",
			},
		},
	}}
}

func newGraphClient(t *testing.T) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		graphListKinds(),
		gitRepository(),
		kustomizationApps(),
		kustomizationOrphan(),
		helmRelease(),
		argoApplication(),
	)
	dyn.PrependReactor("list", "buckets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list buckets failed")
	})
	return dyn
}

func TestBuild(t *testing.T) {
	dyn := newGraphClient(t)
	graph := Build(context.Background(), listerFor(dyn), graphDescs())

	nodesByID := map[string]api.GraphNode{}
	counts := map[string]int{}
	for _, n := range graph.Nodes {
		nodesByID[n.ID] = n
		counts[n.Category]++
	}

	if len(graph.Nodes) != 8 {
		t.Fatalf("nodes = %d, want 8 control-plane nodes", len(graph.Nodes))
	}
	if counts["source"] != 2 {
		t.Fatalf("source nodes = %d, want 2", counts["source"])
	}
	if counts["applier"] != 5 {
		t.Fatalf("applier nodes = %d, want 5 including the two missing dependsOn targets", counts["applier"])
	}
	if counts["app"] != 1 {
		t.Fatalf("app nodes = %d, want 1", counts["app"])
	}
	if counts["managed"] != 0 {
		t.Fatalf("managed nodes = %d; the browser drops them, so they must not cross the wire", counts["managed"])
	}

	wantNodes := []struct {
		id       string
		category string
		kind     string
		status   string
	}{
		{"source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo", "source", "GitRepository", "Ready"},
		{"source.toolkit.fluxcd.io/HelmRepository/flux-system/podinfo-charts", "source", "HelmRepository", ""},
		{"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps", "applier", "Kustomization", "Ready"},
		{"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/orphan", "applier", "Kustomization", ""},
		{"helm.toolkit.fluxcd.io/HelmRelease/flux-system/podinfo", "applier", "HelmRelease", "InstallFailed"},
		{"argoproj.io/Application/argocd/guestbook", "app", "Application", "Healthy Synced"},
	}
	for _, expected := range wantNodes {
		node, ok := nodesByID[expected.id]
		if !ok {
			t.Fatalf("missing node %q", expected.id)
		}
		if node.Category != expected.category {
			t.Fatalf("node %q category = %q, want %q", expected.id, node.Category, expected.category)
		}
		if node.Kind != expected.kind {
			t.Fatalf("node %q kind = %q, want %q", expected.id, node.Kind, expected.kind)
		}
		if node.Status != expected.status {
			t.Fatalf("node %q status = %q, want %q", expected.id, node.Status, expected.status)
		}
	}

	edgeSet := map[string]bool{}
	for _, e := range graph.Edges {
		edgeSet[e.From+"|"+e.To+"|"+e.Kind] = true
	}
	if len(graph.Edges) != 5 {
		t.Fatalf("edges = %d, want 5; an edge to a dropped node has nothing to draw to", len(graph.Edges))
	}
	wantEdges := []string{
		"source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|source",
		"source.toolkit.fluxcd.io/HelmRepository/flux-system/podinfo-charts|helm.toolkit.fluxcd.io/HelmRelease/flux-system/podinfo|source",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/infra|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|dependsOn",
		"kustomize.toolkit.fluxcd.io/Kustomization/data/db|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|dependsOn",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo|manages",
	}
	for _, w := range wantEdges {
		if !edgeSet[w] {
			t.Fatalf("missing edge %q", w)
		}
	}
	for _, dropped := range []string{
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|/Service/default/web|manages",
		"argoproj.io/Application/argocd/guestbook|/ConfigMap/argocd/settings|manages",
	} {
		if edgeSet[dropped] {
			t.Fatalf("edge %q points at a node the browser never renders", dropped)
		}
	}

	for i := 1; i < len(graph.Nodes); i++ {
		if graph.Nodes[i-1].ID > graph.Nodes[i].ID {
			t.Fatalf("nodes not sorted at %d: %q > %q", i, graph.Nodes[i-1].ID, graph.Nodes[i].ID)
		}
	}
	for i := 1; i < len(graph.Edges); i++ {
		prev := graph.Edges[i-1]
		cur := graph.Edges[i]
		if prev.From > cur.From {
			t.Fatalf("edges not sorted by from at %d: %q > %q", i, prev.From, cur.From)
		}
		if prev.From == cur.From && prev.To > cur.To {
			t.Fatalf("edges not sorted by to at %d: %q > %q", i, prev.To, cur.To)
		}
	}
}

func TestBuildEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, graphListKinds())
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): desc("apps", "v1", "deployments", "Deployment"),
	}
	graph := Build(context.Background(), listerFor(dyn), descs)
	if len(graph.Nodes) != 0 {
		t.Fatalf("nodes = %d, want 0", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("edges = %d, want 0", len(graph.Edges))
	}
}

func TestGraphCategory(t *testing.T) {
	cases := []struct {
		group    string
		resource string
		want     string
	}{
		{"source.toolkit.fluxcd.io", "gitrepositories", "source"},
		{"source.toolkit.fluxcd.io", "helmrepositories", "source"},
		{"source.toolkit.fluxcd.io", "ocirepositories", "source"},
		{"source.toolkit.fluxcd.io", "buckets", "source"},
		{"kustomize.toolkit.fluxcd.io", "kustomizations", "applier"},
		{"helm.toolkit.fluxcd.io", "helmreleases", "applier"},
		{"argoproj.io", "applications", "app"},
		{"apps", "deployments", ""},
		{"source.toolkit.fluxcd.io", "receivers", ""},
		{"kustomize.toolkit.fluxcd.io", "other", ""},
		{"helm.toolkit.fluxcd.io", "other", ""},
		{"argoproj.io", "appprojects", ""},
	}
	for _, c := range cases {
		got := graphCategory(desc(c.group, "v1", c.resource, "X"))
		if got != c.want {
			t.Fatalf("graphCategory(%s/%s) = %q, want %q", c.group, c.resource, got, c.want)
		}
	}
}

func TestParseInventoryID(t *testing.T) {
	cases := []struct {
		raw       string
		namespace string
		name      string
		group     string
		kind      string
	}{
		{"prod_api_apps_Deployment", "prod", "api", "apps", "Deployment"},
		{"default_web__Service", "default", "web", "", "Service"},
		{"bad-id", "", "", "", ""},
		{"a_b_c", "", "", "", ""},
		{"a_b_c_d_e", "", "", "", ""},
	}
	for _, c := range cases {
		ns, name, group, kind := parseInventoryID(c.raw)
		if ns != c.namespace || name != c.name || group != c.group || kind != c.kind {
			t.Fatalf("parseInventoryID(%q) = %q,%q,%q,%q want %q,%q,%q,%q", c.raw, ns, name, group, kind, c.namespace, c.name, c.group, c.kind)
		}
	}
}

func TestNodeID(t *testing.T) {
	got := nodeID("apps", "Deployment", "prod", "api")
	if got != "apps/Deployment/prod/api" {
		t.Fatalf("nodeID = %q, want apps/Deployment/prod/api", got)
	}
}

func conditionsObject(conditions []any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"conditions": conditions,
		},
	}}
}

func TestConditionSummary(t *testing.T) {
	cases := []struct {
		name       string
		conditions []any
		want       string
	}{
		{
			name:       "ready",
			conditions: []any{map[string]any{"type": "Ready", "status": "True"}},
			want:       "Ready",
		},
		{
			name:       "reason",
			conditions: []any{map[string]any{"type": "Ready", "status": "False", "reason": "InstallFailed"}},
			want:       "InstallFailed",
		},
		{
			name:       "notReady",
			conditions: []any{map[string]any{"type": "Ready", "status": "False"}},
			want:       "NotReady",
		},
		{
			name:       "onlyOtherType",
			conditions: []any{map[string]any{"type": "Stalled", "status": "True"}},
			want:       "",
		},
		{
			name: "skipsNonMap",
			conditions: []any{
				"not-a-map",
				map[string]any{"type": "Ready", "status": "True"},
			},
			want: "Ready",
		},
		{
			name:       "noConditions",
			conditions: []any{},
			want:       "",
		},
	}
	for _, c := range cases {
		got := unstr.ReadySummary(conditionsObject(c.conditions))
		if got != c.want {
			t.Fatalf("%s: conditionSummary = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStatusOf(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"health": map[string]any{"status": "Degraded"},
			"sync":   map[string]any{"status": "OutOfSync"},
		},
	}}
	if got := statusOf(app, "app"); got != "Degraded OutOfSync" {
		t.Fatalf("statusOf app = %q, want Degraded OutOfSync", got)
	}

	empty := &unstructured.Unstructured{Object: map[string]any{}}
	if got := statusOf(empty, "app"); got != "" {
		t.Fatalf("statusOf empty app = %q, want empty", got)
	}

	applier := conditionsObject([]any{map[string]any{"type": "Ready", "status": "True"}})
	if got := statusOf(applier, "applier"); got != "Ready" {
		t.Fatalf("statusOf applier = %q, want Ready", got)
	}
}

func TestBuildReportsAListThatFailed(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	if graph.Error == "" {
		t.Fatal("a failed list was reported as a graph with nothing in it")
	}
	if !strings.Contains(graph.Error, "buckets") {
		t.Fatalf("error = %q, want it to name the resource", graph.Error)
	}
	if !strings.Contains(graph.Error, "(list buckets failed)") {
		t.Fatalf("error = %q, want the reason", graph.Error)
	}
}

func TestBuildStillReturnsTheNodesItCouldRead(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	if len(graph.Nodes) == 0 {
		t.Fatal("one failing list threw away every node")
	}
}

func TestBuildIsSilentWhenEveryListWorks(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		graphListKinds(),
		gitRepository(),
		kustomizationApps(),
	)

	graph := Build(context.Background(), listerFor(dyn), graphDescs())

	if graph.Error != "" {
		t.Fatalf("error = %q, want none", graph.Error)
	}
}

func nodeByID(graph api.Graph, id string) (api.GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return api.GraphNode{}, false
}

func TestADanglingDependsOnIsDrawnAsMissing(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	node, found := nodeByID(graph, "kustomize.toolkit.fluxcd.io/Kustomization/flux-system/infra")
	if !found {
		t.Fatal("a dependsOn pointing at a Kustomization that is not there vanished from the graph")
	}
	if node.Status != "NotFound" {
		t.Fatalf("status = %q, want NotFound so it is not confused with an unhealthy one", node.Status)
	}
	if node.Category != "applier" {
		t.Fatalf("category = %q; managed nodes are dropped by the frontend filter", node.Category)
	}
}

func TestADependsOnInAnotherNamespaceKeepsThatNamespace(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	node, found := nodeByID(graph, "kustomize.toolkit.fluxcd.io/Kustomization/data/db")
	if !found {
		t.Fatal("a cross-namespace dependsOn vanished")
	}
	if node.Namespace != "data" {
		t.Fatalf("namespace = %q", node.Namespace)
	}
}

func TestARealDependencyKeepsItsOwnStatus(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	node, found := nodeByID(graph, "kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps")
	if !found {
		t.Fatal("the real Kustomization is missing")
	}
	if node.Status == "NotFound" {
		t.Fatal("a Kustomization that exists was marked NotFound")
	}
}

func chartOnlyRelease(namespace, sourceNamespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "cross", "namespace": namespace},
		"spec": map[string]any{
			"chart": map[string]any{
				"spec": map[string]any{
					"chart": "podinfo",
					"sourceRef": map[string]any{
						"kind":      "HelmRepository",
						"name":      "charts",
						"namespace": sourceNamespace,
					},
				},
			},
		},
	}}
}

func graphWith(t *testing.T, objs ...*unstructured.Unstructured) api.Graph {
	t.Helper()
	runtimeObjs := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		runtimeObjs = append(runtimeObjs, obj)
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), graphListKinds(), runtimeObjs...)
	return Build(context.Background(), listerFor(dyn), graphDescs())
}

func TestAChartSourceRefKeepsItsOwnNamespace(t *testing.T) {
	graph := graphWith(t, chartOnlyRelease("apps", "flux-system"))

	_, phantom := nodeByID(graph, "source.toolkit.fluxcd.io/HelmRepository/apps/charts")
	if phantom {
		t.Fatal("the chart's sourceRef namespace was ignored, inventing a repo in the HelmRelease's namespace")
	}
	_, present := nodeByID(graph, "source.toolkit.fluxcd.io/HelmRepository/flux-system/charts")
	if !present {
		t.Fatal("the repo named by spec.chart.spec.sourceRef is missing from the graph")
	}
}

func TestAChartSourceRefWithoutANamespaceFallsBackToTheRelease(t *testing.T) {
	graph := graphWith(t, chartOnlyRelease("apps", ""))

	_, found := nodeByID(graph, "source.toolkit.fluxcd.io/HelmRepository/apps/charts")
	if !found {
		t.Fatal("an omitted sourceRef namespace should default to the HelmRelease's own namespace")
	}
}

func ociRelease() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "oci-app", "namespace": "apps"},
		"spec": map[string]any{
			"chartRef": map[string]any{
				"kind":      "OCIRepository",
				"name":      "podinfo-oci",
				"namespace": "flux-system",
			},
		},
	}}
}

func TestAChartRefIsGraphed(t *testing.T) {
	graph := graphWith(t, ociRelease())

	_, found := nodeByID(graph, "source.toolkit.fluxcd.io/OCIRepository/flux-system/podinfo-oci")
	if !found {
		t.Fatal("spec.chartRef is not graphed, so an OCI-sourced HelmRelease has no source edge")
	}
}

func TestAnInventoryEntryNamingAFluxKindSurvives(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	node, found := nodeByID(graph, "source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo")
	if !found {
		t.Fatal("a GitRepository listed in a Kustomization inventory was dropped as managed")
	}
	if node.Category == categoryManaged {
		t.Fatalf("category = %q; addObject should have overwritten the inventory placeholder", node.Category)
	}
}

func TestWorkloadInventoryNodesDoNotCrossTheWire(t *testing.T) {
	graph := Build(context.Background(), listerFor(newGraphClient(t)), graphDescs())

	for _, node := range graph.Nodes {
		if node.Category == categoryManaged {
			t.Fatalf("node %q is managed; the browser filters these out, so sending them is waste", node.ID)
		}
	}
}

func TestFluxGeneratedHelmChartsAreNotGraphed(t *testing.T) {
	chart := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "HelmChart",
		"metadata":   map[string]any{"name": "beyla-beyla", "namespace": "flux-system"},
	}}
	graph := graphWith(t, chart)

	_, found := nodeByID(graph, "source.toolkit.fluxcd.io/HelmChart/flux-system/beyla-beyla")
	if found {
		t.Fatal("flux generates one HelmChart per HelmRelease; graphing them buries the topology")
	}
}

func readyOfNamed(t *testing.T, objs []*unstructured.Unstructured, name string) api.GraphNode {
	t.Helper()
	runtimeObjs := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		runtimeObjs = append(runtimeObjs, obj)
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), graphListKinds(), runtimeObjs...)
	graph := Build(context.Background(), listerFor(dyn), graphDescs())
	for _, node := range graph.Nodes {
		if node.Name == name {
			return node
		}
	}
	t.Fatalf("no node named %q in %+v", name, graph.Nodes)
	return api.GraphNode{}
}

func releaseWithReady(status, reason string) *unstructured.Unstructured {
	release := helmRelease()
	conditions := []any{map[string]any{"type": "Ready", "status": status, "reason": reason}}
	if status == "" {
		conditions = []any{}
	}
	release.Object["status"] = map[string]any{"conditions": conditions}
	return release
}

func TestAFailedObjectIsMarkedNotReadyNotJustNamed(t *testing.T) {
	node := readyOfNamed(t, []*unstructured.Unstructured{releaseWithReady("False", "InstallFailed")}, "podinfo")

	if node.Ready != "False" {
		t.Fatalf("ready = %q, want False; the reason alone never tells the browser it failed", node.Ready)
	}
	if node.Status != "InstallFailed" {
		t.Fatalf("status = %q, want the reason kept for the reader", node.Status)
	}
}

func TestAReadyObjectIsMarkedReady(t *testing.T) {
	node := readyOfNamed(t, []*unstructured.Unstructured{releaseWithReady("True", "InstallSucceeded")}, "podinfo")

	if node.Ready != "True" {
		t.Fatalf("ready = %q, want True", node.Ready)
	}
}

func TestAReconcilingObjectIsMarkedUnknown(t *testing.T) {
	node := readyOfNamed(t, []*unstructured.Unstructured{releaseWithReady("Unknown", "Progressing")}, "podinfo")

	if node.Ready != "Unknown" {
		t.Fatalf("ready = %q, want Unknown; a reconcile in flight is not a failure", node.Ready)
	}
}

func TestAnObjectWithNoConditionsIsMarkedUnknown(t *testing.T) {
	node := readyOfNamed(t, []*unstructured.Unstructured{releaseWithReady("", "")}, "podinfo")

	if node.Ready != "Unknown" {
		t.Fatalf("ready = %q, want Unknown", node.Ready)
	}
}
