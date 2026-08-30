package server

import (
	"context"
	"encoding/json"
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
	ts, _ := dashboardPair(t, objects...)
	return ts
}

func dashboardPair(t *testing.T, objects ...runtime.Object) (*httptest.Server, *Server) {
	t.Helper()
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		podGVR: "PodList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		{Version: "v1", Resource: "nodes"}:                               "NodeList",
	}
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
	return ts, srv
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

func TestChecksEndpointAuditsThePodsItCanSee(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "web-0"))

	var found api.CheckReport
	resp := getJSON(t, ts.URL+"/api/checks", &found)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if found.Scanned != 1 {
		t.Fatalf("scanned = %d, want the one pod", found.Scanned)
	}
	if len(found.Groups) == 0 {
		t.Fatal("the report carried no checks")
	}
	if !flagged(found, "requests-missing", "web-0") {
		t.Fatal("a pod with no resource requests was not reported")
	}
}

func TestChecksEndpointSkipsTheUsageCheckWithoutMetrics(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "web-0"))

	var found api.CheckReport
	getJSON(t, ts.URL+"/api/checks", &found)

	for _, group := range found.Groups {
		if group.ID != "requests-far-above-usage" {
			continue
		}
		if group.Skipped == "" {
			t.Fatal("the usage check ran on a cluster with no metrics API")
		}
		return
	}
	t.Fatal("the usage check is missing from the report")
}

func failureFrom(t *testing.T, resp *http.Response) api.Failure {
	t.Helper()
	var failure api.Failure
	err := json.NewDecoder(resp.Body).Decode(&failure)
	if err != nil {
		t.Fatalf("decode the refusal body: %v", err)
	}
	return failure
}

func TestCheckFindingsEndpointNeedsACheck(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "web-0"))

	resp := getJSON(t, ts.URL+"/api/checks/findings", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := failureFrom(t, resp).Message; got != "check is required" {
		t.Fatalf("message = %q", got)
	}
}

func TestCheckFindingsEndpointRefusesACheckNobodyRegistered(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "web-0"))

	resp := getJSON(t, ts.URL+"/api/checks/findings?check=invented", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := failureFrom(t, resp).Message; got != "no check goes by that name" {
		t.Fatalf("message = %q", got)
	}
}

func TestCheckFindingsEndpointReturnsThatChecksFindings(t *testing.T) {
	ts := dashboardServer(t, newPodObject("prod", "web-0"))

	var page api.CheckPage
	resp := getJSON(t, ts.URL+"/api/checks/findings?check=requests-missing", &page)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(page.Findings) == 0 {
		t.Fatal("the pod with no requests produced no findings")
	}
	if len(page.Objects) == 0 {
		t.Fatal("the page carried findings but no objects to resolve them against")
	}
	if page.Next != "" {
		t.Fatalf("a single-pod cluster offered a cursor: %q", page.Next)
	}
}

func flagged(found api.CheckReport, id, name string) bool {
	for _, group := range found.Groups {
		if group.ID != id {
			continue
		}
		for _, finding := range group.Findings {
			if finding.Ref < len(found.Objects) && found.Objects[finding.Ref].Name == name {
				return true
			}
		}
	}
	return false
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
