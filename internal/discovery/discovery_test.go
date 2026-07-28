package discovery

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientdiscovery "k8s.io/client-go/discovery"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubDiscovery struct {
	clientdiscovery.DiscoveryInterface
	lists []*metav1.APIResourceList
	err   error
}

func (s *stubDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return s.lists, s.err
}

func listWatch() metav1.Verbs {
	return metav1.Verbs{"get", "list", "watch"}
}

func coreList() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: listWatch()},
			{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get"}},
			{Name: "services", Kind: "Service", Namespaced: true, Verbs: listWatch()},
			{Name: "nodes", Kind: "Node", Namespaced: false, Verbs: listWatch()},
			{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: listWatch()},
			{Name: "persistentvolumes", Kind: "PersistentVolume", Namespaced: false, Verbs: listWatch()},
			{Name: "serviceaccounts", Kind: "ServiceAccount", Namespaced: true, Verbs: listWatch()},
			{Name: "componentstatuses", Kind: "ComponentStatus", Namespaced: false, Verbs: listWatch()},
			{Name: "bindings", Kind: "Binding", Namespaced: true, Verbs: metav1.Verbs{"create"}},
		},
	}
}

func appsList() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{
			{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: listWatch()},
			{Name: "daemonsets", Kind: "DaemonSet", Namespaced: true, Verbs: listWatch()},
		},
	}
}

func crdList() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "example.com/v1alpha1",
		APIResources: []metav1.APIResource{
			{Name: "widgets", Kind: "Widget", Namespaced: true, Verbs: listWatch()},
		},
	}
}

func fullLists() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{coreList(), appsList(), crdList()}
}

func catByName(cats []api.Category, name string) (api.Category, bool) {
	for _, c := range cats {
		if c.Name == name {
			return c, true
		}
	}
	return api.Category{}, false
}

func hasResource(cat api.Category, resource string) bool {
	for _, r := range cat.Resources {
		if r.Resource == resource {
			return true
		}
	}
	return false
}

func TestKey(t *testing.T) {
	result := Key("apps", "v1", "deployments")
	expected := "apps/v1/deployments"
	if result != expected {
		t.Fatalf("Key = %q, want %q", result, expected)
	}
}

func TestKeyCoreGroup(t *testing.T) {
	result := Key("", "v1", "pods")
	expected := "/v1/pods"
	if result != expected {
		t.Fatalf("Key = %q, want %q", result, expected)
	}
}

func TestSupportsListWatch(t *testing.T) {
	cases := []struct {
		name  string
		verbs metav1.Verbs
		want  bool
	}{
		{"list and watch", metav1.Verbs{"get", "list", "watch"}, true},
		{"list only", metav1.Verbs{"get", "list"}, false},
		{"watch only", metav1.Verbs{"watch"}, false},
		{"neither", metav1.Verbs{"get", "create"}, false},
		{"empty", metav1.Verbs{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := supportsListWatch(tc.verbs)
			if result != tc.want {
				t.Fatalf("supportsListWatch(%v) = %v, want %v", tc.verbs, result, tc.want)
			}
		})
	}
}

func TestCategoryFor(t *testing.T) {
	cases := []struct {
		group    string
		resource string
		want     string
	}{
		{"apps", "deployments", "Workloads"},
		{"batch", "jobs", "Workloads"},
		{"networking.k8s.io", "ingresses", "Network"},
		{"discovery.k8s.io", "endpointslices", "Network"},
		{"storage.k8s.io", "storageclasses", "Storage"},
		{"rbac.authorization.k8s.io", "roles", "Access Control"},
		{"autoscaling", "horizontalpodautoscalers", "Config"},
		{"policy", "poddisruptionbudgets", "Config"},
		{"scheduling.k8s.io", "priorityclasses", "Config"},
		{"node.k8s.io", "runtimeclasses", "Config"},
		{"coordination.k8s.io", "leases", "Config"},
		{"apiextensions.k8s.io", "customresourcedefinitions", "Cluster"},
		{"apiregistration.k8s.io", "apiservices", "Cluster"},
		{"admissionregistration.k8s.io", "validatingwebhookconfigurations", "Cluster"},
		{"certificates.k8s.io", "certificatesigningrequests", "Cluster"},
		{"authentication.k8s.io", "tokenreviews", "Cluster"},
		{"authorization.k8s.io", "subjectaccessreviews", "Cluster"},
		{"flowcontrol.apiserver.k8s.io", "flowschemas", "Cluster"},
		{"events.k8s.io", "events", "Cluster"},
		{"example.com", "widgets", "Custom Resources"},
		{"cert-manager.io", "certificates", "Custom Resources"},
	}
	for _, tc := range cases {
		t.Run(tc.group+"/"+tc.resource, func(t *testing.T) {
			result := categoryFor(tc.group, tc.resource)
			if result != tc.want {
				t.Fatalf("categoryFor(%q, %q) = %q, want %q", tc.group, tc.resource, result, tc.want)
			}
		})
	}
}

func TestCoreCategoryFor(t *testing.T) {
	cases := []struct {
		resource string
		want     string
	}{
		{"pods", "Workloads"},
		{"replicationcontrollers", "Workloads"},
		{"configmaps", "Config"},
		{"secrets", "Config"},
		{"resourcequotas", "Config"},
		{"limitranges", "Config"},
		{"services", "Network"},
		{"endpoints", "Network"},
		{"persistentvolumeclaims", "Storage"},
		{"persistentvolumes", "Storage"},
		{"serviceaccounts", "Access Control"},
		{"namespaces", "Cluster"},
		{"nodes", "Cluster"},
		{"componentstatuses", "Cluster"},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			result := coreCategoryFor(tc.resource)
			if result != tc.want {
				t.Fatalf("coreCategoryFor(%q) = %q, want %q", tc.resource, result, tc.want)
			}
		})
	}
}

func TestListCategorizes(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists()}
	cats, byKey, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	workloads, ok := catByName(cats, "Workloads")
	if !ok {
		t.Fatal("missing Workloads category")
	}
	if !hasResource(workloads, "pods") {
		t.Fatal("Workloads missing pods")
	}
	if !hasResource(workloads, "deployments") {
		t.Fatal("Workloads missing deployments")
	}
	if !hasResource(workloads, "daemonsets") {
		t.Fatal("Workloads missing daemonsets")
	}

	network, ok := catByName(cats, "Network")
	if !ok {
		t.Fatal("missing Network category")
	}
	if !hasResource(network, "services") {
		t.Fatal("Network missing services")
	}

	config, ok := catByName(cats, "Config")
	if !ok {
		t.Fatal("missing Config category")
	}
	if !hasResource(config, "configmaps") {
		t.Fatal("Config missing configmaps")
	}

	storage, ok := catByName(cats, "Storage")
	if !ok {
		t.Fatal("missing Storage category")
	}
	if !hasResource(storage, "persistentvolumes") {
		t.Fatal("Storage missing persistentvolumes")
	}

	access, ok := catByName(cats, "Access Control")
	if !ok {
		t.Fatal("missing Access Control category")
	}
	if !hasResource(access, "serviceaccounts") {
		t.Fatal("Access Control missing serviceaccounts")
	}

	cluster, ok := catByName(cats, "Cluster")
	if !ok {
		t.Fatal("missing Cluster category")
	}
	if !hasResource(cluster, "nodes") {
		t.Fatal("Cluster missing nodes")
	}
	if !hasResource(cluster, "componentstatuses") {
		t.Fatal("Cluster missing componentstatuses")
	}

	custom, ok := catByName(cats, "Custom Resources")
	if !ok {
		t.Fatal("missing Custom Resources category")
	}
	if !hasResource(custom, "widgets") {
		t.Fatal("Custom Resources missing widgets")
	}

	desc, ok := byKey[Key("apps", "v1", "deployments")]
	if !ok {
		t.Fatal("byKey missing apps/v1/deployments")
	}
	if desc.Kind != "Deployment" {
		t.Fatalf("Kind = %q, want Deployment", desc.Kind)
	}
	if desc.Category != "Workloads" {
		t.Fatalf("Category = %q, want Workloads", desc.Category)
	}
	if !desc.Namespaced {
		t.Fatal("deployments Namespaced = false, want true")
	}
}

func TestListSkipsSubresources(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists()}
	_, byKey, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, ok := byKey[Key("", "v1", "pods/log")]; ok {
		t.Fatal("byKey contains subresource pods/log, want skipped")
	}
}

func TestListSkipsNonListWatch(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists()}
	_, byKey, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, ok := byKey[Key("", "v1", "bindings")]; ok {
		t.Fatal("byKey contains bindings (create-only), want skipped")
	}
}

func TestListOrdersCategories(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists()}
	cats, _, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, c.Name)
	}
	expected := []string{"Cluster", "Workloads", "Config", "Network", "Storage", "Access Control", "Custom Resources"}
	if len(names) != len(expected) {
		t.Fatalf("categories = %v, want %v", names, expected)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("category order = %v, want %v", names, expected)
		}
	}
}

func TestListOrdersWorkloadsByEverydayUse(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists()}
	cats, _, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	workloads, ok := catByName(cats, "Workloads")
	if !ok {
		t.Fatal("missing Workloads category")
	}
	resources := make([]string, 0, len(workloads.Resources))
	for _, r := range workloads.Resources {
		resources = append(resources, r.Resource)
	}
	expected := []string{"pods", "deployments", "daemonsets"}
	for i := range expected {
		if resources[i] != expected[i] {
			t.Fatalf("Workloads resources = %v, want %v first", resources, expected)
		}
	}
}

func TestListPutsUnrankedKindsAfterRankedOnes(t *testing.T) {
	lists := []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "controllerrevisions", Kind: "ControllerRevision", Namespaced: true, Verbs: listWatch()},
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: listWatch()},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: listWatch()},
			},
		},
	}
	cats, _, err := List(&stubDiscovery{lists: lists})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	workloads, ok := catByName(cats, "Workloads")
	if !ok {
		t.Fatal("missing Workloads category")
	}

	kinds := make([]string, 0, len(workloads.Resources))
	for _, r := range workloads.Resources {
		kinds = append(kinds, r.Kind)
	}
	want := []string{"Pod", "Deployment", "ControllerRevision"}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
}

func TestListSortsUnrankedCategoriesAlphabetically(t *testing.T) {
	lists := []*metav1.APIResourceList{
		{
			GroupVersion: "cilium.io/v2",
			APIResources: []metav1.APIResource{
				{Name: "ciliumnetworkpolicies", Kind: "CiliumNetworkPolicy", Namespaced: true, Verbs: listWatch()},
				{Name: "ciliumendpoints", Kind: "CiliumEndpoint", Namespaced: true, Verbs: listWatch()},
			},
		},
	}
	cats, _, err := List(&stubDiscovery{lists: lists})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	custom, ok := catByName(cats, "Custom Resources")
	if !ok {
		t.Fatal("missing Custom Resources category")
	}
	if custom.Resources[0].Resource != "ciliumendpoints" {
		t.Fatalf("resources = %v, want alphabetical when unranked", custom.Resources)
	}
}

func TestListOrdersExtraCategoriesAlphabetically(t *testing.T) {
	lists := []*metav1.APIResourceList{
		{
			GroupVersion: "zebra.io/v1",
			APIResources: []metav1.APIResource{{Name: "zebras", Kind: "Zebra", Namespaced: true, Verbs: listWatch()}},
		},
		{
			GroupVersion: "alpha.io/v1",
			APIResources: []metav1.APIResource{{Name: "alphas", Kind: "Alpha", Namespaced: true, Verbs: listWatch()}},
		},
	}
	stub := &stubDiscovery{lists: lists}
	cats, _, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 1 {
		t.Fatalf("categories = %d, want 1 (Custom Resources)", len(cats))
	}
	custom := cats[0]
	if custom.Name != "Custom Resources" {
		t.Fatalf("category = %q, want Custom Resources", custom.Name)
	}
	if len(custom.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(custom.Resources))
	}
	if custom.Resources[0].Resource != "alphas" {
		t.Fatalf("first resource = %q, want alphas", custom.Resources[0].Resource)
	}
}

func TestListSkipsNilAndUnparseableLists(t *testing.T) {
	lists := []*metav1.APIResourceList{
		nil,
		{
			GroupVersion: "bad/group/version",
			APIResources: []metav1.APIResource{{Name: "broken", Kind: "Broken", Verbs: listWatch()}},
		},
		coreList(),
	}
	stub := &stubDiscovery{lists: lists}
	cats, byKey, err := List(stub)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, ok := byKey[Key("", "v1", "pods")]; !ok {
		t.Fatal("byKey missing pods after nil/unparseable lists")
	}
	if _, ok := catByName(cats, "Workloads"); !ok {
		t.Fatal("missing Workloads category")
	}
}

func TestGroupByCategoryOrdersExtraAlphabetically(t *testing.T) {
	descs := []api.ResourceDescriptor{
		{Resource: "deployments", Category: "Workloads"},
		{Resource: "zebras", Category: "Zebra"},
		{Resource: "alphas", Category: "Alpha"},
	}
	cats := groupByCategory(descs)
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, c.Name)
	}
	expected := []string{"Workloads", "Alpha", "Zebra"}
	if len(names) != len(expected) {
		t.Fatalf("categories = %v, want %v", names, expected)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("category order = %v, want %v", names, expected)
		}
	}
}

func TestListPropagatesError(t *testing.T) {
	stub := &stubDiscovery{lists: fullLists(), err: errors.New("partial discovery")}
	_, byKey, err := List(stub)
	if err == nil {
		t.Fatal("List returned nil error, want propagated error")
	}
	if _, ok := byKey[Key("", "v1", "pods")]; !ok {
		t.Fatal("byKey missing pods on partial error")
	}
}
