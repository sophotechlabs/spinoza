package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

var appViewDescs = []api.ResourceDescriptor{
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications", Kind: "Application", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
	{Group: "", Version: "v1", Resource: "services", Kind: "Service", Namespaced: true},
	{Group: "", Version: "v1", Resource: "events", Kind: "Event", Namespaced: true},
}

func appViewServer(t *testing.T, objs ...runtime.Object) *httptest.Server {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{}
	descs := map[string]api.ResourceDescriptor{}
	for _, one := range appViewDescs {
		gvr := schema.GroupVersionResource{Group: one.Group, Version: one.Version, Resource: one.Resource}
		kinds[gvr] = one.Kind + "List"
		descs[discovery.Key(one.Group, one.Version, one.Resource)] = one
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	return clusterServer(t, &stubBackendCluster{backend: mgr})
}

const appQuery = "?group=argoproj.io&version=v1alpha1&resource=applications" +
	"&namespace=argocd&name=podinfo"

func applicationWithResources() *unstructured.Unstructured {
	app := newArgoApplication()
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"status": "Synced"}, "status", "sync")
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"status": "Healthy"}, "status", "health")
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"version": "v1", "kind": "Service", "name": "podinfo", "namespace": "web", "status": "Synced"},
	}, "status", "resources")
	return app
}

func TestGitopsAppReturnsThePerApplicationView(t *testing.T) {
	ts := appViewServer(t, applicationWithResources())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app"+appQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var app api.GitopsApp
	if err := json.Unmarshal(body, &app); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if app.Controller != api.ControllerArgo || app.Name != "podinfo" {
		t.Fatalf("app = %+v", app)
	}
	if len(app.Resources) != 1 || app.Resources[0].Kind != "Service" {
		t.Fatalf("resources = %+v", app.Resources)
	}
}

func TestGitopsAppRefusesAnObjectNoControllerApplies(t *testing.T) {
	ts := appViewServer(t, newArgoApplication())
	query := "?group=apps&version=v1&resource=deployments&namespace=web&name=podinfo"

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app"+query, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGitopsAppReportsAnApplicationThatIsNotThere(t *testing.T) {
	ts := appViewServer(t)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app"+appQuery, nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGitopsAppNeedsARef(t *testing.T) {
	ts := appViewServer(t)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app?version=v1alpha1", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGitopsAppGraphHangsTheResourcesOffTheApplication(t *testing.T) {
	ts := appViewServer(t, applicationWithResources())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app/graph"+appQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var graph api.Graph
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("graph = %+v, want the application and its one resource", graph)
	}
	if graph.Edges[0].Kind != "manages" {
		t.Fatalf("edge = %+v", graph.Edges[0])
	}
}

func TestGitopsAppGraphReportsAnApplicationThatIsNotThere(t *testing.T) {
	ts := appViewServer(t)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/gitops/app/graph"+appQuery, nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
