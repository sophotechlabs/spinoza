package flux

import (
	"context"
	"errors"
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
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]any{
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

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Reconcile, stamp)
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

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Reconcile, stamp)
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

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Suspend, stamp)
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

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Resume, stamp)
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

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Suspend, stamp)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}

	if readBack(t, client).GetAnnotations()[reconcileAnnotation] != "" {
		t.Fatalf("suspend added a reconcile annotation")
	}
}

func TestClusterScopedTarget(t *testing.T) {
	provider := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "notification.toolkit.fluxcd.io/v1beta3",
		"kind":       "Provider",
		"metadata":   map[string]any{"name": "slack"},
	}}
	client := actionClient(provider)
	ref := api.ObjectRef{
		Group:    "notification.toolkit.fluxcd.io",
		Version:  "v1beta3",
		Resource: "providers",
		Name:     "slack",
	}

	_, err := Do(context.Background(), client, nil, ref, Suspend, stamp)
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

	_, err := Do(context.Background(), client, nil, ref, Reconcile, stamp)

	if err == nil {
		t.Fatalf("expected a non-flux group to be rejected")
	}
}

func TestRejectsUnknownAction(t *testing.T) {
	client := actionClient(newKustomization(false))

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Action("explode"), stamp)

	if err == nil {
		t.Fatalf("expected an unknown action to be rejected")
	}
}

func TestPropagatesAPIError(t *testing.T) {
	client := actionClient()

	_, err := Do(context.Background(), client, nil, kustomizationRef(), Reconcile, stamp)

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

func TestDescriptorLookupIgnoresOtherGroupsAndKinds(t *testing.T) {
	cases := []map[string]api.ResourceDescriptor{
		{
			"deployment": {Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"},
		},
		{
			"helm": {Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories", Kind: "HelmRepository"},
		},
	}
	for _, descs := range cases {
		if desc, found := descriptorOf(descs, "GitRepository"); found {
			t.Fatalf("descriptor = %+v, want no GitRepository", desc)
		}
	}
}

func sourceDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		"gitrepositories": {
			Group:      "source.toolkit.fluxcd.io",
			Version:    "v1",
			Resource:   "gitrepositories",
			Kind:       "GitRepository",
			Namespaced: true,
		},
	}
}

func newSource() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata":   map[string]any{"name": "flux-system", "namespace": "flux-system"},
		"spec":       map[string]any{"url": "https://example.test/infra"},
	}}
}

func sourcedKustomization() *unstructured.Unstructured {
	obj := newKustomization(false)
	_ = unstructured.SetNestedMap(obj.Object, map[string]any{
		"kind": "GitRepository",
		"name": "flux-system",
	}, "spec", "sourceRef")
	return obj
}

func sourceClient(t *testing.T, objs ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{
		kustomizationGVR: "KustomizationList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}: "GitRepositoryList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func TestReconcileWithSourceAsksTheSourceFirst(t *testing.T) {
	client := sourceClient(t, sourcedKustomization(), newSource())

	result, err := Do(t.Context(), client, sourceDescs(), kustomizationRef(), ReconcileSource, stamp)
	if err != nil {
		t.Fatalf("reconcile with source: %v", err)
	}

	if result.RequestedAt == "" {
		t.Fatal("no timestamp was reported back to poll on")
	}
	source, getErr := client.
		Resource(schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}).
		Namespace("flux-system").
		Get(t.Context(), "flux-system", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read the source back: %v", getErr)
	}
	if source.GetAnnotations()[reconcileAnnotation] != result.RequestedAt {
		t.Fatal("the source was not asked to reconcile")
	}
	object, _ := client.Resource(kustomizationGVR).Namespace("flux-system").Get(t.Context(), "apps", metav1.GetOptions{})
	if object.GetAnnotations()[reconcileAnnotation] != result.RequestedAt {
		t.Fatal("the object itself was not asked to reconcile")
	}
}

func TestReconcileWithSourceRefusesAnObjectWithNoSource(t *testing.T) {
	client := sourceClient(t, newKustomization(false))

	_, err := Do(t.Context(), client, sourceDescs(), kustomizationRef(), ReconcileSource, stamp)

	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestReconcileWithSourceRefusesASourceKindTheClusterDoesNotServe(t *testing.T) {
	client := sourceClient(t, sourcedKustomization(), newSource())

	_, err := Do(t.Context(), client, map[string]api.ResourceDescriptor{}, kustomizationRef(), ReconcileSource, stamp)

	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestReconcileWithSourceReportsAnObjectThatIsNotThere(t *testing.T) {
	client := sourceClient(t)

	_, err := Do(t.Context(), client, sourceDescs(), kustomizationRef(), ReconcileSource, stamp)

	if err == nil {
		t.Fatal("expected an error for an object that is not there")
	}
}
