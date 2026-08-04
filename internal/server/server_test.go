package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/version"
)

var depGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

func deploymentDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      "apps",
		Version:    "v1",
		Resource:   "deployments",
		Kind:       "Deployment",
		Namespaced: true,
		Category:   "Workloads",
	}
}

func newDeployment(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"uid":       "uid-" + name,
		},
		"spec": map[string]any{"replicas": int64(1)},
		"status": map[string]any{
			"readyReplicas":     int64(1),
			"updatedReplicas":   int64(1),
			"availableReplicas": int64(1),
		},
	}}
}

func testManager(t *testing.T, objs ...runtime.Object) (*resources.Manager, dynamic.Interface) {
	t.Helper()
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{depGVR: "DeploymentList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): deploymentDesc(),
	}
	cats := []api.Category{{Name: "Workloads", Resources: []api.ResourceDescriptor{deploymentDesc()}}}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Categories: cats, Descriptors: descs})
	return mgr, dyn
}

const testToken = "SPINOZATESTTOKEN0000000000"

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><head></head><body>spinoza-index</body></html>")},
		"app.js":     &fstest.MapFile{Data: []byte("spinoza-bundle")},
	}
}

func authed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AuthHeader) == "" {
			r.Header.Set(AuthHeader, testToken)
		}
		next.ServeHTTP(w, r)
	})
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/ws"
}

func readMsg(ctx context.Context, t *testing.T, c *websocket.Conn) api.ServerMsg {
	t.Helper()
	var msg api.ServerMsg
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatalf("read message: %v", err)
	}
	return msg
}

func TestHealthzReturnsOK(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var health api.Health
	if decodeErr := json.Unmarshal(body, &health); decodeErr != nil {
		t.Fatalf("decode %s: %v", body, decodeErr)
	}
	if health.Status != "ok" {
		t.Fatalf("status = %q", health.Status)
	}
	if health.Version != version.String() {
		t.Fatalf("version = %q, want the build to name itself", health.Version)
	}
	if health.Context != "p-mk2" {
		t.Fatalf("context = %q, want the context a bug report needs", health.Context)
	}
}

func TestVersionEndpointNamesTheBuild(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/version", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var build api.Build
	if err := json.Unmarshal(body, &build); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if build.Version != version.String() {
		t.Fatalf("version = %q", build.Version)
	}
}

func TestNoCrossOriginAccessIsGranted(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/resources")
	if err != nil {
		t.Fatalf("GET /api/resources: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("the API grants cross-origin reads")
	}
	if resp.Header.Get("Access-Control-Allow-Methods") != "" {
		t.Fatal("the API grants cross-origin writes")
	}
}

func TestRootServesSPAIndex(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "spinoza-index") {
		t.Fatalf("body = %q, want SPA index", string(body))
	}
}

func TestResourcesEndpoint(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/resources")
	if err != nil {
		t.Fatalf("GET /api/resources: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
	}
	var catalog api.ResourceCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cats := catalog.Categories
	if len(cats) != 1 {
		t.Fatalf("categories = %d, want 1", len(cats))
	}
	if cats[0].Name != "Workloads" {
		t.Fatalf("category = %q, want Workloads", cats[0].Name)
	}
	if len(cats[0].Resources) != 1 || cats[0].Resources[0].Resource != "deployments" {
		t.Fatalf("resources = %v, want [deployments]", cats[0].Resources)
	}
}

func fluxGitRepo() *unstructured.Unstructured {
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

func fluxKustomization() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]any{
			"sourceRef": map[string]any{
				"kind": "GitRepository",
				"name": "app-repo",
			},
		},
	}}
}

func TestGraphEndpoint(t *testing.T) {
	gitRepoGVR := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	kustGVR := schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		gitRepoGVR: "GitRepositoryList",
		kustGVR:    "KustomizationList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, fluxGitRepo(), fluxKustomization())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group:      "source.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "gitrepositories",
			Kind:       "GitRepository",
			Namespaced: true,
			Category:   "Custom Resources",
		},
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"): {
			Group:      "kustomize.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "kustomizations",
			Kind:       "Kustomization",
			Namespaced: true,
			Category:   "Custom Resources",
		},
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/gitops/graph")
	if err != nil {
		t.Fatalf("GET /api/gitops/graph: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
	}
	var graph api.Graph
	if err := json.NewDecoder(resp.Body).Decode(&graph); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(graph.Edges))
	}
	edge := graph.Edges[0]
	if edge.Kind != "source" {
		t.Fatalf("edge kind = %q, want source", edge.Kind)
	}
	if edge.From != "source.toolkit.fluxcd.io/GitRepository/flux-system/app-repo" {
		t.Fatalf("edge from = %q, want the GitRepository node", edge.From)
	}
	if edge.To != "kustomize.toolkit.fluxcd.io/Kustomization/flux-system/apps" {
		t.Fatalf("edge to = %q, want the Kustomization node", edge.To)
	}
}

func TestFluxEndpoint(t *testing.T) {
	gitRepoGVR := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	kustGVR := schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		gitRepoGVR: "GitRepositoryList",
		kustGVR:    "KustomizationList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, fluxGitRepo(), fluxKustomization())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group:      "source.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "gitrepositories",
			Kind:       "GitRepository",
			Namespaced: true,
			Category:   "Custom Resources",
		},
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"): {
			Group:      "kustomize.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "kustomizations",
			Kind:       "Kustomization",
			Namespaced: true,
			Category:   "Custom Resources",
		},
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/flux")
	if err != nil {
		t.Fatalf("GET /api/flux: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
	}
	var dash api.FluxDashboard
	if err := json.NewDecoder(resp.Body).Decode(&dash); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dash.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(dash.Groups))
	}
	if dash.Groups[0].Name != "Kustomizations" {
		t.Fatalf("group 0 = %q, want Kustomizations", dash.Groups[0].Name)
	}
	if dash.Groups[1].Name != "Sources" {
		t.Fatalf("group 1 = %q, want Sources", dash.Groups[1].Name)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		{Group: "", Version: "v1", Resource: "nodes"}:                    "NodeList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset()})
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/metrics")
	if err != nil {
		t.Fatalf("GET /api/metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var m api.Metrics
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestEventToMsgDeleted(t *testing.T) {
	msg := eventToMsg("sub-1", resources.Event{Kind: "deleted", UID: "uid-1"})
	if msg.Type != "deleted" {
		t.Fatalf("Type = %q, want deleted", msg.Type)
	}
	if msg.SubID != "sub-1" {
		t.Fatalf("SubID = %q, want sub-1", msg.SubID)
	}
	if msg.UID != "uid-1" {
		t.Fatalf("UID = %q, want uid-1", msg.UID)
	}
	if msg.Row != nil {
		t.Fatalf("Row = %v, want nil", msg.Row)
	}
}

func TestEventToMsgAdded(t *testing.T) {
	row := api.Row{UID: "uid-2", Name: "dep-b"}
	msg := eventToMsg("sub-2", resources.Event{Kind: "added", Row: row})
	if msg.Type != "added" {
		t.Fatalf("Type = %q, want added", msg.Type)
	}
	if msg.SubID != "sub-2" {
		t.Fatalf("SubID = %q, want sub-2", msg.SubID)
	}
	if msg.Row == nil {
		t.Fatal("Row = nil, want row")
	}
	if msg.Row.UID != "uid-2" {
		t.Fatalf("Row.UID = %q, want uid-2", msg.Row.UID)
	}
}

func TestEventToMsgModified(t *testing.T) {
	row := api.Row{UID: "uid-3", Name: "dep-c"}
	msg := eventToMsg("sub-3", resources.Event{Kind: "modified", Row: row})
	if msg.Type != "modified" {
		t.Fatalf("Type = %q, want modified", msg.Type)
	}
	if msg.Row == nil || msg.Row.Name != "dep-c" {
		t.Fatalf("Row = %v, want dep-c", msg.Row)
	}
}

func TestWSSnapshotAndDeltas(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}

	snap := readMsg(ctx, t, conn)
	if snap.Type != "snapshot" {
		t.Fatalf("Type = %q, want snapshot", snap.Type)
	}
	if snap.SubID != "s1" {
		t.Fatalf("SubID = %q, want s1", snap.SubID)
	}
	if !snap.Namespaced {
		t.Fatal("Namespaced = false, want true")
	}
	if len(snap.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(snap.Columns))
	}
	if len(snap.Rows) != 1 || snap.Rows[0].Name != "web" {
		t.Fatalf("rows = %v, want [web]", snap.Rows)
	}

	_, err = dyn.Resource(depGVR).Namespace("default").Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	added := readMsg(ctx, t, conn)
	if added.Type != "added" {
		t.Fatalf("Type = %q, want added", added.Type)
	}
	if added.Row == nil || added.Row.Name != "api" {
		t.Fatalf("Row = %v, want api", added.Row)
	}

	cur, err := dyn.Resource(depGVR).Namespace("default").Get(ctx, "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	setErr := unstructured.SetNestedField(cur.Object, int64(3), "spec", "replicas")
	if setErr != nil {
		t.Fatalf("set replicas: %v", setErr)
	}
	_, err = dyn.Resource(depGVR).Namespace("default").Update(ctx, cur, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	modified := readMsg(ctx, t, conn)
	if modified.Type != "modified" {
		t.Fatalf("Type = %q, want modified", modified.Type)
	}

	err = dyn.Resource(depGVR).Namespace("default").Delete(ctx, "api", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	deleted := readMsg(ctx, t, conn)
	if deleted.Type != "deleted" {
		t.Fatalf("Type = %q, want deleted", deleted.Type)
	}
	if deleted.UID != "uid-api" {
		t.Fatalf("UID = %q, want uid-api", deleted.UID)
	}
}

func TestWSUnsubscribeStopsDeltas(t *testing.T) {
	mgr, dyn := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}
	snap := readMsg(ctx, t, conn)
	if snap.Type != "snapshot" {
		t.Fatalf("Type = %q, want snapshot", snap.Type)
	}

	unsub := api.ClientMsg{Type: "unsubscribe", SubID: "s1"}
	writeErr = wsjson.Write(ctx, conn, unsub)
	if writeErr != nil {
		t.Fatalf("write unsubscribe: %v", writeErr)
	}

	time.Sleep(200 * time.Millisecond)
	_, err = dyn.Resource(depGVR).Namespace("default").Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer readCancel()
	var msg api.ServerMsg
	readErr := wsjson.Read(readCtx, conn, &msg)
	if readErr == nil {
		t.Fatalf("received unexpected message after unsubscribe: %+v", msg)
	}
}

func TestWSSubscribeUnknownResource(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "statefulsets", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}

	msg := readMsg(ctx, t, conn)
	if msg.Type != "error" {
		t.Fatalf("Type = %q, want error", msg.Type)
	}
	if msg.SubID != "s1" {
		t.Fatalf("SubID = %q, want s1", msg.SubID)
	}
	if msg.Message == "" {
		t.Fatal("Message = empty, want error text")
	}
}

func TestWSResubscribeReplacesSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}
	first := readMsg(ctx, t, conn)
	if first.Type != "snapshot" {
		t.Fatalf("Type = %q, want snapshot", first.Type)
	}

	writeErr = wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write resubscribe: %v", writeErr)
	}
	second := readMsg(ctx, t, conn)
	if second.Type != "snapshot" {
		t.Fatalf("Type = %q, want snapshot on resubscribe", second.Type)
	}
}

func TestWSExitsOnServerContextCancel(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	srv := New(fixed(mgr), testAssets(), testToken)

	ts := httptest.NewUnstartedServer(authed(srv.Handler()))
	baseCtx, cancelBase := context.WithCancel(context.Background())
	ts.Config.BaseContext = func(net.Listener) context.Context {
		return baseCtx
	}
	ts.Start()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}
	snap := readMsg(ctx, t, conn)
	if snap.Type != "snapshot" {
		t.Fatalf("Type = %q, want snapshot", snap.Type)
	}

	cancelBase()

	_, _, readErr := conn.Read(ctx)
	if readErr == nil {
		t.Fatal("expected connection to close after server context cancel")
	}
}

func TestWSRejectsNonWebsocketRequest(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws")
	if err != nil {
		t.Fatalf("GET /ws: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want a non-upgrade rejection", resp.StatusCode)
	}
}

func readRaw(ctx context.Context, t *testing.T, c *websocket.Conn) map[string]json.RawMessage {
	t.Helper()
	fields := map[string]json.RawMessage{}
	err := wsjson.Read(ctx, c, &fields)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	return fields
}

func subscribeRaw(t *testing.T, mgr *resources.Manager, sub api.ClientMsg) map[string]json.RawMessage {
	t.Helper()
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	writeErr := wsjson.Write(ctx, conn, sub)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}
	return readRaw(ctx, t, conn)
}

func TestWSSnapshotOfAnEmptyResourceCarriesRows(t *testing.T) {
	mgr, _ := testManager(t)

	fields := subscribeRaw(t, mgr, api.ClientMsg{
		Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default",
	})

	if string(fields["type"]) != `"snapshot"` {
		t.Fatalf("type = %s", fields["type"])
	}
	if string(fields["rows"]) != "[]" {
		t.Fatalf("rows = %s, want []; the browser iterates it without a guard", fields["rows"])
	}
}

func clusterScopedManager(t *testing.T) *resources.Manager {
	t.Helper()
	nodeGVR := schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeGVR: "NodeList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	desc := api.ResourceDescriptor{Version: "v1", Resource: "nodes", Kind: "Node", Namespaced: false}
	descs := map[string]api.ResourceDescriptor{discovery.Key("", "v1", "nodes"): desc}
	return resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
}

func TestWSSnapshotOfAClusterScopedResourceCarriesNamespaced(t *testing.T) {
	mgr := clusterScopedManager(t)

	fields := subscribeRaw(t, mgr, api.ClientMsg{
		Type: "subscribe", SubID: "s1", Version: "v1", Resource: "nodes",
	})

	value, present := fields["namespaced"]
	if !present {
		t.Fatal("namespaced was dropped for a cluster-scoped resource")
	}
	if string(value) != "false" {
		t.Fatalf("namespaced = %s, want false", value)
	}
}

func TestWSKeepsReadingWhileASubscriptionIsStillSyncing(t *testing.T) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{depGVR: "DeploymentList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, newDeployment("default", "web"))
	gate := make(chan struct{})
	dyn.PrependReactor("list", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		<-gate
		return false, nil, nil
	})
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): deploymentDesc(),
	}
	cats := []api.Category{{Name: "Workloads", Resources: []api.ResourceDescriptor{deploymentDesc()}}}
	mgr := resources.NewManager(t.Context(), resources.Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Categories:  cats,
		Descriptors: descs,
	})

	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	defer close(gate)

	slow := api.ClientMsg{Type: "subscribe", SubID: "slow", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	writeErr := wsjson.Write(ctx, conn, slow)
	if writeErr != nil {
		t.Fatalf("write subscribe: %v", writeErr)
	}

	next := api.ClientMsg{Type: "subscribe", SubID: "unknown", Group: "apps", Version: "v1", Resource: "statefulsets", Namespace: "default"}
	nextErr := wsjson.Write(ctx, conn, next)
	if nextErr != nil {
		t.Fatalf("write second subscribe: %v", nextErr)
	}

	msg := readMsg(ctx, t, conn)
	if msg.SubID != "unknown" {
		t.Fatalf("SubID = %q, want the second frame answered while %q was still syncing", msg.SubID, "slow")
	}
	if msg.Type != "error" {
		t.Fatalf("Type = %q, want error", msg.Type)
	}
}
