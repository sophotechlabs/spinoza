package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func catalog() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "configmaps"): {
			Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true,
		},
		discovery.Key("apps", "v1", "deployments"): {
			Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
		},
		discovery.Key("", "v1", "nodes"): {Version: "v1", Resource: "nodes", Kind: "Node"},
	}
}

func viewManager(t *testing.T, releases *helm.Service) *Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Version: "v1", Resource: "nodes"}:                               "NodeList",
			{Version: "v1", Resource: "pods"}:                                "PodList",
			{Version: "v1", Resource: "events"}:                              "EventList",
			{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		})
	return NewManager(ctx, Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Helm:        releases,
		Descriptors: catalog(),
	})
}

func TestTheManagerResolvesAKindToItsPluralResource(t *testing.T) {
	mgr := viewManager(t, nil)

	found, ok := mgr.resolveKind("apps/v1", "Deployment")

	if !ok {
		t.Fatal("a kind discovery reports was not resolved")
	}
	if found.Group != "apps" || found.Version != "v1" || found.Resource != "deployments" {
		t.Fatalf("resolved %+v, want apps/v1/deployments", found)
	}
	if !found.Namespaced {
		t.Fatal("a namespaced kind was reported as cluster-scoped")
	}
}

func TestTheManagerResolvesACoreKind(t *testing.T) {
	mgr := viewManager(t, nil)

	found, ok := mgr.resolveKind("v1", "ConfigMap")

	if !ok {
		t.Fatal("a core kind was not resolved")
	}
	if found.Group != "" || found.Resource != "configmaps" {
		t.Fatalf("resolved %+v, want the core configmaps", found)
	}
}

func TestTheManagerRefusesAKindItDoesNotKnow(t *testing.T) {
	mgr := viewManager(t, nil)

	found, ok := mgr.resolveKind("acme.io/v1", "Widget")

	if ok {
		t.Fatal("an unknown kind was resolved")
	}
	if found.Resource != "" {
		t.Fatalf("resource = %q, want nothing", found.Resource)
	}
}

func TestTheManagerRefusesTheRightKindInTheWrongGroup(t *testing.T) {
	mgr := viewManager(t, nil)

	found, ok := mgr.resolveKind("other/v1", "Deployment")

	if ok {
		t.Fatal("a kind from another api group was resolved")
	}
	if found.Resource != "" {
		t.Fatalf("resource = %q, want nothing", found.Resource)
	}
}

func TestABareApiVersionIsAVersionNotAGroup(t *testing.T) {
	mgr := viewManager(t, nil)

	wrong, asGroup := mgr.resolveKind("apps", "Deployment")
	node, asVersion := mgr.resolveKind("v1", "Node")

	if asGroup {
		t.Fatalf("a bare apiVersion was read as an api group, resolving %+v", wrong)
	}
	if !asVersion {
		t.Fatal("a bare apiVersion was not read as a core version")
	}
	if node.Resource != "nodes" {
		t.Fatalf("resource = %q, want nodes", node.Resource)
	}
	if node.Namespaced {
		t.Fatal("nodes were reported as namespaced")
	}
}

func TestTheManagerBuildsAnOverview(t *testing.T) {
	mgr := viewManager(t, nil)

	got := mgr.Overview(context.Background())

	if got.Nodes.Total != 0 {
		t.Fatalf("nodes = %d, want none from an empty fake", got.Nodes.Total)
	}
	if got.Warnings == nil {
		t.Fatal("warnings should be an empty list, not nil")
	}
}

func TestHelmMethodsSayWhenHelmIsNotWiredUp(t *testing.T) {
	mgr := viewManager(t, nil)

	_, listErr := mgr.HelmReleases(context.Background())
	_, detailErr := mgr.HelmRelease(context.Background(), "demo", "podinfo", 0)
	_, rollbackErr := mgr.HelmRollback(context.Background(), "demo", "podinfo", 1)
	_, uninstallErr := mgr.HelmUninstall(context.Background(), "demo", "podinfo")
	support := mgr.HelmSupport()

	for name, err := range map[string]error{
		"list":      listErr,
		"detail":    detailErr,
		"rollback":  rollbackErr,
		"uninstall": uninstallErr,
	} {
		if err == nil {
			t.Fatalf("%s reported success with no helm service", name)
		}
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("%s err = %v, want an internal failure", name, err)
		}
	}
	if support.Available {
		t.Fatal("support claimed helm works with no service")
	}
	if !strings.Contains(support.Reason, "not wired up") {
		t.Fatalf("reason = %q, want it to say helm is not wired up", support.Reason)
	}
}

func TestHelmMethodsReachTheService(t *testing.T) {
	cs := k8sfake.NewClientset()
	releases := helm.NewService(cs, helmMeta(t, cs), nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})
	mgr := viewManager(t, releases)

	list, err := mgr.HelmReleases(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Releases) != 0 {
		t.Fatalf("releases = %v, want none", list.Releases)
	}

	_, detailErr := mgr.HelmRelease(context.Background(), "demo", "podinfo", 0)
	if detailErr == nil {
		t.Fatal("a release that is not there reported success")
	}

	support := mgr.HelmSupport()
	if support.Available {
		t.Fatal("a service with no runner claimed helm works")
	}

	_, rollbackErr := mgr.HelmRollback(context.Background(), "demo", "podinfo", 1)
	_, uninstallErr := mgr.HelmUninstall(context.Background(), "demo", "podinfo")
	if rollbackErr == nil || uninstallErr == nil {
		t.Fatal("an action with no runner reported success")
	}
}

func TestTheManagerRefusesAKindAtAVersionItDoesNotServe(t *testing.T) {
	mgr := viewManager(t, nil)

	found, ok := mgr.resolveKind("apps/v1beta1", "Deployment")

	if ok {
		t.Fatalf("resolved %+v, want nothing for a version discovery never listed", found)
	}
}

func TestAHelmListThatFailsIsReported(t *testing.T) {
	releases := helm.NewService(
		k8sfake.NewClientset(),
		nil,
		helm.NewRunner("helm"),
		nil,
		nil,
		api.ContextRef{Name: "p-mk1"},
	)
	mgr := viewManager(t, releases)

	list, err := mgr.HelmReleases(t.Context())

	if err == nil {
		t.Fatalf("list = %+v, want the failure reported", list)
	}
	if list.Releases == nil {
		t.Fatal("the browser iterates the releases without a guard")
	}
}

func TestTheOverviewCarriesTheClusterVersionFromDiscovery(t *testing.T) {
	mgr := viewManager(t, nil)
	mgr.UseDiscovery(&stubDiscovery{version: "v1.36.2+k3s1"}, nil)

	got := mgr.Overview(t.Context())

	if got.Version != "v1.36.2+k3s1" {
		t.Fatalf("version = %q, want what discovery reported", got.Version)
	}
}
