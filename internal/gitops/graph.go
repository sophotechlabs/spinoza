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
	b := &builder{
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
		b.collect(d, category)
	}
	return b.graph()
}

func indexByKind(descs map[string]api.ResourceDescriptor) map[string]api.ResourceDescriptor {
	byKind := map[string]api.ResourceDescriptor{}
	for _, d := range descs {
		byKind[d.Group+"/"+d.Kind] = d
	}
	return byKind
}

func graphCategory(d api.ResourceDescriptor) string {
	if d.Group == fluxSourceGroup && fluxSourceResources[d.Resource] {
		return "source"
	}
	if d.Group == "kustomize.toolkit.fluxcd.io" && d.Resource == "kustomizations" {
		return "applier"
	}
	if d.Group == "helm.toolkit.fluxcd.io" && d.Resource == "helmreleases" {
		return "applier"
	}
	if d.Group == "argoproj.io" && d.Resource == "applications" {
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

func (b *builder) collect(d api.ResourceDescriptor, category string) {
	gvr := schema.GroupVersionResource{Group: d.Group, Version: d.Version, Resource: d.Resource}
	list, err := b.dyn.Resource(gvr).List(b.ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for i := range list.Items {
		u := &list.Items[i]
		b.addObject(u, d, category)
	}
}

func (b *builder) addObject(u *unstructured.Unstructured, d api.ResourceDescriptor, category string) {
	id := nodeID(d.Group, d.Kind, u.GetNamespace(), u.GetName())
	b.nodes[id] = api.GraphNode{
		ID:        id,
		Kind:      d.Kind,
		Group:     d.Group,
		Version:   d.Version,
		Resource:  d.Resource,
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
		Status:    statusOf(u, category),
		Category:  category,
	}
	if category == "applier" {
		b.applierEdges(id, u)
	}
	if category == "app" {
		b.appEdges(id, u)
	}
}

func (b *builder) applierEdges(id string, u *unstructured.Unstructured) {
	b.sourceEdge(id, u)
	b.dependsOnEdges(id, u)
	b.inventoryEdges(id, u)
}

func (b *builder) sourceEdge(id string, u *unstructured.Unstructured) {
	kind := nestedString(u, "spec", "sourceRef", "kind")
	name := nestedString(u, "spec", "sourceRef", "name")
	if kind == "" || name == "" {
		kind = nestedString(u, "spec", "chart", "spec", "sourceRef", "kind")
		name = nestedString(u, "spec", "chart", "spec", "sourceRef", "name")
	}
	if kind == "" || name == "" {
		return
	}
	namespace := nestedString(u, "spec", "sourceRef", "namespace")
	if namespace == "" {
		namespace = u.GetNamespace()
	}
	sid := nodeID(fluxSourceGroup, kind, namespace, name)
	b.ensureRef(sid, fluxSourceGroup, kind, namespace, name, "source")
	b.addEdge(sid, id, "source")
}

func (b *builder) dependsOnEdges(id string, u *unstructured.Unstructured) {
	for _, dep := range nestedSlice(u, "spec", "dependsOn") {
		m, ok := dep.(map[string]any)
		if !ok {
			continue
		}
		name := stringAt(m, "name")
		if name == "" {
			continue
		}
		namespace := stringAt(m, "namespace")
		if namespace == "" {
			namespace = u.GetNamespace()
		}
		did := nodeID(u.GroupVersionKind().Group, u.GetKind(), namespace, name)
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
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		kind := stringAt(m, "kind")
		name := stringAt(m, "name")
		if kind == "" || name == "" {
			continue
		}
		group := stringAt(m, "group")
		namespace := stringAt(m, "namespace")
		mid := nodeID(group, kind, namespace, name)
		b.ensureRef(mid, group, kind, namespace, name, "managed")
		b.addEdge(id, mid, "manages")
	}
}

func (b *builder) ensureRef(id, group, kind, namespace, name, category string) {
	if _, ok := b.nodes[id]; ok {
		return
	}
	d := b.byKind[group+"/"+kind]
	b.nodes[id] = api.GraphNode{
		ID:        id,
		Kind:      kind,
		Group:     group,
		Version:   d.Version,
		Resource:  d.Resource,
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
	slices.SortFunc(nodes, func(a, b api.GraphNode) int {
		return strings.Compare(a.ID, b.ID)
	})
	edges := make([]api.GraphEdge, 0, len(b.edges))
	for _, e := range b.edges {
		edges = append(edges, e)
	}
	slices.SortFunc(edges, func(a, b api.GraphEdge) int {
		if a.From != b.From {
			return strings.Compare(a.From, b.From)
		}
		return strings.Compare(a.To, b.To)
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

func statusOf(u *unstructured.Unstructured, category string) string {
	if category == "app" {
		health := nestedString(u, "status", "health", "status")
		sync := nestedString(u, "status", "sync", "status")
		return strings.TrimSpace(health + " " + sync)
	}
	return conditionSummary(u)
}

func conditionSummary(u *unstructured.Unstructured) string {
	for _, c := range nestedSlice(u, "status", "conditions") {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] != "Ready" {
			continue
		}
		if m["status"] == "True" {
			return "Ready"
		}
		reason, ok := m["reason"].(string)
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
