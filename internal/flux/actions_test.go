package flux

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var (
	kustomizationGVR = schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	clusterScopedGVR = schema.GroupVersionResource{
		Group:    "notification.toolkit.fluxcd.io",
		Version:  "v1beta3",
		Resource: "providers",
	}
)

var stamp = time.Date(2026, 7, 27, 16, 30, 0, 0, time.UTC)

func actionClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		kustomizationGVR: "KustomizationList",
		clusterScopedGVR: "ProviderList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func newKustomization(suspended bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]interface{}{
			"interval": "10m",
			"suspend":  suspended,
		},
	}}
}

func kustomizationRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "kustomize.toolkit.fluxcd.io",
		Version:   "v1",
		Resource:  "kustomizations",
		Namespace: "flux-system",
		Name:      "apps",
	}
}

func readBack(t *testing.T, client *fake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(kustomizationGVR).
		Namespace("flux-system").
		Get(context.Background(), "apps", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestReconcileSetsTheRequestAnnotation(t *testing.T) {
	client := actionClient(newKustomization(false))

	err := Do(context.Background(), client, kustomizationRef(), Reconcile, stamp)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := readBack(t, client)
	want := "2026-07-27T16:30:00Z"
	if got.GetAnnotations()[reconcileAnnotation] != want {
		t.Fatalf("annotation = %q, want %q", got.GetAnnotations()[reconcileAnnotation], want)
	}
}

func TestReconcileLeavesTheSpecAlone(t *testing.T) {
	client := actionClient(newKustomization(false))

	err := Do(context.Background(), client, kustomizationRef(), Reconcile, stamp)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := readBack(t, client)
	interval, _, _ := unstructured.NestedString(got.Object, "spec", "interval")
	if interval != "10m" {
		t.Fatalf("spec.interval = %q, want 10m", interval)
	}
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if suspended {
		t.Fatalf("reconcile changed spec.suspend")
	}
}

func TestSuspendSetsTheField(t *testing.T) {
	client := actionClient(newKustomization(false))

	err := Do(context.Background(), client, kustomizationRef(), Suspend, stamp)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	got := readBack(t, client)
	suspended, found, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !found || !suspended {
		t.Fatalf("spec.suspend = %v (found %v), want true", suspended, found)
	}
}

func TestResumeClearsTheField(t *testing.T) {
	client := actionClient(newKustomization(true))

	err := Do(context.Background(), client, kustomizationRef(), Resume, stamp)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	got := readBack(t, client)
	suspended, found, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !found || suspended {
		t.Fatalf("spec.suspend = %v (found %v), want false", suspended, found)
	}
}

func TestSuspendDoesNotAnnotate(t *testing.T) {
	client := actionClient(newKustomization(false))

	err := Do(context.Background(), client, kustomizationRef(), Suspend, stamp)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}

	if readBack(t, client).GetAnnotations()[reconcileAnnotation] != "" {
		t.Fatalf("suspend added a reconcile annotation")
	}
}

func TestClusterScopedTarget(t *testing.T) {
	provider := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "notification.toolkit.fluxcd.io/v1beta3",
		"kind":       "Provider",
		"metadata":   map[string]interface{}{"name": "slack"},
	}}
	client := actionClient(provider)
	ref := api.ObjectRef{
		Group:    "notification.toolkit.fluxcd.io",
		Version:  "v1beta3",
		Resource: "providers",
		Name:     "slack",
	}

	err := Do(context.Background(), client, ref, Suspend, stamp)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	got, getErr := client.Resource(clusterScopedGVR).Get(context.Background(), "slack", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspended {
		t.Fatalf("cluster-scoped suspend did not apply")
	}
}

func TestRejectsNonFluxGroup(t *testing.T) {
	client := actionClient()
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "d", Name: "web"}

	err := Do(context.Background(), client, ref, Reconcile, stamp)

	if err == nil {
		t.Fatalf("expected a non-flux group to be rejected")
	}
}

func TestRejectsUnknownAction(t *testing.T) {
	client := actionClient(newKustomization(false))

	err := Do(context.Background(), client, kustomizationRef(), Action("explode"), stamp)

	if err == nil {
		t.Fatalf("expected an unknown action to be rejected")
	}
}

func TestPropagatesAPIError(t *testing.T) {
	client := actionClient()

	err := Do(context.Background(), client, kustomizationRef(), Reconcile, stamp)

	if err == nil {
		t.Fatalf("expected an error patching a missing object")
	}
}

func TestIsFluxGroup(t *testing.T) {
	cases := map[string]bool{
		"kustomize.toolkit.fluxcd.io":    true,
		"source.toolkit.fluxcd.io":       true,
		"helm.toolkit.fluxcd.io":         true,
		"notification.toolkit.fluxcd.io": true,
		"image.toolkit.fluxcd.io":        true,
		"apps":                           false,
		"":                               false,
		"fluxcd.io":                      false,
		"toolkit.fluxcd.io":              false,
	}
	for group, want := range cases {
		if got := IsFluxGroup(group); got != want {
			t.Fatalf("IsFluxGroup(%q) = %v, want %v", group, got, want)
		}
	}
}
