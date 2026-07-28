package gitops

import (
	"context"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const fluxSourceGroup = "source.toolkit.fluxcd.io"

var fluxSourceResources = map[string]bool{
	"gitrepositories":  true,
	"helmrepositories": true,
	"ocirepositories":  true,
	"buckets":          true,
}

func Build(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor) api.Graph {
	build := &builder{
		ctx:    ctx,
		dyn:    dyn,
		byKind: indexByKind(descs),
		nodes:  map[string]api.GraphNode{},
		edges:  map[string]api.GraphEdge{},
	}
	for _, d := range descs {
		category := graphCategory(d)
		if category == "" {
			continue
		}
		build.collect(d, category)
	}
	return build.graph()
}

func indexByKind(descs map[string]api.ResourceDescriptor) map[string]api.ResourceDescriptor {
	byKind := map[string]api.ResourceDescriptor{}
	for _, d := range descs {
		byKind[d.Group+"/"+d.Kind] = d
	}
	return byKind
}

func graphCategory(desc api.ResourceDescriptor) string {
	if desc.Group == fluxSourceGroup && fluxSourceResources[desc.Resource] {
		return "source"
	}
	if desc.Group == "kustomize.toolkit.fluxcd.io" && desc.Resource == "kustomizations" {
		return "applier"
	}
	if desc.Group == "helm.toolkit.fluxcd.io" && desc.Resource == "helmreleases" {
		return "applier"
	}
	if desc.Group == "argoproj.io" && desc.Resource == "applications" {
		return "app"
	}
	return ""
}

type builder struct {
	ctx    context.Context
	dyn    dynamic.Interface
	byKind map[string]api.ResourceDescriptor
	nodes  map[string]api.GraphNode
	edges  map[string]api.GraphEdge
}

func (b *builder) collect(desc api.ResourceDescriptor, category string) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := b.dyn.Resource(gvr).List(b.ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for i := range list.Items {
		u := &list.Items[i]
		b.addObject(u, desc, category)
	}
}

func (b *builder) addObject(obj *unstructured.Unstructured, desc api.ResourceDescriptor, category string) {
	id := nodeID(desc.Group, desc.Kind, obj.GetNamespace(), obj.GetName())
	b.nodes[id] = api.GraphNode{
		ID:        id,
		Kind:      desc.Kind,
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Status:    statusOf(obj, category),
		Category:  category,
	}
	if category == "applier" {
		b.applierEdges(id, obj)
	}
	if category == "app" {
		b.appEdges(id, obj)
	}
}

func (b *builder) applierEdges(id string, u *unstructured.Unstructured) {
	b.sourceEdge(id, u)
	b.dependsOnEdges(id, u)
	b.inventoryEdges(id, u)
}

func (b *builder) sourceEdge(id string, obj *unstructured.Unstructured) {
	kind := nestedString(obj, "spec", "sourceRef", "kind")
	name := nestedString(obj, "spec", "sourceRef", "name")
	if kind == "" || name == "" {
		kind = nestedString(obj, "spec", "chart", "spec", "sourceRef", "kind")
		name = nestedString(obj, "spec", "chart", "spec", "sourceRef", "name")
	}
	if kind == "" || name == "" {
		return
	}
	namespace := nestedString(obj, "spec", "sourceRef", "namespace")
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	sid := nodeID(fluxSourceGroup, kind, namespace, name)
	b.ensureRef(sid, fluxSourceGroup, kind, namespace, name, "source")
	b.addEdge(sid, id, "source")
}

func (b *builder) dependsOnEdges(id string, obj *unstructured.Unstructured) {
	for _, dep := range nestedSlice(obj, "spec", "dependsOn") {
		entry, ok := dep.(map[string]any)
		if !ok {
			continue
		}
		name := stringAt(entry, "name")
		if name == "" {
			continue
		}
		namespace := stringAt(entry, "namespace")
		if namespace == "" {
			namespace = obj.GetNamespace()
		}
		did := nodeID(obj.GroupVersionKind().Group, obj.GetKind(), namespace, name)
		b.addEdge(did, id, "dependsOn")
	}
}

func (b *builder) inventoryEdges(id string, u *unstructured.Unstructured) {
	for _, e := range nestedSlice(u, "status", "inventory", "entries") {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		raw := stringAt(m, "id")
		ns, name, group, kind := parseInventoryID(raw)
		if kind == "" || name == "" {
			continue
		}
		mid := nodeID(group, kind, ns, name)
		b.ensureRef(mid, group, kind, ns, name, "managed")
		b.addEdge(id, mid, "manages")
	}
}

func (b *builder) appEdges(id string, u *unstructured.Unstructured) {
	for _, r := range nestedSlice(u, "status", "resources") {
		entry, ok := r.(map[string]any)
		if !ok {
			continue
		}
		kind := stringAt(entry, "kind")
		name := stringAt(entry, "name")
		if kind == "" || name == "" {
			continue
		}
		group := stringAt(entry, "group")
		namespace := stringAt(entry, "namespace")
		mid := nodeID(group, kind, namespace, name)
		b.ensureRef(mid, group, kind, namespace, name, "managed")
		b.addEdge(id, mid, "manages")
	}
}

func (b *builder) ensureRef(id, group, kind, namespace, name, category string) {
	if _, ok := b.nodes[id]; ok {
		return
	}
	desc := b.byKind[group+"/"+kind]
	b.nodes[id] = api.GraphNode{
		ID:        id,
		Kind:      kind,
		Group:     group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Name:      name,
		Namespace: namespace,
		Status:    "",
		Category:  category,
	}
}

func (b *builder) addEdge(from, to, kind string) {
	key := from + "|" + to + "|" + kind
	b.edges[key] = api.GraphEdge{From: from, To: to, Kind: kind}
}

func (b *builder) graph() api.Graph {
	nodes := make([]api.GraphNode, 0, len(b.nodes))
	for _, n := range b.nodes {
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(left, right api.GraphNode) int {
		return strings.Compare(left.ID, right.ID)
	})
	edges := make([]api.GraphEdge, 0, len(b.edges))
	for _, e := range b.edges {
		edges = append(edges, e)
	}
	slices.SortFunc(edges, func(left, right api.GraphEdge) int {
		if left.From != right.From {
			return strings.Compare(left.From, right.From)
		}
		return strings.Compare(left.To, right.To)
	})
	return api.Graph{Nodes: nodes, Edges: edges}
}

func nodeID(group, kind, namespace, name string) string {
	return group + "/" + kind + "/" + namespace + "/" + name
}

func parseInventoryID(raw string) (namespace, name, group, kind string) {
	parts := strings.Split(raw, "_")
	if len(parts) != 4 {
		return "", "", "", ""
	}
	return parts[0], parts[1], parts[2], parts[3]
}

func statusOf(obj *unstructured.Unstructured, category string) string {
	if category == "app" {
		health := nestedString(obj, "status", "health", "status")
		sync := nestedString(obj, "status", "sync", "status")
		return strings.TrimSpace(health + " " + sync)
	}
	return conditionSummary(obj)
}

func conditionSummary(u *unstructured.Unstructured) string {
	for _, c := range nestedSlice(u, "status", "conditions") {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] != "Ready" {
			continue
		}
		if entry["status"] == "True" {
			return "Ready"
		}
		reason, ok := entry["reason"].(string)
		if ok && reason != "" {
			return reason
		}
		return "NotReady"
	}
	return ""
}

func nestedString(u *unstructured.Unstructured, fields ...string) string {
	v, found, err := unstructured.NestedString(u.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return v
}

func nestedSlice(u *unstructured.Unstructured, fields ...string) []any {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}

func stringAt(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}
