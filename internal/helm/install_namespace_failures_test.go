package helm

import (
	"errors"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestAPreviewWithoutAClientRendersLocally(t *testing.T) {
	runner := &stubRunner{out: `{"manifest":"kind: Service\n"}`}
	svc := NewService(
		nil,
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)
	req := installRequest()
	req.DryRun = true

	_, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !slices.Contains(runner.args[0], "--dry-run=client") {
		t.Fatalf("args = %v, want a local render without a Kubernetes client", runner.args[0])
	}
}

func TestAPreviewWithAnUnreadableNamespaceRendersLocally(t *testing.T) {
	runner := &stubRunner{out: `{"manifest":"kind: Service\n"}`}
	svc := installer(t, runner, namespaceObject("demo"))
	client, ok := svc.cs.(*k8sfake.Clientset)
	if !ok {
		t.Fatalf("client = %T, want the fake clientset", svc.cs)
	}
	client.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("namespaces are forbidden")
	})
	req := installRequest()
	req.DryRun = true

	_, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !slices.Contains(runner.args[0], "--dry-run=client") {
		t.Fatalf("args = %v, want a local render when namespace discovery fails", runner.args[0])
	}
}
