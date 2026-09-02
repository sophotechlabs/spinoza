package rbac

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type cluster struct {
	mu       sync.Mutex
	held     map[string][]*unstructured.Unstructured
	fail     map[string]error
	panicFor map[string]bool
	warmed   []api.ResourceDescriptor
}

func (c *cluster) List(_ context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.panicFor[desc.Resource] {
		panic("list panic")
	}
	if err, wrong := c.fail[desc.Resource]; wrong {
		return nil, err
	}
	return c.held[desc.Resource], nil
}

func (c *cluster) Warm(_ context.Context, descs []api.ResourceDescriptor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warmed = append(c.warmed, descs...)
}

func descriptors(resources ...string) map[string]api.ResourceDescriptor {
	out := map[string]api.ResourceDescriptor{}
	for _, one := range resources {
		out[Group+"/v1/"+one] = api.ResourceDescriptor{
			Group: Group, Version: "v1", Resource: one, Kind: strings.ToUpper(one[:1]) + one[1:],
		}
	}
	return out
}

func everyKind() map[string]api.ResourceDescriptor {
	return descriptors("roles", "clusterroles", "rolebindings", "clusterrolebindings")
}

func TestReadingACLusterBuildsTheIndex(t *testing.T) {
	held := &cluster{held: map[string][]*unstructured.Unstructured{
		"clusterroles":        {clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"secrets"}))},
		"clusterrolebindings": {binding("read", "", "ClusterRole", "reader", user("ana"))},
	}}

	index := Read(t.Context(), held, everyKind())

	if len(index.Holders) != 1 || index.Holders[0].Subject.Name != "ana" {
		t.Fatalf("holders = %+v", labels(index))
	}
	if index.Error != "" {
		t.Fatalf("error = %q", index.Error)
	}
}

func TestEveryKindItNeedsIsWarmedFirst(t *testing.T) {
	held := &cluster{}

	Read(t.Context(), held, everyKind())

	if len(held.warmed) != 4 {
		t.Fatalf("warmed = %+v", held.warmed)
	}
}

func TestAKindTheClusterDoesNotHaveIsNamedRatherThanFatal(t *testing.T) {
	held := &cluster{held: map[string][]*unstructured.Unstructured{
		"clusterroles":        {clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"secrets"}))},
		"clusterrolebindings": {binding("read", "", "ClusterRole", "reader", user("ana"))},
	}}

	index := Read(t.Context(), held, descriptors("clusterroles", "clusterrolebindings"))

	if !strings.Contains(index.Error, "not discovered: roles, rolebindings") {
		t.Fatalf("error = %q", index.Error)
	}
	if len(index.Holders) != 1 {
		t.Fatalf("it gave up on the kinds it did have: %+v", labels(index))
	}
}

func TestAKindThatWouldNotListIsNamed(t *testing.T) {
	held := &cluster{
		held: map[string][]*unstructured.Unstructured{
			"clusterrolebindings": {binding("read", "", "ClusterRole", "reader", user("ana"))},
		},
		fail: map[string]error{"clusterroles": errors.New("forbidden")},
	}

	index := Read(t.Context(), held, everyKind())

	if !strings.Contains(index.Error, "clusterroles: forbidden") {
		t.Fatalf("error = %q", index.Error)
	}
}

func TestAKindThatPanicsWhileListingIsNamed(t *testing.T) {
	lister := &cluster{panicFor: map[string]bool{"roles": true}}

	index := Read(t.Context(), lister, everyKind())

	if !strings.Contains(index.Error, "roles") {
		t.Fatalf("error = %q, want the panicking kind", index.Error)
	}
}

func TestAClusterWithNoRBACAtAllComesBackEmpty(t *testing.T) {
	index := Read(t.Context(), &cluster{}, everyKind())

	if len(index.Holders) != 0 {
		t.Fatalf("holders = %+v", labels(index))
	}
	if index.Error != "" {
		t.Fatalf("error = %q", index.Error)
	}
}

func TestAKindFromAnotherGroupIsNotMistakenForRBAC(t *testing.T) {
	descs := everyKind()
	descs["/v1/roles"] = api.ResourceDescriptor{Group: "", Version: "v1", Resource: "roles"}
	held := &cluster{}

	Read(t.Context(), held, descs)

	for _, one := range held.warmed {
		if one.Group != Group {
			t.Fatalf("it warmed %+v", one)
		}
	}
}

func TestNamespacedAndClusterKindsLandInTheirOwnPlaces(t *testing.T) {
	held := &cluster{held: map[string][]*unstructured.Unstructured{
		"roles":               {role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		"rolebindings":        {binding("read", "web", "Role", "reader", user("ana"))},
		"clusterroles":        {clusterRole("admin", rule([]string{"*"}, []string{"*"}, []string{"*"}))},
		"clusterrolebindings": {binding("admins", "", "ClusterRole", "admin", user("bo"))},
	}}

	index := Read(t.Context(), held, everyKind())

	if len(index.Holders) != 2 {
		t.Fatalf("holders = %+v", labels(index))
	}
	if index.Holders[0].Subject.Name != "bo" {
		t.Fatalf("the cluster-admin was not first: %+v", labels(index))
	}
}
