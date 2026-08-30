package checks

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type corpus struct {
	byResource map[target][]*unstructured.Unstructured
	names      map[target]map[string]Named
	absent     map[target]bool
	unread     []target
	mentioned  map[string]int
}

func newCorpus(
	items []held, names []Named, absent []string, asked, unread []target, mentions map[string]int,
) *corpus {
	out := &corpus{
		byResource: map[target][]*unstructured.Unstructured{},
		names:      map[target]map[string]Named{},
		absent:     map[target]bool{},
	}
	for _, item := range items {
		key := target{group: item.desc.Group, resource: item.desc.Resource}
		out.byResource[key] = append(out.byResource[key], item.obj)
	}
	for _, found := range names {
		key := target{group: found.Ref.Group, resource: found.Ref.Resource}
		if out.names[key] == nil {
			out.names[key] = map[string]Named{}
		}
		out.names[key][found.Ref.Namespace+"/"+found.Ref.Name] = found
	}
	out.mentioned = mentionedStrings(items)
	for name, seen := range mentions {
		out.mentioned[name] += seen
	}
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
	out.unread = unread
	return out
}

// refused names a kind the cluster would not let the audit read. Any check that
// answers "nothing anywhere names this" has to stand down when one exists: the
// reference it is looking for may be in exactly the kind that was refused.
func (c *corpus) refused() string {
	if len(c.unread) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.unread))
	for _, want := range c.unread {
		names = append(names, want.resource)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func mentionedStrings(items []held) map[string]int {
	out := map[string]int{}
	here := map[string]bool{}
	for _, item := range items {
		clear(here)
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
		_, found := held[namespace+"/"+name]
		return found
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

func (c *corpus) every(resource string) []Named {
	out := []Named{}
	for key, held := range c.names {
		if key.resource != resource {
			continue
		}
		for _, found := range held {
			out = append(out, found)
		}
	}
	for key, held := range c.byResource {
		if key.resource != resource {
			continue
		}
		for _, obj := range held {
			ref := api.ObjectRef{
				Group: key.group, Version: "v1", Resource: key.resource,
				Namespace: obj.GetNamespace(), Name: obj.GetName(),
			}
			out = append(out, namedOf(ref, obj))
		}
	}
	slices.SortFunc(out, func(left, right Named) int {
		return strings.Compare(left.Ref.Namespace+"/"+left.Ref.Name, right.Ref.Namespace+"/"+right.Ref.Name)
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
