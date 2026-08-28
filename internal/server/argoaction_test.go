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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

var argoApplicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

const applicationQuery = "?group=argoproj.io&version=v1alpha1&resource=applications" +
	"&namespace=argocd&name=podinfo"

func newArgoApplication() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "podinfo",
			"namespace": "argocd",
		},
		"spec": map[string]any{"project": "default"},
	}}
}

func argoActionServer(t *testing.T, protected bool, objs ...runtime.Object) (*httptest.Server, dynamic.Interface) {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{argoApplicationGVR: "ApplicationList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("argoproj.io", "v1alpha1", "applications"): {
			Group:      "argoproj.io",
			Version:    "v1alpha1",
			Resource:   "applications",
			Kind:       "Application",
			Namespaced: true,
		},
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	ts := clusterServer(t, &stubBackendCluster{backend: mgr, protected: protected})
	return ts, dyn
}

func storedApplication(t *testing.T, dyn dynamic.Interface) *unstructured.Unstructured {
	t.Helper()
	got, err := dyn.Resource(argoApplicationGVR).
		Namespace("argocd").
		Get(t.Context(), "podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestArgoActionSyncs(t *testing.T) {
	ts, dyn := argoActionServer(t, false, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if _, found, _ := unstructured.NestedMap(storedApplication(t, dyn).Object, "operation"); !found {
		t.Fatal("no sync operation reached the application")
	}
}

func TestArgoActionRefreshes(t *testing.T) {
	ts, dyn := argoActionServer(t, false, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=refresh", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if storedApplication(t, dyn).GetAnnotations()["argocd.argoproj.io/refresh"] != "normal" {
		t.Fatal("the refresh annotation did not land")
	}
}

func TestArgoActionRejectsUnknownAction(t *testing.T) {
	ts, _ := argoActionServer(t, false, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=explode", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestArgoActionRejectsANonArgoGroup(t *testing.T) {
	ts, _ := argoActionServer(t, false, newArgoApplication())
	query := "?group=apps&version=v1&resource=deployments&namespace=d&name=web&action=sync"

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+query, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestArgoActionRequiresParams(t *testing.T) {
	ts, _ := argoActionServer(t, false, newArgoApplication())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action?version=v1alpha1&action=sync", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestArgoActionRejectsGet(t *testing.T) {
	ts, _ := argoActionServer(t, false, newArgoApplication())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestArgoActionMissingObject(t *testing.T) {
	ts, _ := argoActionServer(t, false)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSyncOnAProtectedClusterNeedsTheTypedName(t *testing.T) {
	ts, dyn := argoActionServer(t, true, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync", nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412: %s", resp.StatusCode, body)
	}
	if _, found, _ := unstructured.NestedMap(storedApplication(t, dyn).Object, "operation"); found {
		t.Fatal("the sync ran without confirmation")
	}
}

func TestSyncOnAProtectedClusterRunsOnceTheNameMatches(t *testing.T) {
	ts, dyn := argoActionServer(t, true, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync&confirm=podinfo", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if _, found, _ := unstructured.NestedMap(storedApplication(t, dyn).Object, "operation"); !found {
		t.Fatal("the confirmed sync did not run")
	}
}

func TestSyncOnAProtectedClusterRefusesTheWrongName(t *testing.T) {
	ts, _ := argoActionServer(t, true, newArgoApplication())

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action"+applicationQuery+"&action=sync&confirm=something-else", nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
}

func TestRefreshNeedsNoConfirmationEvenWhenProtected(t *testing.T) {
	ts, dyn := argoActionServer(t, true, newArgoApplication())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/argocd/action"+applicationQuery+"&action=refresh", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want a read-only refresh to go through: %s", resp.StatusCode, body)
	}
	if storedApplication(t, dyn).GetAnnotations()["argocd.argoproj.io/refresh"] != "normal" {
		t.Fatal("the refresh annotation did not land")
	}
}
