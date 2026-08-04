package resources

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func countDesc(resource string) api.ResourceDescriptor {
	return api.ResourceDescriptor{Group: "apps", Version: "v1", Resource: resource, Kind: "Deployment"}
}

func countClient(t *testing.T, objs ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func countObject(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": name, "namespace": "default"},
	}}
}

func TestCountReportsZeroForAnEmptyType(t *testing.T) {
	counts := Count(context.Background(), countClient(t), []api.ResourceDescriptor{countDesc("deployments")})

	if counts["apps/v1/deployments"] != 0 {
		t.Fatalf("count = %d, want 0", counts["apps/v1/deployments"])
	}
}

func TestCountAddsWhatThePageLeftBehind(t *testing.T) {
	objs := []runtime.Object{countObject("a"), countObject("b"), countObject("c")}
	counts := Count(context.Background(), countClient(t, objs...), []api.ResourceDescriptor{countDesc("deployments")})

	if counts["apps/v1/deployments"] != 3 {
		t.Fatalf("count = %d, want 3", counts["apps/v1/deployments"])
	}
}

func TestCountReportsUnknownWhenTheListIsRefused(t *testing.T) {
	dyn := countClient(t)
	dyn.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{countDesc("deployments")})

	if counts["apps/v1/deployments"] != countUnknown {
		t.Fatalf("count = %d, want the unknown marker so the browser does not claim it is empty", counts["apps/v1/deployments"])
	}
}

func TestCountCoversEveryTypeItWasGiven(t *testing.T) {
	descs := []api.ResourceDescriptor{countDesc("deployments"), countDesc("statefulsets")}

	counts := Count(context.Background(), countClient(t), descs)

	if len(counts) != 2 {
		t.Fatalf("counts = %+v, want one entry per type", counts)
	}
}

func TestRemainingOfIgnoresANegativeOrAbsentCount(t *testing.T) {
	negative := int64(-4)
	if remainingOf(&negative) != 0 {
		t.Fatal("a negative remainder was added to the total")
	}
	if remainingOf(nil) != 0 {
		t.Fatal("an absent remainder was not treated as zero")
	}
}
