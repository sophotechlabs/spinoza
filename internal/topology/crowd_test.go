package topology

import (
	"context"
	"strconv"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const crowd = 210

func numbered(namespace string, index int, ready int64) *unstructured.Unstructured {
	name := namespace + "-" + strconv.Itoa(index)
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   meta(name, namespace, name),
		"spec":       map[string]any{"replicas": int64(1)},
		"status":     map[string]any{"readyReplicas": ready},
	}}
}

func crowdedCluster(namespaces []string, withOrphan bool) []runtime.Object {
	objects := []runtime.Object{}
	for _, namespace := range namespaces {
		for index := range crowd {
			ready := int64(1)
			if namespace == "b" && index == 0 {
				ready = 0
			}
			objects = append(objects, numbered(namespace, index, ready))
		}
	}
	if withOrphan {
		objects = append(objects, pod("cluster-owned", "pod-orphan", "Running", "True", owner{
			kind:       "Rollout",
			name:       "fleet",
			uid:        "rollout-fleet",
			apiVersion: "argoproj.io/v1alpha1",
		}, ""))
	}
	return objects
}

func crowdedDescs() map[string]api.ResourceDescriptor {
	out := descs()
	fleet := out["argoproj.io/v1alpha1/rollouts"]
	fleet.Namespaced = false
	out["argoproj.io/v1alpha1/rollouts"] = fleet
	return out
}

func buildCrowd(t *testing.T, namespaces []string, withOrphan bool, req Request) api.Graph {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		listKinds(),
		crowdedCluster(namespaces, withOrphan)...,
	)
	return Build(context.Background(), listerFor(dyn), crowdedDescs(), req)
}

func TestTooManyNodesFoldIntoNamespaces(t *testing.T) {
	graph := buildCrowd(t, []string{"a", "b"}, true, Request{})
	nodes := nodesByID(graph)

	quiet, ok := nodes["namespace/a"]
	if !ok {
		t.Fatalf("the quiet namespace did not fold into one node: %d nodes", len(graph.Nodes))
	}
	if quiet.Contains != crowd {
		t.Fatalf("the namespace node folded %d objects, want %d", quiet.Contains, crowd)
	}
	if quiet.Category != categoryNamespace {
		t.Fatalf("the namespace node is a %q", quiet.Category)
	}
	if quiet.Resource != "namespaces" {
		t.Fatalf("the namespace node resolved to resource %q", quiet.Resource)
	}
	if _, shown := nodes["a-0"]; shown {
		t.Fatal("a Deployment inside the quiet namespace is still drawn")
	}
}

func TestTheNamespaceWithSomethingBrokenStaysOpen(t *testing.T) {
	nodes := nodesByID(buildCrowd(t, []string{"a", "b"}, true, Request{}))

	if _, shown := nodes["b-0"]; !shown {
		t.Fatal("the failing Deployment folded away")
	}
	busy, ok := nodes["namespace/b"]
	if !ok {
		t.Fatal("the namespace node vanished")
	}
	if busy.Contains != crowd-1 {
		t.Fatalf("the namespace node folded %d objects, want %d", busy.Contains, crowd-1)
	}
}

func TestWhatIsInNoNamespaceGetsItsOwnDistrict(t *testing.T) {
	nodes := nodesByID(buildCrowd(t, []string{"a", "b"}, true, Request{}))

	district, ok := nodes["namespace/"]
	if !ok {
		t.Fatal("what is in no namespace has nowhere to fold into")
	}
	if district.Name != "cluster-scoped" {
		t.Fatalf("the district for what has no namespace is called %q", district.Name)
	}
	if district.Resource != "" {
		t.Fatalf("the district for what has no namespace claims resource %q", district.Resource)
	}
}

func TestOneCrowdedNamespaceHasNothingToFoldInto(t *testing.T) {
	graph := buildCrowd(t, []string{"a"}, false, Request{Namespace: "a"})

	if len(graph.Nodes) != crowd {
		t.Fatalf("drew %d nodes, want %d", len(graph.Nodes), crowd)
	}
}

func knotted(name, uid, ownerName, ownerUID string) *unstructured.Unstructured {
	holder := meta(name, "prod", uid)
	holder["ownerReferences"] = []any{map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"name":       ownerName,
		"uid":        ownerUID,
		"controller": true,
	}}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata":   holder,
		"spec":       map[string]any{"replicas": int64(1)},
		"status":     map[string]any{"readyReplicas": int64(1)},
	}}
}

func TestTwoObjectsOwningEachOtherDoNotHangTheFold(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		listKinds(),
		knotted("left", "rs-left", "right", "rs-right"),
		knotted("right", "rs-right", "left", "rs-left"),
	)

	graph := Build(context.Background(), listerFor(dyn), descs(), Request{})

	if len(graph.Nodes) != 0 {
		t.Fatalf("drew %d nodes, want the cycle dropped rather than walked forever", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("drew %d edges into nodes that are not there", len(graph.Edges))
	}
}
