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
	mentioned  map[string]int
}

func newCorpus(items []held, names []api.ObjectRef, absent []string, asked, unread []target) *corpus {
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
	out.mentioned = mentionedStrings(items)
	requested := map[target]bool{}
	for _, want := range asked {
		requested[want] = true
	}
	for _, want := range allTargets() {
		if !requested[want] {
			out.absent[want] = true
		}
	}
	for _, name := range absent {
		for _, want := range allTargets() {
			if want.resource == name {
				out.absent[want] = true
			}
		}
	}
	// A listing the apiserver refused is not an empty cluster. Treating it as
	// one turned 500 references into "missing" on GKE production 2026-08-29,
	// where the account could not list Secrets.
	for _, want := range unread {
		out.absent[want] = true
	}
	return out
}

func mentionedStrings(items []held) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		here := map[string]bool{}
		gatherStrings(item.obj.Object, here)
		delete(here, item.obj.GetName())
		for name := range here {
			out[name]++
		}
	}
	return out
}

func gatherStrings(value any, into map[string]bool) {
	switch typed := value.(type) {
	case string:
		into[typed] = true
	case map[string]any:
		for _, entry := range typed {
			gatherStrings(entry, into)
		}
	case []any:
		for _, entry := range typed {
			gatherStrings(entry, into)
		}
	default:
		return
	}
}

// An object's own name is dropped before counting, so anything left is another
// object naming it. That is what lets a Flux GitRepository or a cert-manager
// Certificate count as a reference without the audit knowing either kind.
// Proved needed on p-mk1 2026-08-29, where every remaining orphan was named by
// a custom resource.
func (c *corpus) mentionedElsewhere(name string) bool {
	return c.mentioned[name] > 0
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
