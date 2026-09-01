package cluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/transport"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

func accessKubeconfig(t *testing.T, server string) string {
	t.Helper()
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: remote
clusters:
- name: remote
  cluster:
    server: %s
    insecure-skip-tls-verify: true
contexts:
- name: remote
  context:
    cluster: remote
    user: test-user
users:
- name: test-user
  user:
    token: test-token
`, server)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func widgetTarget(namespace, name string) api.ObjectRef {
	return api.ObjectRef{
		Group:     "example.com",
		Version:   "v1",
		Resource:  "widgets",
		Namespace: namespace,
		Name:      name,
	}
}

func TestCrossContextReaderGetsTheNamespacedObjectAsYAML(t *testing.T) {
	requests := make(chan *http.Request, 1)
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "apiVersion":"example.com/v1",
            "kind":"Widget",
            "metadata":{"name":"one","namespace":"prod"},
            "spec":{"replicas":3}
        }`))
	}))
	t.Cleanup(apiserver.Close)
	path := accessKubeconfig(t, apiserver.URL)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{
		User:   "alice@example.com",
		Groups: []string{"platform", "sre"},
	})

	document, err := readerFor(Options{Kubeconfig: path, Impersonate: true})(
		ctx,
		api.ContextRef{Name: "remote"},
		widgetTarget("prod", "one"),
	)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	seen := <-requests
	if seen.URL.Path != "/apis/example.com/v1/namespaces/prod/widgets/one" {
		t.Fatalf("path = %q", seen.URL.Path)
	}
	if seen.Header.Get("Authorization") != "Bearer test-token" {
		t.Fatalf("authorization = %q", seen.Header.Get("Authorization"))
	}
	if seen.Header.Get(transport.ImpersonateUserHeader) != "alice@example.com" {
		t.Fatalf("impersonated user = %q", seen.Header.Get(transport.ImpersonateUserHeader))
	}
	groups := strings.Join(seen.Header.Values(transport.ImpersonateGroupHeader), ",")
	if groups != "platform,sre" {
		t.Fatalf("impersonated groups = %q", groups)
	}
	for _, fragment := range []string{"apiVersion: example.com/v1", "kind: Widget", "replicas: 3"} {
		if !strings.Contains(document, fragment) {
			t.Errorf("document does not contain %q:\n%s", fragment, document)
		}
	}
}

func TestCrossContextListerUsesTheKubeconfigCarriedByTheContext(t *testing.T) {
	requests := make(chan *http.Request, 1)
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "apiVersion":"example.com/v1",
            "kind":"WidgetList",
            "items":[
                {"apiVersion":"example.com/v1","kind":"Widget","metadata":{"name":"one","namespace":"prod"}},
                {"apiVersion":"example.com/v1","kind":"Widget","metadata":{"name":"two","namespace":"prod"}}
            ]
        }`))
	}))
	t.Cleanup(apiserver.Close)
	path := accessKubeconfig(t, apiserver.URL)

	items, err := listerFor(Options{Kubeconfig: filepath.Join(t.TempDir(), "missing")})(
		t.Context(),
		api.ContextRef{Name: "remote", Kubeconfig: path},
		widgetTarget("prod", ""),
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	seen := <-requests
	if seen.URL.Path != "/apis/example.com/v1/namespaces/prod/widgets" {
		t.Fatalf("path = %q", seen.URL.Path)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].GetName() != "one" || items[1].GetName() != "two" {
		t.Fatalf("names = %q, %q", items[0].GetName(), items[1].GetName())
	}
	items[0].SetName("changed")
	if items[1].GetName() != "two" {
		t.Fatal("list entries shared storage")
	}
}

func TestCrossContextReaderReturnsTheApiserverFailure(t *testing.T) {
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
            "kind":"Status",
            "apiVersion":"v1",
            "status":"Failure",
            "message":"widgets.example.com one is forbidden",
            "reason":"Forbidden",
            "code":403
        }`))
	}))
	t.Cleanup(apiserver.Close)
	path := accessKubeconfig(t, apiserver.URL)

	_, err := readerFor(Options{Kubeconfig: path})(
		t.Context(),
		api.ContextRef{Name: "remote"},
		widgetTarget("prod", "one"),
	)

	if err == nil {
		t.Fatal("a forbidden read succeeded")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossContextListerReturnsTheApiserverFailure(t *testing.T) {
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{
            "kind":"Status",
            "apiVersion":"v1",
            "status":"Failure",
            "message":"storage unavailable",
            "reason":"InternalError",
            "code":500
        }`))
	}))
	t.Cleanup(apiserver.Close)
	path := accessKubeconfig(t, apiserver.URL)

	_, err := listerFor(Options{Kubeconfig: path})(
		t.Context(),
		api.ContextRef{Name: "remote"},
		widgetTarget("prod", ""),
	)

	if err == nil {
		t.Fatal("a failed list succeeded")
	}
	if !strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossContextAccessNamesKubeconfigFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	ref := api.ContextRef{Name: "remote"}
	target := widgetTarget("prod", "one")

	_, readErr := readerFor(Options{Kubeconfig: missing})(t.Context(), ref, target)
	if readErr == nil {
		t.Fatal("read with a missing kubeconfig succeeded")
	}
	if !strings.Contains(readErr.Error(), "kube:") {
		t.Fatalf("read error = %v", readErr)
	}

	_, listErr := listerFor(Options{Kubeconfig: missing})(t.Context(), ref, target)
	if listErr == nil {
		t.Fatal("list with a missing kubeconfig succeeded")
	}
	if !strings.Contains(listErr.Error(), "kube:") {
		t.Fatalf("list error = %v", listErr)
	}
}

func TestCrossContextAccessHonoursCancellation(t *testing.T) {
	received := make(chan struct{}, 2)
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		received <- struct{}{}
	}))
	t.Cleanup(apiserver.Close)
	path := accessKubeconfig(t, apiserver.URL)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ref := api.ContextRef{Name: "remote"}
	target := widgetTarget("prod", "one")

	_, readErr := readerFor(Options{Kubeconfig: path})(ctx, ref, target)
	if !strings.Contains(fmt.Sprint(readErr), context.Canceled.Error()) {
		t.Fatalf("read error = %v, want cancellation", readErr)
	}

	_, listErr := listerFor(Options{Kubeconfig: path})(ctx, ref, target)
	if !strings.Contains(fmt.Sprint(listErr), context.Canceled.Error()) {
		t.Fatalf("list error = %v, want cancellation", listErr)
	}

	select {
	case <-received:
		t.Fatal("a canceled request reached the apiserver")
	default:
	}
}

func discoveryAPIServer(t *testing.T, partial bool) *httptest.Server {
	t.Helper()
	apiserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_, _ = w.Write([]byte(`{
                "kind":"APIVersions",
                "apiVersion":"v1",
                "versions":["v1"],
                "serverAddressByClientCIDRs":[]
            }`))
		case "/apis":
			groups := ""
			if partial {
				groups = `{
					"name":"broken.example.com",
					"versions":[{"groupVersion":"broken.example.com/v1","version":"v1"}],
					"preferredVersion":{"groupVersion":"broken.example.com/v1","version":"v1"}
				}`
			}
			_, _ = fmt.Fprintf(w, `{
                "kind":"APIGroupList",
                "apiVersion":"v1",
				"groups":[%s]
			}`, groups)
		case "/api/v1":
			_, _ = w.Write([]byte(`{
                "kind":"APIResourceList",
                "apiVersion":"v1",
                "groupVersion":"v1",
                "resources":[
                    {
                        "name":"pods",
                        "singularName":"pod",
                        "namespaced":true,
                        "kind":"Pod",
                        "verbs":["get","list","watch"]
                    }
                ]
            }`))
		case "/apis/broken.example.com/v1":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{
				"kind":"Status",
				"apiVersion":"v1",
				"status":"Failure",
				"message":"broken discovery is unavailable",
				"reason":"ServiceUnavailable",
				"code":503
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{
                "kind":"Status",
                "apiVersion":"v1",
                "status":"Failure",
                "reason":"NotFound",
                "code":404
            }`))
		}
	}))
	t.Cleanup(apiserver.Close)
	return apiserver
}

func TestBuildWiresAReachableContext(t *testing.T) {
	apiserver := discoveryAPIServer(t, false)
	path := accessKubeconfig(t, apiserver.URL)
	helmRoot := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(helmRoot, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(helmRoot, "cache"))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	manager, bundle, err := build(
		ctx,
		api.ContextRef{Name: "remote"},
		Options{Kubeconfig: path},
		prom.Target{},
	)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if manager == nil {
		t.Fatal("build returned no resource manager")
	}
	if bundle == nil {
		t.Fatal("build returned no Kubernetes clients")
	}
	if bundle.Ref.Name != "remote" {
		t.Fatalf("context = %q", bundle.Ref.Name)
	}
	if bundle.Config.Host != apiserver.URL {
		t.Fatalf("host = %q, want %q", bundle.Config.Host, apiserver.URL)
	}

	catalog := manager.Resources()
	if catalog.Error != "" {
		t.Fatalf("catalog error = %q", catalog.Error)
	}
	foundPod := false
	for _, category := range catalog.Categories {
		for _, resource := range category.Resources {
			if resource.Group == "" && resource.Version == "v1" && resource.Resource == "pods" {
				foundPod = true
			}
		}
	}
	if !foundPod {
		t.Fatalf("catalog = %+v, want core pods", catalog.Categories)
	}
}

func TestNewStartsOnAReachableContext(t *testing.T) {
	apiserver := discoveryAPIServer(t, false)
	path := accessKubeconfig(t, apiserver.URL)
	helmRoot := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(helmRoot, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(helmRoot, "cache"))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	cluster, err := New(ctx, Options{Kubeconfig: path})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if cluster.Manager("") == nil {
		t.Fatal("a reachable current context produced no resource manager")
	}
	if cluster.Current().Name != "remote" {
		t.Fatalf("current context = %q, want remote", cluster.Current().Name)
	}
	if cluster.Contexts().Error != "" {
		t.Fatalf("contexts error = %q, want the startup failure cleared", cluster.Contexts().Error)
	}
}

func TestBuildKeepsResourcesFromAPartialDiscovery(t *testing.T) {
	apiserver := discoveryAPIServer(t, true)
	path := accessKubeconfig(t, apiserver.URL)
	helmRoot := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(helmRoot, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(helmRoot, "cache"))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	manager, _, err := build(
		ctx,
		api.ContextRef{Name: "remote"},
		Options{Kubeconfig: path},
		prom.Target{},
	)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	catalog := manager.Resources()
	if !strings.Contains(catalog.Error, "broken.example.com/v1") {
		t.Fatalf("catalog error = %q, want the incomplete API group named", catalog.Error)
	}
	foundPod := false
	for _, category := range catalog.Categories {
		for _, resource := range category.Resources {
			if resource.Group == "" && resource.Version == "v1" && resource.Resource == "pods" {
				foundPod = true
			}
		}
	}
	if !foundPod {
		t.Fatalf("catalog = %+v, want resources from the API group that answered", catalog.Categories)
	}
}
