package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/compare"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const comparedQuery = "?group=apps&version=v1&resource=deployments&namespace=prod&name=web"

func comparedDeployment(replicas int64, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":            "web",
			"namespace":       "prod",
			"uid":             uid,
			"resourceVersion": "1",
		},
		"spec":   map[string]any{"replicas": replicas},
		"status": map[string]any{"readyReplicas": replicas},
	}}
}

// the reader that talks to another cluster hands back yaml, so the stub does too.
func rawOf(t *testing.T, item *unstructured.Unstructured) string {
	t.Helper()
	raw, err := compare.YAML(item)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return raw
}

func compareServer(t *testing.T, elsewhere map[string]string, objs ...runtime.Object) (*httptest.Server, *stubCluster) {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	ctx := t.Context()
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): {
			Group:      "apps",
			Version:    "v1",
			Resource:   "deployments",
			Kind:       "Deployment",
			Namespaced: true,
		},
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	cluster := &stubCluster{mgr: mgr, current: api.ContextRef{Name: "staging"}, elsewhere: elsewhere}
	return clusterServer(t, cluster), cluster
}

func comparisonFrom(t *testing.T, body []byte) api.Comparison {
	t.Helper()
	var result api.Comparison
	err := json.Unmarshal(body, &result)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	return result
}

// what the two sides come back as

func TestBothSidesComeBackNormalised(t *testing.T) {
	far := map[string]string{"prod/prod/web": rawOf(t, comparedDeployment(3, "far-uid"))}
	ts, _ := compareServer(t, far, comparedDeployment(3, "near-uid"))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	result := comparisonFrom(t, body)
	if !result.Identical {
		t.Fatalf("the same manifest read as different:\nleft:\n%s\nright:\n%s", result.Left, result.Right)
	}
	if strings.Contains(result.Left, "uid:") || strings.Contains(result.Right, "status:") {
		t.Fatalf("per-cluster fields survived:\n%s\n%s", result.Left, result.Right)
	}
}

func TestARealDifferenceIsReported(t *testing.T) {
	far := map[string]string{"prod/prod/web": rawOf(t, comparedDeployment(5, "far-uid"))}
	ts, _ := compareServer(t, far, comparedDeployment(3, "near-uid"))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	result := comparisonFrom(t, body)
	if result.Identical {
		t.Fatal("a different replica count was reported as identical")
	}
	if !strings.Contains(result.Left, "replicas: 3") || !strings.Contains(result.Right, "replicas: 5") {
		t.Fatalf("sides = %q / %q", result.Left, result.Right)
	}
}

func TestEachSideSaysWhichContextItCameFrom(t *testing.T) {
	far := map[string]string{"prod/prod/web": rawOf(t, comparedDeployment(3, "far-uid"))}
	ts, _ := compareServer(t, far, comparedDeployment(3, "near-uid"))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	result := comparisonFrom(t, body)
	if result.LeftContext != "staging" || result.RightContext != "prod" {
		t.Fatalf("contexts = %q / %q", result.LeftContext, result.RightContext)
	}
}

func TestTheFarSideCanCarryItsOwnNamespaceAndName(t *testing.T) {
	far := map[string]string{"prod/other/api": rawOf(t, comparedDeployment(3, "far-uid"))}
	ts, _ := compareServer(t, far, comparedDeployment(3, "near-uid"))

	_, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/compare"+comparedQuery+"&against=prod&againstNamespace=other&againstName=api", nil)

	result := comparisonFrom(t, body)
	if result.Missing != "" {
		t.Fatalf("missing = %q, want the override to have found it", result.Missing)
	}
}

func TestRawKeepsWhatNormalisationWouldStrip(t *testing.T) {
	far := map[string]string{"prod/prod/web": rawOf(t, comparedDeployment(3, "far-uid"))}
	ts, _ := compareServer(t, far, comparedDeployment(3, "near-uid"))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod&raw=true", nil)

	result := comparisonFrom(t, body)
	if !strings.Contains(result.Right, "uid: far-uid") {
		t.Fatalf("right = %q, want the raw object", result.Right)
	}
	if result.Identical {
		t.Fatal("two objects with different uids were called identical in raw mode")
	}
}

func TestAClusterScopedObjectComparesWithoutANamespace(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "p-mk1", "uid": "near"},
		"spec":       map[string]any{"podCIDR": "10.42.0.0/24"},
	}}
	far := node.DeepCopy()
	far.SetUID("far")
	kinds := map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "nodes"}: "NodeList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, node)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "nodes"): {Version: "v1", Resource: "nodes", Kind: "Node"},
	}
	mgr := resources.NewManager(t.Context(), resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	cluster := &stubCluster{
		mgr:       mgr,
		current:   api.ContextRef{Name: "staging"},
		elsewhere: map[string]string{"prod//p-mk1": rawOf(t, far)},
	}
	ts := clusterServer(t, cluster)

	_, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/compare?version=v1&resource=nodes&name=p-mk1&against=prod", nil)

	result := comparisonFrom(t, body)
	if result.Missing != "" {
		t.Fatalf("missing = %q, want the cluster-scoped read to have found it", result.Missing)
	}
	if !result.Identical {
		t.Fatalf("a node differing only by uid was not identical:\n%s\n%s", result.Left, result.Right)
	}
}

// what it says when the far side cannot answer

func TestAnObjectMissingThereIsSaidPlainly(t *testing.T) {
	ts, _ := compareServer(t, map[string]string{}, comparedDeployment(3, "near-uid"))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	result := comparisonFrom(t, body)
	if result.Missing != "that context has no such object" {
		t.Fatalf("missing = %q", result.Missing)
	}
	if result.Right != "" || result.Identical {
		t.Fatalf("result = %+v, want an empty right side", result)
	}
}

func TestAFarSideThatFailsSaysWhy(t *testing.T) {
	ts, cluster := compareServer(t, map[string]string{}, comparedDeployment(3, "near-uid"))
	cluster.readErr = errors.New("the apiserver is unreachable")

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	result := comparisonFrom(t, body)
	if !strings.Contains(result.Missing, "unreachable") {
		t.Fatalf("missing = %q, want the reason", result.Missing)
	}
}

func TestTheContextToCompareAgainstIsRequired(t *testing.T) {
	ts, _ := compareServer(t, map[string]string{}, comparedDeployment(3, "near-uid"))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestAnObjectMissingHereIsAnError(t *testing.T) {
	far := map[string]string{"prod/prod/web": rawOf(t, comparedDeployment(3, "far-uid"))}
	ts, _ := compareServer(t, far)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/compare"+comparedQuery+"&against=prod", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
