package argocd

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

func actionClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		applicationGVR: "ApplicationList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func newApplication() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "podinfo",
			"namespace": "argocd",
		},
		"spec": map[string]any{
			"project": "default",
			"source":  map[string]any{"repoURL": "https://example.test/apps", "path": "podinfo"},
		},
	}}
}

func applicationRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: "argocd",
		Name:      "podinfo",
	}
}

func readBack(t *testing.T, client *fake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(applicationGVR).
		Namespace("argocd").
		Get(t.Context(), "podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestSyncAsksTheControllerForAnOperation(t *testing.T) {
	client := actionClient(newApplication())

	result, err := Do(t.Context(), client, applicationRef(), Sync)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	if result.Action != "sync" {
		t.Fatalf("action = %q, want sync", result.Action)
	}
	got := readBack(t, client)
	operation, found, _ := unstructured.NestedMap(got.Object, "operation")
	if !found {
		t.Fatal("sync wrote no operation for the controller to pick up")
	}
	if _, ok := operation["sync"]; !ok {
		t.Fatalf("operation = %v, want a sync in it", operation)
	}
	who, _, _ := unstructured.NestedString(got.Object, "operation", "initiatedBy", "username")
	if who != fieldManager {
		t.Fatalf("initiatedBy.username = %q, want %q", who, fieldManager)
	}
}

func TestSyncLeavesTheSpecAlone(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Sync)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := readBack(t, client)
	project, _, _ := unstructured.NestedString(got.Object, "spec", "project")
	if project != "default" {
		t.Fatalf("spec.project = %q, want it untouched", project)
	}
	if got.GetAnnotations()[refreshAnnotation] != "" {
		t.Fatal("sync also asked for a refresh")
	}
}

func TestRefreshSetsTheAnnotationTheControllerWatches(t *testing.T) {
	client := actionClient(newApplication())

	result, err := Do(t.Context(), client, applicationRef(), Refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if result.Action != "refresh" {
		t.Fatalf("action = %q, want refresh", result.Action)
	}
	got := readBack(t, client)
	if got.GetAnnotations()[refreshAnnotation] != normalRefresh {
		t.Fatalf("annotation = %q, want %q", got.GetAnnotations()[refreshAnnotation], normalRefresh)
	}
}

func TestRefreshStartsNoSync(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if _, found, _ := unstructured.NestedMap(readBack(t, client).Object, "operation"); found {
		t.Fatal("refresh queued a sync operation")
	}
}

func TestRejectsANonArgoGroup(t *testing.T) {
	client := actionClient()
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "d", Name: "web"}

	_, err := Do(t.Context(), client, ref, Sync)

	want := `"apps" is not an argo cd resource group`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestRejectsKindsTheControllerCannotSync(t *testing.T) {
	client := actionClient()
	ref := api.ObjectRef{Group: Group, Version: "v1alpha1", Resource: appProjects, Namespace: "argocd", Name: "default"}

	_, err := Do(t.Context(), client, ref, Sync)

	want := `only applications can be synced or refreshed, not "appprojects"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestRejectsAnUnknownAction(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Action("explode"))

	want := `unknown action "explode"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestReportsWhatTheAPIServerSaid(t *testing.T) {
	client := actionClient()

	result, err := Do(t.Context(), client, applicationRef(), Sync)

	if err == nil {
		t.Fatal("expected an error patching an application that is not there")
	}
	if result.Action != "sync" {
		t.Fatalf("action = %q, want the attempted action back", result.Action)
	}
}

func TestClusterScopedApplicationsPatchWithoutANamespace(t *testing.T) {
	app := newApplication()
	app.SetNamespace("")
	client := actionClient(app)
	ref := applicationRef()
	ref.Namespace = ""

	_, err := Do(t.Context(), client, ref, Refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	got, getErr := client.Resource(applicationGVR).Get(t.Context(), "podinfo", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	if got.GetAnnotations()[refreshAnnotation] != normalRefresh {
		t.Fatal("the cluster-scoped patch did not land")
	}
}

func TestIsArgoGroup(t *testing.T) {
	cases := map[string]bool{
		"argoproj.io":             true,
		"apps":                    false,
		"":                        false,
		"argoproj.io.example.com": false,
	}
	for group, want := range cases {
		if got := IsArgoGroup(group); got != want {
			t.Fatalf("IsArgoGroup(%q) = %v, want %v", group, got, want)
		}
	}
}
