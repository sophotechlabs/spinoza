package kube

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func podsResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Version: "v1", Resource: "pods"}
}

func listEverything() metav1.ListOptions {
	return metav1.ListOptions{}
}

func kubeconfigFor(server string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- name: test-cluster
  cluster:
    server: %s
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: test-token
`, server)
}

// Every client in a bundle is built on one config, so wrapping that config's
// transport is what makes a failed list, watch or read say something about the
// cluster without anyone having to remember to report it.
func TestWhatTheClientsRunIntoIsReported(t *testing.T) {
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"31","gitVersion":"v1.31.0"}`))
	}))
	t.Setenv("KUBECONFIG", writeKubeconfig(t, kubeconfigFor(apiserver.URL)))
	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if bundle.Reach == nil {
		t.Fatal("the bundle carries nothing to report what its clients run into")
	}

	_, versionErr := bundle.Clientset.Discovery().ServerVersion()
	if versionErr != nil {
		t.Fatalf("server version: %v", versionErr)
	}

	if answering, _ := bundle.Reach.State(); !answering {
		t.Fatal("a cluster that answered was reported as gone")
	}

	apiserver.Close()
	_, goneErr := bundle.Clientset.Discovery().ServerVersion()

	if goneErr == nil {
		t.Fatal("a cluster that had gone answered anyway")
	}
	answering, reason := bundle.Reach.State()
	if answering {
		t.Fatal("a cluster that had gone was still reported as answering")
	}
	if !strings.Contains(reason, apiserver.Listener.Addr().String()) {
		t.Fatalf("reason = %q, want it to name what could not be reached", reason)
	}
}

// The dynamic client is built from the same config as the typed one, and a
// list is what most of spinoza actually does.
func TestWhatTheDynamicClientRunsIntoIsReportedToo(t *testing.T) {
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Setenv("KUBECONFIG", writeKubeconfig(t, kubeconfigFor(apiserver.URL)))
	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	apiserver.Close()

	_, listErr := bundle.Dynamic.
		Resource(podsResource()).
		Namespace("default").
		List(t.Context(), listEverything())

	if listErr == nil {
		t.Fatal("a list against a cluster that had gone came back fine")
	}
	if answering, _ := bundle.Reach.State(); answering {
		t.Fatal("a failed list said nothing about the cluster")
	}
}
