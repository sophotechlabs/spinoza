package checks

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type corpus struct {
	items      []held
	byResource map[target][]*unstructured.Unstructured
	names      map[target]map[string]Named
	absent     map[target]bool
	asked      map[target]bool
	unread     []target
	mentioned  map[string]int
}

func newCorpus(
	items []held, names []Named, absent []string, asked, unread []target, mentions map[string]int,
) *corpus {
	out := &corpus{
		items:      items,
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
	out.asked = requested
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
	for _, want := range unread {
		out.absent[want] = true
	}
	out.unread = unread
	return out
}

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

func (c *corpus) mentionedElsewhere(name string) bool {
	return c.mentioned[name] > 0
}

func (c *corpus) everything() []held {
	return c.items
}

func (c *corpus) read(group, resource string) bool {
	key := target{group: group, resource: resource}
	if _, held := c.byResource[key]; held {
		return true
	}
	return c.asked[key] && !c.absent[key]
}

func (c *corpus) subjectsOfKind(kind string) []Subject {
	out := []Subject{}
	for _, item := range c.items {
		if item.obj.GetKind() != kind {
			continue
		}
		out = append(out, subjectFromHeld(item))
	}
	return out
}

func subjectFromHeld(item held) Subject {
	origin, managedBy := originOf(item.obj)
	return Subject{
		Ref: api.ObjectRef{
			Group:     item.desc.Group,
			Version:   item.desc.Version,
			Resource:  item.desc.Resource,
			Namespace: item.obj.GetNamespace(),
			Name:      item.obj.GetName(),
		},
		Kind:      item.obj.GetKind(),
		Object:    item.obj,
		Pod:       map[string]any{},
		Replicas:  1,
		Origin:    origin,
		ManagedBy: managedBy,
	}
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
