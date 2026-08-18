package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func podDescriptors() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "pods"): {
			Group:      "",
			Version:    "v1",
			Resource:   "pods",
			Kind:       "Pod",
			Namespaced: true,
			Category:   "Workloads",
		},
	}
}

func dashboardServer(t *testing.T, objects ...runtime.Object) *httptest.Server {
	t.Helper()
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{podGVR: "PodList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, objects...)
	metaScheme := runtime.NewScheme()
	if err := metav1.AddMetaToScheme(metaScheme); err != nil {
		t.Fatalf("meta scheme: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Metadata:    metadatafake.NewSimpleMetadataClient(metaScheme, metaFor(objects)...),
		Descriptors: podDescriptors(),
	})
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func metaFor(objects []runtime.Object) []runtime.Object {
	out := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		item, ok := object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		out = append(out, &metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Namespace: item.GetNamespace(), Name: item.GetName()},
		})
	}
	return out
}

func newPodObject(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"uid":       "uid-" + name,
		},
		"spec": map[string]any{
			"containers": []any{map[string]any{"name": "app"}},
		},
	}}
}

func TestFluxOverviewEndpointAnswersWithoutFlux(t *testing.T) {
	ts := dashboardServer(t)

	var overview api.FluxOverview
	resp := getJSON(t, ts.URL+"/api/flux/overview", &overview)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
	}
	if len(overview.Controllers) != 0 {
		t.Fatalf("controllers = %d, want none on a cluster without flux", len(overview.Controllers))
	}
}

func TestArgoEndpointAnswersWithoutArgo(t *testing.T) {
	ts := dashboardServer(t)

	var dashboard api.ArgoDashboard
	getJSON(t, ts.URL+"/api/argocd", &dashboard)

	if len(dashboard.Apps) != 0 {
		t.Fatalf("apps = %d, want none on a cluster without argo", len(dashboard.Apps))
	}
}

func TestSearchEndpointFindsAnObjectByName(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "airbyte-server"), newPodObject("prod", "web-0"))

	var found api.SearchResults
	getJSON(t, ts.URL+"/api/search?q=airbyte", &found)

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %d, want the one pod whose name matches", len(found.Hits))
	}
	if found.Hits[0].Name != "airbyte-server" {
		t.Fatalf("hit = %q, want airbyte-server", found.Hits[0].Name)
	}
}

func TestSearchEndpointHasNothingToLookFor(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "airbyte-server"))

	var found api.SearchResults
	getJSON(t, ts.URL+"/api/search?q=", &found)

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %d, want none for an empty query", len(found.Hits))
	}
}

func TestDebugSupportEndpointSaysItIsUnavailable(t *testing.T) {
	ts := dashboardServer(t)

	var support api.DebugSupport
	getJSON(t, ts.URL+"/api/debug/support?namespace=prod&pod=web-0", &support)

	if support.Allowed {
		t.Fatal("allowed = true, want false without a debug service")
	}
	if support.Namespace != "prod" || support.Pod != "web-0" {
		t.Fatalf("support = %+v, want it to answer about prod/web-0", support)
	}
}

func TestDebugSupportEndpointNeedsANamespace(t *testing.T) {
	ts := dashboardServer(t)

	resp := getJSON(t, ts.URL+"/api/debug/support?pod=web-0", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestATerminalNeedsAWebsocketUpgrade(t *testing.T) {
	ts := dashboardServer(t)

	resp := getJSON(t, ts.URL+"/api/exec?namespace=prod&pod=web-0", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a plain GET was accepted as a terminal")
	}
}

func TestATerminalNeedsANamespaceAndPod(t *testing.T) {
	ts := dashboardServer(t)

	resp := getJSON(t, ts.URL+"/api/exec?pod=web-0", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
