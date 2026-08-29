package checks

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type corpus struct {
	byResource map[target][]*unstructured.Unstructured
	names      map[target]map[string]bool
	absent     map[target]bool
}

func newCorpus(items []held, names []api.ObjectRef, absent []string) *corpus {
	out := &corpus{
		byResource: map[target][]*unstructured.Unstructured{},
		names:      map[target]map[string]bool{},
		absent:     map[target]bool{},
	}
	for _, item := range items {
		key := target{group: item.desc.Group, resource: item.desc.Resource}
		out.byResource[key] = append(out.byResource[key], item.obj)
	}
	for _, ref := range names {
		key := target{group: ref.Group, resource: ref.Resource}
		if out.names[key] == nil {
			out.names[key] = map[string]bool{}
		}
		out.names[key][ref.Namespace+"/"+ref.Name] = true
	}
	for _, name := range absent {
		for _, want := range allTargets() {
			if want.resource == name {
				out.absent[want] = true
			}
		}
	}
	return out
}

func (c *corpus) of(group, resource string) []*unstructured.Unstructured {
	return c.byResource[target{group: group, resource: resource}]
}

func (c *corpus) has(group, resource string) bool {
	return !c.absent[target{group: group, resource: resource}]
}

func (c *corpus) named(group, resource, namespace, name string) bool {
	key := target{group: group, resource: resource}
	if held, ok := c.names[key]; ok {
		return held[namespace+"/"+name]
	}
	for _, obj := range c.of(group, resource) {
		if obj.GetName() != name {
			continue
		}
		if obj.GetNamespace() != namespace {
			continue
		}
		return true
	}
	return false
}

func (c *corpus) every(resource string) []api.ObjectRef {
	out := []api.ObjectRef{}
	for key, held := range c.names {
		if key.resource != resource {
			continue
		}
		for name := range held {
			namespace, short, _ := strings.Cut(name, "/")
			out = append(out, api.ObjectRef{
				Group: key.group, Version: "v1", Resource: key.resource,
				Namespace: namespace, Name: short,
			})
		}
	}
	for key, held := range c.byResource {
		if key.resource != resource {
			continue
		}
		for _, obj := range held {
			out = append(out, api.ObjectRef{
				Group: key.group, Version: "v1", Resource: key.resource,
				Namespace: obj.GetNamespace(), Name: obj.GetName(),
			})
		}
	}
	slices.SortFunc(out, func(left, right api.ObjectRef) int {
		return strings.Compare(left.Namespace+"/"+left.Name, right.Namespace+"/"+right.Name)
	})
	return out
}

func (c *corpus) inNamespace(group, resource, namespace string) []*unstructured.Unstructured {
	out := []*unstructured.Unstructured{}
	for _, obj := range c.of(group, resource) {
		if obj.GetNamespace() != namespace {
			continue
		}
		out = append(out, obj)
	}
	return out
}

func (c *corpus) namespace(name string) *unstructured.Unstructured {
	for _, obj := range c.of("", "namespaces") {
		if obj.GetName() == name {
			return obj
		}
	}
	return nil
}

func missingResources(needs []target, held *corpus) []string {
	out := []string{}
	for _, want := range needs {
		if held.has(want.group, want.resource) {
			continue
		}
		out = append(out, want.resource)
	}
	return out
}

func skippedBecause(missing []string) string {
	return "not audited: this cluster did not report " + strings.Join(missing, " or ")
}
