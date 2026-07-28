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

var kustomizationGVR = schema.GroupVersionResource{
	Group:    "kustomize.toolkit.fluxcd.io",
	Version:  "v1",
	Resource: "kustomizations",
}

const kustomizationQuery = "?group=kustomize.toolkit.fluxcd.io&version=v1&resource=kustomizations" +
	"&namespace=flux-system&name=apps"

func newKustomization() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]interface{}{"interval": "10m", "suspend": false},
	}}
}

func fluxActionServer(t *testing.T, objs ...runtime.Object) (*httptest.Server, dynamic.Interface) {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{kustomizationGVR: "KustomizationList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"): {
			Group:      "kustomize.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "kustomizations",
			Kind:       "Kustomization",
			Namespaced: true,
		},
	}
	mgr := resources.NewManager(ctx, dyn, k8sfake.NewClientset(), nil, nil, nil, descs)
	ts := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts, dyn
}

func storedKustomization(t *testing.T, dyn dynamic.Interface) *unstructured.Unstructured {
	t.Helper()
	got, err := dyn.Resource(kustomizationGVR).
		Namespace("flux-system").
		Get(context.Background(), "apps", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestFluxActionSuspends(t *testing.T) {
	ts, dyn := fluxActionServer(t, newKustomization())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=suspend", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}
	suspended, _, _ := unstructured.NestedBool(storedKustomization(t, dyn).Object, "spec", "suspend")
	if !suspended {
		t.Fatalf("spec.suspend was not set")
	}
}

func TestFluxActionResumes(t *testing.T) {
	suspended := newKustomization()
	if err := unstructured.SetNestedField(suspended.Object, true, "spec", "suspend"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts, dyn := fluxActionServer(t, suspended)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=resume", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	value, _, _ := unstructured.NestedBool(storedKustomization(t, dyn).Object, "spec", "suspend")
	if value {
		t.Fatalf("spec.suspend was not cleared")
	}
}

func TestFluxActionReconciles(t *testing.T) {
	ts, dyn := fluxActionServer(t, newKustomization())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=reconcile", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	annotations := storedKustomization(t, dyn).GetAnnotations()
	if annotations["reconcile.fluxcd.io/requestedAt"] == "" {
		t.Fatalf("reconcile annotation missing: %v", annotations)
	}
}

func TestFluxActionRejectsUnknownAction(t *testing.T) {
	ts, _ := fluxActionServer(t, newKustomization())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=explode", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestFluxActionRejectsNonFluxGroup(t *testing.T) {
	ts, _ := fluxActionServer(t, newKustomization())
	query := "?group=apps&version=v1&resource=deployments&namespace=d&name=web&action=reconcile"

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+query, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestFluxActionRequiresParams(t *testing.T) {
	ts, _ := fluxActionServer(t, newKustomization())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action?version=v1&action=suspend", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestFluxActionRejectsGet(t *testing.T) {
	ts, _ := fluxActionServer(t, newKustomization())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=suspend", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestFluxActionMissingObject(t *testing.T) {
	ts, _ := fluxActionServer(t)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/flux/action"+kustomizationQuery+"&action=suspend", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
