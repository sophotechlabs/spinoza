package resources

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

func deploymentDescriptor() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      "apps",
		Version:    "v1",
		Resource:   "deployments",
		Kind:       "Deployment",
		Namespaced: true,
	}
}

func TestListNamesCarriesIdentityOwnershipAndManager(t *testing.T) {
	owned := meta("apps", "v1", "Deployment", "prod", "web")
	owned.OwnerReferences = []metav1.OwnerReference{{Name: "web-app"}}
	owned.Labels = map[string]string{"helm.toolkit.fluxcd.io/name": "web"}
	handmade := meta("apps", "v1", "Deployment", "staging", "api")
	handmade.Labels = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
	manager := NewManager(t.Context(), Deps{Metadata: fakeMeta(t, owned, handmade)})

	items, err := manager.ListNames(t.Context(), deploymentDescriptor())
	if err != nil {
		t.Fatalf("list names: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	byName := map[string]checks.Named{}
	for _, item := range items {
		byName[item.Ref.Name] = item
	}
	web := byName["web"]
	if web.Ref.Namespace != "prod" || web.Ref.Group != "apps" || web.Ref.Resource != "deployments" {
		t.Fatalf("web ref = %+v", web.Ref)
	}
	if !web.Owned || web.Manager != "Flux" {
		t.Fatalf("web = owned:%t manager:%q", web.Owned, web.Manager)
	}
	apiDeployment := byName["api"]
	if apiDeployment.Owned || apiDeployment.Manager != "Helm" {
		t.Fatalf("api = owned:%t manager:%q", apiDeployment.Owned, apiDeployment.Manager)
	}
}

func TestListNamesReturnsMetadataFailures(t *testing.T) {
	client := fakeMeta(t)
	client.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployment metadata is forbidden")
	})
	manager := NewManager(t.Context(), Deps{Metadata: client})

	_, err := manager.ListNames(t.Context(), deploymentDescriptor())

	if err == nil || err.Error() != "deployment metadata is forbidden" {
		t.Fatalf("error = %v", err)
	}
}

func TestScanReturnsIndependentObjectsFromEveryNamespace(t *testing.T) {
	client := newClient(
		t,
		newDeployment("prod", "web"),
		newDeployment("staging", "api"),
	)
	manager := NewManager(t.Context(), Deps{Dynamic: client})

	items, err := manager.Scan(t.Context(), deploymentDescriptor())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	namespaces := map[string]bool{}
	for _, item := range items {
		namespaces[item.GetNamespace()] = true
	}
	if !namespaces["prod"] || !namespaces["staging"] {
		t.Fatalf("namespaces = %v", namespaces)
	}
	firstName := items[0].GetName()
	items[1].SetName("changed")
	if items[0].GetName() != firstName {
		t.Fatal("scan results shared object storage")
	}
}

func TestScanReturnsDynamicClientFailures(t *testing.T) {
	client := newClient(t)
	client.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments are unavailable")
	})
	manager := NewManager(t.Context(), Deps{Dynamic: client})

	_, err := manager.Scan(t.Context(), deploymentDescriptor())

	if err == nil || err.Error() != "deployments are unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestListAllSkipsObjectsItCannotTurnIntoRows(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(newDeployment("prod", "web")); err != nil {
		t.Fatalf("add deployment: %v", err)
	}
	if err := indexer.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "typed", Namespace: "prod"}}); err != nil {
		t.Fatalf("add typed pod: %v", err)
	}
	lister := cache.NewGenericLister(indexer, schema.GroupResource{Group: "apps", Resource: "deployments"})

	items, err := listAll(lister)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(items) != 1 || items[0].GetName() != "web" {
		t.Fatalf("items = %+v, want only the unstructured deployment", items)
	}
}

type failedGenericLister struct {
	err error
}

func (f failedGenericLister) List(labels.Selector) ([]runtime.Object, error) {
	return nil, f.err
}

func (f failedGenericLister) Get(string) (runtime.Object, error) {
	return nil, f.err
}

func (f failedGenericLister) ByNamespace(string) cache.GenericNamespaceLister {
	return nil
}

func TestListAllReturnsListerFailures(t *testing.T) {
	want := errors.New("cache index failed")

	_, err := listAll(failedGenericLister{err: want})

	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestListNamesUsesTheRequestedResource(t *testing.T) {
	client := fakeMeta(t)
	var action k8stesting.ListAction
	client.PrependReactor("list", "deployments", func(got k8stesting.Action) (bool, runtime.Object, error) {
		listed, ok := got.(k8stesting.ListAction)
		if !ok {
			t.Fatalf("action = %T, want ListAction", got)
		}
		action = listed
		return false, nil, nil
	})
	manager := NewManager(t.Context(), Deps{Metadata: client})

	_, err := manager.ListNames(t.Context(), deploymentDescriptor())
	if err != nil {
		t.Fatalf("list names: %v", err)
	}
	if action == nil {
		t.Fatal("metadata client saw no list action")
	}
	if action.GetResource() != depGVR {
		t.Fatalf("resource = %s, want %s", action.GetResource(), depGVR)
	}
	if action.GetNamespace() != metav1.NamespaceAll {
		t.Fatalf("namespace = %q, want all", action.GetNamespace())
	}
}
