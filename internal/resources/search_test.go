package resources

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func searchScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		panic(err)
	}
	return scheme
}

func meta(group, version, kind, namespace, name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: schema.GroupVersion{Group: group, Version: version}.String(),
			Kind:       kind,
		},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
}

func descriptorsFor(keys ...string) []api.ResourceDescriptor {
	known := map[string]api.ResourceDescriptor{
		"/v1/pods": {Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true},
		"/v1/services": {
			Version: "v1", Resource: "services", Kind: "Service", Namespaced: true,
		},
		"/v1/configmaps": {
			Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true,
		},
		"/v1/secrets": {
			Version: "v1", Resource: "secrets", Kind: "Secret", Namespaced: true,
		},
		"/v1/persistentvolumeclaims": {
			Version: "v1", Resource: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Namespaced: true,
		},
		"apps/v1/deployments": {
			Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
		},
		"apps/v1/replicasets": {
			Group: "apps", Version: "v1", Resource: "replicasets", Kind: "ReplicaSet", Namespaced: true,
		},
	}
	out := make([]api.ResourceDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, known[key])
	}
	return out
}

func fakeMeta(t *testing.T, objects ...runtime.Object) *metadatafake.FakeMetadataClient {
	t.Helper()
	return metadatafake.NewSimpleMetadataClient(searchScheme(), objects...)
}

func names(results api.SearchResults) []string {
	out := make([]string, 0, len(results.Hits))
	for _, hit := range results.Hits {
		out = append(out, hit.Name)
	}
	return out
}

func TestSearchFindsObjectsByPartOfTheirName(t *testing.T) {
	client := fakeMeta(
		t,
		meta("", "v1", "Pod", "airbyte", "airbyte-server-0"),
		meta("", "v1", "Pod", "shop", "web-0"),
	)

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "airbyte", CountLimits{})

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %v, want the airbyte pod only", names(found))
	}
	hit := found.Hits[0]
	if hit.Kind != "Pod" || hit.Namespace != "airbyte" || hit.Resource != "pods" {
		t.Fatalf("hit = %+v", hit)
	}
}

func TestSearchIgnoresCase(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "Pod", "shop", "Airbyte-Worker"))

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "AIRBYTE", CountLimits{})

	if len(found.Hits) != 1 {
		t.Fatalf("a differently cased name was missed: %v", names(found))
	}
}

func TestSearchLooksAcrossSeveralKinds(t *testing.T) {
	client := fakeMeta(
		t,
		meta("", "v1", "Pod", "airbyte", "airbyte-server-0"),
		meta("apps", "v1", "Deployment", "airbyte", "airbyte-server"),
		meta("", "v1", "Service", "airbyte", "airbyte-api"),
	)

	found := Search(
		context.Background(),
		client,
		descriptorsFor("/v1/pods", "apps/v1/deployments", "/v1/services"),
		"airbyte",
		CountLimits{},
	)

	if len(found.Hits) != 3 {
		t.Fatalf("hits = %v, want all three kinds", names(found))
	}
}

func TestSearchLeavesOutKindsItDoesNotSweep(t *testing.T) {
	client := fakeMeta(t, meta("apps", "v1", "ReplicaSet", "airbyte", "airbyte-server-abc"))

	found := Search(
		context.Background(),
		client,
		descriptorsFor("apps/v1/replicasets"),
		"airbyte",
		CountLimits{},
	)

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %v, want none from a kind outside the sweep", names(found))
	}
}

func TestSearchNeedsMoreThanASingleLetter(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "Pod", "shop", "a-pod"))

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "a", CountLimits{})

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %v, want nothing for a one-letter query", names(found))
	}
}

func TestSearchIgnoresSurroundingSpace(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "Pod", "shop", "airbyte-0"))

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "  airbyte  ", CountLimits{})

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %v, want the pod", names(found))
	}
}

func TestSearchSortsSoTheListIsStable(t *testing.T) {
	client := fakeMeta(
		t,
		meta("", "v1", "Pod", "shop", "airbyte-2"),
		meta("", "v1", "Pod", "apps", "airbyte-1"),
		meta("apps", "v1", "Deployment", "shop", "airbyte-dep"),
	)

	found := Search(
		context.Background(),
		client,
		descriptorsFor("/v1/pods", "apps/v1/deployments"),
		"airbyte",
		CountLimits{},
	)

	if names(found)[0] != "airbyte-dep" {
		t.Fatalf("order = %v, want Deployment before Pod", names(found))
	}
	if names(found)[1] != "airbyte-1" {
		t.Fatalf("order = %v, want namespaces in order", names(found))
	}
}

func TestSearchStopsAfterEnoughOfOneKind(t *testing.T) {
	objects := make([]runtime.Object, 0, searchPerKind+5)
	for index := range searchPerKind + 5 {
		objects = append(objects, meta("", "v1", "Pod", "shop", "airbyte-"+string(rune('a'+index))))
	}
	client := fakeMeta(t, objects...)

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "airbyte", CountLimits{})

	if len(found.Hits) != searchPerKind {
		t.Fatalf("hits = %d, want the per-kind cap", len(found.Hits))
	}
	if !found.Truncated {
		t.Fatal("the caller was not told the list was cut short")
	}
}

func TestSearchStopsAfterEnoughResultsAcrossKinds(t *testing.T) {
	objects := make([]runtime.Object, 0, 6*searchPerKind)
	types := []struct {
		group    string
		kind     string
		key      string
		resource string
	}{
		{kind: "Pod", key: "/v1/pods", resource: "pods"},
		{kind: "Service", key: "/v1/services", resource: "services"},
		{kind: "ConfigMap", key: "/v1/configmaps", resource: "configmaps"},
		{kind: "Secret", key: "/v1/secrets", resource: "secrets"},
		{kind: "PersistentVolumeClaim", key: "/v1/persistentvolumeclaims", resource: "persistentvolumeclaims"},
		{group: "apps", kind: "Deployment", key: "apps/v1/deployments", resource: "deployments"},
	}
	keys := make([]string, 0, len(types))
	for _, resourceType := range types {
		keys = append(keys, resourceType.key)
		for index := range searchPerKind {
			name := fmt.Sprintf("airbyte-%s-%02d", resourceType.resource, index)
			objects = append(objects, meta(resourceType.group, "v1", resourceType.kind, "shop", name))
		}
	}
	client := fakeMeta(t, objects...)

	found := Search(context.Background(), client, descriptorsFor(keys...), "airbyte", CountLimits{})

	if len(found.Hits) != searchTotal {
		t.Fatalf("hits = %d, want the global cap", len(found.Hits))
	}
	if !found.Truncated {
		t.Fatal("the caller was not told the combined result was cut short")
	}
	for _, hit := range found.Hits {
		if hit.Kind == "Service" {
			t.Fatalf("sorting and truncation kept a service beyond the first %d hits", searchTotal)
		}
	}
}

func TestSearchSaysWhyAKindCouldNotBeRead(t *testing.T) {
	client := fakeMeta(t)
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})

	found := Search(context.Background(), client, descriptorsFor("/v1/pods"), "airbyte", CountLimits{})

	if found.Errors["/v1/pods"] == "" {
		t.Fatalf("errors = %v, want the refusal", found.Errors)
	}
	if len(found.Hits) != 0 {
		t.Fatalf("hits = %v, want none", names(found))
	}
}

func TestSearchCarriesOnWhenOneKindFails(t *testing.T) {
	client := fakeMeta(t, meta("apps", "v1", "Deployment", "airbyte", "airbyte-server"))
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})

	found := Search(
		context.Background(),
		client,
		descriptorsFor("/v1/pods", "apps/v1/deployments"),
		"airbyte",
		CountLimits{},
	)

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %v, want the deployment", names(found))
	}
}

func TestSearchHasItsOwnLimits(t *testing.T) {
	limits := searchLimits(CountLimits{})

	if limits.Budget != searchTimeout || limits.PerType != searchPerType {
		t.Fatalf("limits = %+v", limits)
	}
	if limits.Concurrency != searchConcurrency {
		t.Fatalf("concurrency = %d", limits.Concurrency)
	}
}

func TestSearchKeepsTheLimitsItWasGiven(t *testing.T) {
	limits := searchLimits(CountLimits{Budget: time.Second, PerType: time.Second, Concurrency: 2})

	if limits.Budget != time.Second || limits.Concurrency != 2 {
		t.Fatalf("limits = %+v", limits)
	}
}

func TestAManagerWithoutAMetadataClientFindsNothing(t *testing.T) {
	manager := &Manager{}

	found := manager.Search(context.Background(), "airbyte")

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %v, want none", names(found))
	}
}

func TestAManagerSearchesTheKindsItKnows(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "Pod", "airbyte", "airbyte-server-0"))
	manager := &Manager{
		meta:  client,
		descs: map[string]api.ResourceDescriptor{"/v1/pods": descriptorsFor("/v1/pods")[0]},
	}

	found := manager.Search(context.Background(), "airbyte")

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %v, want the pod", names(found))
	}
}

func namespacesManager(t *testing.T, objects ...runtime.Object) *Manager {
	t.Helper()
	return NewManager(t.Context(), Deps{Metadata: fakeMeta(t, objects...)})
}

func TestNamespacesComeBackSorted(t *testing.T) {
	mgr := namespacesManager(
		t,
		meta("", "v1", "Namespace", "", "shop"),
		meta("", "v1", "Namespace", "", "argocd"),
		meta("", "v1", "Namespace", "", "kube-system"),
	)

	got := mgr.Namespaces(context.Background())

	want := []string{"argocd", "kube-system", "shop"}
	if len(got.Names) != len(want) {
		t.Fatalf("names = %v, want %v", got.Names, want)
	}
	for i, name := range want {
		if got.Names[i] != name {
			t.Fatalf("names = %v, want %v", got.Names, want)
		}
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none", got.Error)
	}
}

func TestNamespacesCarryTheRefusalInstead(t *testing.T) {
	mgr := namespacesManager(t)
	client, ok := mgr.meta.(*metadatafake.FakeMetadataClient)
	if !ok {
		t.Fatal("the manager is not holding the fake metadata client")
	}
	client.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("namespaces is forbidden")
	})

	got := mgr.Namespaces(context.Background())

	if got.Error == "" {
		t.Fatal("a refused listing came back as an empty cluster")
	}
	if len(got.Names) != 0 {
		t.Fatalf("names = %v, want none", got.Names)
	}
}

func TestNamespacesWithoutAClusterAreEmpty(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{})

	got := mgr.Namespaces(context.Background())

	if len(got.Names) != 0 || got.Error != "" {
		t.Fatalf("namespaces = %+v, want nothing without a cluster", got)
	}
}

func TestNamespacesAreLimitedToWhatTheAccountCanRead(t *testing.T) {
	manager := NewManager(t.Context(), Deps{
		Metadata: namespacesNamed("payments", "kube-system", "storefront"),
		Perms:    boundTo("payments", "storefront"),
	})

	found := manager.Namespaces(alice(t))

	want := []string{"payments", "storefront"}
	if len(found.Names) != len(want) {
		t.Fatalf("names = %v, want %v", found.Names, want)
	}
	for index, name := range want {
		if found.Names[index] != name {
			t.Fatalf("names = %v, want %v", found.Names, want)
		}
	}
}
