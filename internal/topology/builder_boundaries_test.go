package topology

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
)

func newEmptyBuilder() *builder {
	return &builder{
		byKind:     indexByKind(descs()),
		kindFor:    indexKinds(descs()),
		objects:    map[string]*object{},
		identity:   map[string]string{},
		podsByNs:   map[string][]string{},
		podsByPair: map[string]map[string][]string{},
		controller: map[string]string{},
		edges:      map[string]api.GraphEdge{},
		failures:   listerr.New(),
		namespaces: map[string]bool{},
	}
}

func TestAMissingClusterScopedReferenceKeepsNoNamespace(t *testing.T) {
	build := newEmptyBuilder()
	id := build.ensure("example.com", "Fleet", "prod", "west", categoryWorkload, statusMissing)
	node := build.objects[id].node

	if id != "example.com/Fleet//west" {
		t.Fatalf("id = %q, want a cluster-scoped identity", id)
	}
	if node.Namespace != "" {
		t.Fatalf("namespace = %q, want none", node.Namespace)
	}
	if build.identity[identityKey("example.com", "Fleet", "", "west")] != id {
		t.Fatal("the cluster-scoped identity was not indexed without a namespace")
	}
}

func TestAPlaceholderIsNotTraversedAsAListedObject(t *testing.T) {
	build := newEmptyBuilder()
	build.ensure("", kindService, "prod", "missing", categoryService, statusMissing)

	if listed := build.listed(); len(listed) != 0 {
		t.Fatalf("listed %d placeholder objects, want none", len(listed))
	}
}

func TestParallelEdgesAreSortedByKind(t *testing.T) {
	build := newEmptyBuilder()
	build.objects["from"] = &object{node: api.GraphNode{ID: "from"}}
	build.objects["to"] = &object{node: api.GraphNode{ID: "to"}}
	build.edges["routes"] = api.GraphEdge{From: "from", To: "to", Kind: edgeRoutes}
	build.edges["owns"] = api.GraphEdge{From: "from", To: "to", Kind: edgeOwns}

	edges := build.graph(Request{}).Edges
	if len(edges) != 2 {
		t.Fatalf("edges = %v, want two", edges)
	}
	if edges[0].Kind != edgeOwns || edges[1].Kind != edgeRoutes {
		t.Fatalf("edge kinds = %q, %q, want owns then routes", edges[0].Kind, edges[1].Kind)
	}
}

func TestAStaleControllerReferenceDoesNotFoldOrTemplateAPod(t *testing.T) {
	build := newEmptyBuilder()
	pod := &object{
		node: api.GraphNode{ID: "pod", Kind: kindPod},
		raw:  &unstructured.Unstructured{},
	}
	build.objects[pod.node.ID] = pod
	build.controller[pod.node.ID] = "missing-owner"

	if build.templatedByItsController(pod) {
		t.Fatal("a missing controller was treated as a workload template")
	}
	if parents := build.foldParents([]string{pod.node.ID}); len(parents) != 0 {
		t.Fatalf("a pod folded into a missing controller: %v", parents)
	}
}
