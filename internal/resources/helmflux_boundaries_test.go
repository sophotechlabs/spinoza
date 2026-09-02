package resources

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func TestFluxOwnersIgnoreOtherResourcesInTheFluxAPIGroup(t *testing.T) {
	desc := api.ResourceDescriptor{
		Group:      helm.FluxGroup,
		Version:    "v1beta2",
		Resource:   "helmcharts",
		Kind:       "HelmChart",
		Namespaced: true,
	}
	mgr := NewManager(t.Context(), Deps{
		Descriptors: map[string]api.ResourceDescriptor{
			discovery.Key(desc.Group, desc.Version, desc.Resource): desc,
		},
	})

	if got := mgr.fluxOwners(t.Context()); len(got) != 0 {
		t.Fatalf("owners = %v, want a non-HelmRelease resource ignored", got)
	}
}

func TestFluxOwnerDiscoveryIgnoresAHelmReleaseListThatCannotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	desc := api.ResourceDescriptor{
		Group:      helm.FluxGroup,
		Version:    "v2",
		Resource:   helm.FluxResource,
		Kind:       "HelmRelease",
		Namespaced: true,
	}
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "HelmReleaseList"},
	)
	mgr := NewManager(ctx, Deps{
		Dynamic: dyn,
		Descriptors: map[string]api.ResourceDescriptor{
			discovery.Key(desc.Group, desc.Version, desc.Resource): desc,
		},
	})

	if got := mgr.fluxOwners(ctx); len(got) != 0 {
		t.Fatalf("owners = %v, want none from a list that could not start", got)
	}
}
