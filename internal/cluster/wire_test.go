package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

const deadKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
  name: dead
contexts:
- context:
    cluster: dead
    user: nobody
  name: dead
current-context: dead
users:
- name: nobody
  user: {}
`

var widgetGVR = schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}

func TestNewStartsWithoutAClusterWhenNothingAnswers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(deadKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	built, err := New(ctx, Options{Kubeconfig: path})
	if err != nil {
		t.Fatalf("new: %v, want the startup failure kept for the ui instead", err)
	}
	if built.Manager("") != nil {
		t.Fatal("a manager appeared with no cluster answering")
	}
	if built.Current().Name != "" {
		t.Fatalf("current = %q, want no context installed", built.Current().Name)
	}
	contexts := built.Contexts()
	if !strings.Contains(contexts.Error, "lists no resource types") {
		t.Fatalf("error = %q, want the unreachable context named", contexts.Error)
	}
	if !strings.Contains(contexts.Error, "dead") {
		t.Fatalf("error = %q, want the context name in it", contexts.Error)
	}
}

func TestNewRefusesAnExplicitContextThatCannotBeReached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(deadKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	_, err := New(t.Context(), Options{Kubeconfig: path, Context: "dead"})

	if err == nil {
		t.Fatal("an explicitly requested unreachable context was ignored")
	}
	if !strings.Contains(err.Error(), "dead") {
		t.Fatalf("error = %v, want the requested context named", err)
	}
}

func TestBuildNamesAKubeconfigItCannotLoad(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	manager, bundle, err := build(
		t.Context(),
		api.ContextRef{Name: "gone"},
		Options{Kubeconfig: missing},
		prom.Target{},
	)

	if err == nil {
		t.Fatal("a missing kubeconfig produced a cluster connection")
	}
	if manager != nil || bundle != nil {
		t.Fatalf("manager = %v, bundle = %v, want neither after setup failed", manager, bundle)
	}
	if !strings.Contains(err.Error(), "kube:") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want the failed kubeconfig operation and path", err)
	}
}

func TestNewRefusesABadPrometheusSpec(t *testing.T) {
	_, err := New(context.Background(), Options{PromSpec: "no-slash"})
	if err == nil {
		t.Fatal("expected the prom spec to be refused")
	}
	if !strings.Contains(err.Error(), "namespace/service:port") {
		t.Fatalf("err = %v, want the expected shape named", err)
	}
}

func widget(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"namespace": namespace,
			"name":      name,
		},
	}}
}

func TestObjectsInScopesNamespacedResources(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(),
		widget("alpha", "one"), widget("beta", "two"))

	listed, err := objectsIn(client, widgetGVR, "alpha").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].GetName() != "one" {
		t.Fatalf("items = %+v, want only alpha/one", listed.Items)
	}
}

func TestObjectsInLeavesClusterScopedResourcesUnscoped(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(),
		widget("", "one"), widget("", "two"))

	listed, err := objectsIn(client, widgetGVR, "").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed.Items) != 2 {
		t.Fatalf("items = %d, want both cluster-scoped objects", len(listed.Items))
	}
}

func TestMetadataClientRejectsAnInvalidAPIAddress(t *testing.T) {
	client := metaClient(&kube.Bundle{Config: &rest.Config{Host: "://invalid"}})

	if client != nil {
		t.Fatal("an invalid API address produced a metadata client")
	}
}

func TestMetadataClientBuildsFromAValidAPIAddress(t *testing.T) {
	client := metaClient(&kube.Bundle{Config: &rest.Config{Host: "https://cluster.example"}})

	if client == nil {
		t.Fatal("a valid API address produced no metadata client")
	}
}
