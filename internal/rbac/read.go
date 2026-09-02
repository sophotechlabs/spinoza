package rbac

import (
	"context"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const Group = "rbac.authorization.k8s.io"

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
}

var wanted = []string{"roles", "clusterroles", "rolebindings", "clusterrolebindings"}

func Read(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor) Index {
	found, absent := kinds(descs)
	lister.Warm(ctx, found)
	held := Held{}
	trouble := []string{}
	var reading sync.Mutex
	var asking sync.WaitGroup
	for _, desc := range found {
		asking.Add(1)
		safe.Go("reading RBAC "+desc.Resource, func() {
			defer asking.Done()
			defer func() {
				caught := recover()
				if caught == nil {
					return
				}
				safe.Log("reading RBAC "+desc.Resource, caught)
				reading.Lock()
				trouble = append(trouble, desc.Resource+": spinoza could not finish reading this resource")
				reading.Unlock()
			}()
			objects, err := lister.List(ctx, desc)
			reading.Lock()
			defer reading.Unlock()
			if err != nil {
				trouble = append(trouble, desc.Resource+": "+err.Error())
				return
			}
			place(&held, desc.Resource, objects)
		})
	}
	asking.Wait()
	index := Build(held)
	index.Error = joined(trouble, absent)
	return index
}

func place(held *Held, resource string, objects []*unstructured.Unstructured) {
	switch resource {
	case "roles":
		held.Roles = objects
	case "clusterroles":
		held.ClusterRoles = objects
	case "rolebindings":
		held.Bindings = objects
	case "clusterrolebindings":
		held.ClusterBinds = objects
	default:
	}
}

func kinds(descs map[string]api.ResourceDescriptor) (found []api.ResourceDescriptor, absent []string) {
	for _, want := range wanted {
		desc, ok := matching(descs, want)
		if !ok {
			absent = append(absent, want)
			continue
		}
		found = append(found, desc)
	}
	return found, absent
}

func matching(descs map[string]api.ResourceDescriptor, resource string) (api.ResourceDescriptor, bool) {
	for _, desc := range descs {
		if desc.Group == Group && desc.Resource == resource {
			return desc, true
		}
	}
	return api.ResourceDescriptor{}, false
}

func joined(trouble, absent []string) string {
	parts := trouble
	if len(absent) > 0 {
		parts = append(parts, "not discovered: "+strings.Join(absent, ", "))
	}
	return strings.Join(parts, " · ")
}
