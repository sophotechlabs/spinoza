package gitops

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
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
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata": map[string]interface{}{
			"name":      "app-repo",
			"namespace": "flux-system",
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
}

func kustomizationApps() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]interface{}{
			"sourceRef": map[string]interface{}{
				"kind":      "GitRepository",
				"name":      "app-repo",
				"namespace": "flux-system",
			},
			"dependsOn": []interface{}{
				map[string]interface{}{"name": "infra"},
				map[string]interface{}{"name": "db", "namespace": "data"},
				map[string]interface{}{"namespace": "x"},
				"not-a-map",
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
			"inventory": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"id": "default_web__Service", "v": "v1"},
					map[string]interface{}{"id": "prod_api_apps_Deployment", "v": "v1"},
					map[string]interface{}{"id": "prod_api_apps_Deployment", "v": "v1"},
					map[string]interface{}{"id": "flux-system_app-repo_source.toolkit.fluxcd.io_GitRepository", "v": "v1"},
					map[string]interface{}{"id": "bad-id", "v": "v1"},
					"not-a-map",
				},
			},
		},
	}}
}

func kustomizationOrphan() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "orphan",
			"namespace": "flux-system",
		},
	}}
}

func helmRelease() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata": map[string]interface{}{
			"name":      "podinfo",
			"namespace": "flux-system",
		},
		"spec": map[string]interface{}{
			"chart": map[string]interface{}{
				"spec": map[string]interface{}{
					"sourceRef": map[string]interface{}{
						"kind": "HelmRepository",
						"name": "podinfo-charts",
					},
				},
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Stalled", "status": "False"},
				map[string]interface{}{"type": "Ready", "status": "False", "reason": "InstallFailed"},
			},
			"inventory": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"id": "flux-system_podinfo__ConfigMap", "v": "v1"},
				},
			},
		},
	}}
}

func argoApplication() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "guestbook",
			"namespace": "argocd",
		},
		"status": map[string]interface{}{
			"health": map[string]interface{}{"status": "Healthy"},
			"sync":   map[string]interface{}{"status": "Synced"},
			"resources": []interface{}{
				map[string]interface{}{"group": "apps", "version": "v1", "kind": "Deployment", "namespace": "prod", "name": "api"},
				map[string]interface{}{"group": "", "version": "v1", "kind": "Service", "namespace": "default", "name": "web"},
				map[string]interface{}{"kind": "ConfigMap", "name": "settings", "namespace": "argocd"},
				map[string]interface{}{"name": "no-kind"},
				map[string]interface{}{"kind": "NoName"},
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
	graph := Build(context.Background(), dyn, graphDescs())

	nodesByID := map[string]api.GraphNode{}
	counts := map[string]int{}
	for _, n := range graph.Nodes {
		nodesByID[n.ID] = n
		counts[n.Category]++
	}

	if len(graph.Nodes) != 10 {
		t.Fatalf("nodes = %d, want 10", len(graph.Nodes))
	}
	if counts["source"] != 2 {
		t.Fatalf("source nodes = %d, want 2", counts["source"])
	}
	if counts["applier"] != 3 {
		t.Fatalf("applier nodes = %d, want 3", counts["applier"])
	}
	if counts["app"] != 1 {
		t.Fatalf("app nodes = %d, want 1", counts["app"])
	}
	if counts["managed"] != 4 {
		t.Fatalf("managed nodes = %d, want 4", counts["managed"])
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
		{"/Service/default/web", "managed", "Service", ""},
		{"apps/Deployment/prod/api", "managed", "Deployment", ""},
		{"/ConfigMap/flux-system/podinfo", "managed", "ConfigMap", ""},
		{"/ConfigMap/argocd/settings", "managed", "ConfigMap", ""},
	}
	for _, w := range wantNodes {
		n, ok := nodesByID[w.id]
		if !ok {
			t.Fatalf("missing node %q", w.id)
		}
		if n.Category != w.category {
			t.Fatalf("node %q category = %q, want %q", w.id, n.Category, w.category)
		}
		if n.Kind != w.kind {
			t.Fatalf("node %q kind = %q, want %q", w.id, n.Kind, w.kind)
		}
		if n.Status != w.status {
			t.Fatalf("node %q status = %q, want %q", w.id, n.Status, w.status)
		}
	}

	edgeSet := map[string]bool{}
	for _, e := range graph.Edges {
		edgeSet[e.From+"|"+e.To+"|"+e.Kind] = true
	}
	if len(graph.Edges) != 11 {
		t.Fatalf("edges = %d, want 11", len(graph.Edges))
	}
	wantEdges := []string{
		"source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|source",
		"source.toolkit.fluxcd.io/HelmRepository/flux-system/podinfo-charts|helm.toolkit.fluxcd.io/HelmRelease/flux-system/podinfo|source",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/infra|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|dependsOn",
		"kustomize.toolkit.fluxcd.io/Kustomization/data/db|kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|dependsOn",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|/Service/default/web|manages",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|apps/Deployment/prod/api|manages",
		"kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps|source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo|manages",
		"helm.toolkit.fluxcd.io/HelmRelease/flux-system/podinfo|/ConfigMap/flux-system/podinfo|manages",
		"argoproj.io/Application/argocd/guestbook|apps/Deployment/prod/api|manages",
		"argoproj.io/Application/argocd/guestbook|/Service/default/web|manages",
		"argoproj.io/Application/argocd/guestbook|/ConfigMap/argocd/settings|manages",
	}
	for _, w := range wantEdges {
		if !edgeSet[w] {
			t.Fatalf("missing edge %q", w)
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
	graph := Build(context.Background(), dyn, descs)
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

func conditionsObject(conditions []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": conditions,
		},
	}}
}

func TestConditionSummary(t *testing.T) {
	cases := []struct {
		name       string
		conditions []interface{}
		want       string
	}{
		{
			name:       "ready",
			conditions: []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}},
			want:       "Ready",
		},
		{
			name:       "reason",
			conditions: []interface{}{map[string]interface{}{"type": "Ready", "status": "False", "reason": "InstallFailed"}},
			want:       "InstallFailed",
		},
		{
			name:       "notReady",
			conditions: []interface{}{map[string]interface{}{"type": "Ready", "status": "False"}},
			want:       "NotReady",
		},
		{
			name:       "onlyOtherType",
			conditions: []interface{}{map[string]interface{}{"type": "Stalled", "status": "True"}},
			want:       "",
		},
		{
			name: "skipsNonMap",
			conditions: []interface{}{
				"not-a-map",
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
			want: "Ready",
		},
		{
			name:       "noConditions",
			conditions: []interface{}{},
			want:       "",
		},
	}
	for _, c := range cases {
		got := conditionSummary(conditionsObject(c.conditions))
		if got != c.want {
			t.Fatalf("%s: conditionSummary = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStatusOf(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"health": map[string]interface{}{"status": "Degraded"},
			"sync":   map[string]interface{}{"status": "OutOfSync"},
		},
	}}
	if got := statusOf(app, "app"); got != "Degraded OutOfSync" {
		t.Fatalf("statusOf app = %q, want Degraded OutOfSync", got)
	}

	empty := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if got := statusOf(empty, "app"); got != "" {
		t.Fatalf("statusOf empty app = %q, want empty", got)
	}

	applier := conditionsObject([]interface{}{map[string]interface{}{"type": "Ready", "status": "True"}})
	if got := statusOf(applier, "applier"); got != "Ready" {
		t.Fatalf("statusOf applier = %q, want Ready", got)
	}
}
