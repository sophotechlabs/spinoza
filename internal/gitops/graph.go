package gitops

import (
	"context"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	fluxSourceGroup = "source.toolkit.fluxcd.io"
	statusMissing   = "NotFound"
	readyTrue       = "True"
	readyFalse      = "False"
	readyUnknown    = "Unknown"
	categoryManaged = "managed"
)

var fluxSourceResources = map[string]bool{
	"gitrepositories":  true,
	"helmrepositories": true,
	"ocirepositories":  true,
	"buckets":          true,
}

func Build(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor) api.Graph {
	build := &builder{
		lister:   lister,
		byKind:   indexByKind(descs),
		nodes:    map[string]api.GraphNode{},
		edges:    map[string]api.GraphEdge{},
		failures: listerr.New(),
	}
	needed := []api.ResourceDescriptor{}
	for _, d := range descs {
		if graphCategory(d) == "" {
			continue
		}
		needed = append(needed, d)
	}
	lister.Warm(ctx, needed)
	for _, d := range needed {
		build.collect(ctx, d, graphCategory(d))
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

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
}

type builder struct {
	lister   Lister
	byKind   map[string]api.ResourceDescriptor
	nodes    map[string]api.GraphNode
	edges    map[string]api.GraphEdge
	failures *listerr.Collector
}

func (b *builder) collect(ctx context.Context, desc api.ResourceDescriptor, category string) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	items, err := b.lister.List(ctx, desc)
	b.failures.Record(gvr.GroupResource().String(), err)
	if err != nil {
		return
	}
	for _, item := range items {
		b.addObject(item, desc, category)
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
		Ready:     readyOf(obj, category),
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
	kind, name, namespace := sourceRefOf(obj)
	if kind == "" || name == "" {
		return
	}
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	sid := nodeID(fluxSourceGroup, kind, namespace, name)
	b.ensureRef(sid, fluxSourceGroup, kind, namespace, name, "source", "")
	b.addEdge(sid, id, "source")
}

func sourceRefOf(obj *unstructured.Unstructured) (string, string, string) {
	kind := unstr.String(obj, "spec", "chartRef", "kind")
	name := unstr.String(obj, "spec", "chartRef", "name")
	if kind != "" && name != "" {
		return kind, name, unstr.String(obj, "spec", "chartRef", "namespace")
	}
	kind = unstr.String(obj, "spec", "sourceRef", "kind")
	name = unstr.String(obj, "spec", "sourceRef", "name")
	if kind != "" && name != "" {
		return kind, name, unstr.String(obj, "spec", "sourceRef", "namespace")
	}
	kind = unstr.String(obj, "spec", "chart", "spec", "sourceRef", "kind")
	name = unstr.String(obj, "spec", "chart", "spec", "sourceRef", "name")
	return kind, name, unstr.String(obj, "spec", "chart", "spec", "sourceRef", "namespace")
}

func (b *builder) dependsOnEdges(id string, obj *unstructured.Unstructured) {
	for _, dep := range unstr.Slice(obj, "spec", "dependsOn") {
		entry, ok := dep.(map[string]any)
		if !ok {
			continue
		}
		name := unstr.At(entry, "name")
		if name == "" {
			continue
		}
		namespace := unstr.At(entry, "namespace")
		if namespace == "" {
			namespace = obj.GetNamespace()
		}
		did := nodeID(obj.GroupVersionKind().Group, obj.GetKind(), namespace, name)
		b.ensureRef(did, obj.GroupVersionKind().Group, obj.GetKind(), namespace, name, "applier", statusMissing)
		b.addEdge(did, id, "dependsOn")
	}
}

func (b *builder) inventoryEdges(id string, u *unstructured.Unstructured) {
	for _, e := range unstr.Slice(u, "status", "inventory", "entries") {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		raw := unstr.At(m, "id")
		ns, name, group, kind := parseInventoryID(raw)
		if kind == "" || name == "" {
			continue
		}
		mid := nodeID(group, kind, ns, name)
		b.ensureRef(mid, group, kind, ns, name, categoryManaged, "")
		b.addEdge(id, mid, "manages")
	}
}

func (b *builder) appEdges(id string, u *unstructured.Unstructured) {
	for _, r := range unstr.Slice(u, "status", "resources") {
		entry, ok := r.(map[string]any)
		if !ok {
			continue
		}
		kind := unstr.At(entry, "kind")
		name := unstr.At(entry, "name")
		if kind == "" || name == "" {
			continue
		}
		group := unstr.At(entry, "group")
		namespace := unstr.At(entry, "namespace")
		mid := nodeID(group, kind, namespace, name)
		b.ensureRef(mid, group, kind, namespace, name, categoryManaged, "")
		b.addEdge(id, mid, "manages")
	}
}

func (b *builder) ensureRef(id, group, kind, namespace, name, category, status string) {
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
		Status:    status,
		Ready:     readyForPlaceholder(status),
		Category:  category,
	}
}

func (b *builder) addEdge(from, to, kind string) {
	key := from + "|" + to + "|" + kind
	b.edges[key] = api.GraphEdge{From: from, To: to, Kind: kind}
}

func (b *builder) graph() api.Graph {
	nodes := make([]api.GraphNode, 0, len(b.nodes))
	kept := make(map[string]bool, len(b.nodes))
	for id, node := range b.nodes {
		if node.Category == categoryManaged {
			continue
		}
		kept[id] = true
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(left, right api.GraphNode) int {
		return strings.Compare(left.ID, right.ID)
	})
	edges := make([]api.GraphEdge, 0, len(b.edges))
	for _, e := range b.edges {
		if !kept[e.From] || !kept[e.To] {
			continue
		}
		edges = append(edges, e)
	}
	slices.SortFunc(edges, func(left, right api.GraphEdge) int {
		if left.From != right.From {
			return strings.Compare(left.From, right.From)
		}
		return strings.Compare(left.To, right.To)
	})
	return api.Graph{Nodes: nodes, Edges: edges, Error: b.failures.Message()}
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

func readyForPlaceholder(status string) string {
	if status == statusMissing {
		return readyFalse
	}
	return readyUnknown
}

func readyOf(obj *unstructured.Unstructured, category string) string {
	if category == "app" {
		return argoReady(obj)
	}
	return conditionReady(obj)
}

func argoReady(obj *unstructured.Unstructured) string {
	health := unstr.String(obj, "status", "health", "status")
	if health == "Healthy" {
		return readyTrue
	}
	if health == "Degraded" {
		return readyFalse
	}
	if health == "Missing" {
		return readyFalse
	}
	return readyUnknown
}

func conditionReady(u *unstructured.Unstructured) string {
	for _, c := range unstr.Slice(u, "status", "conditions") {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] != "Ready" {
			continue
		}
		if entry["status"] == "True" {
			return readyTrue
		}
		if entry["status"] == "False" {
			return readyFalse
		}
		return readyUnknown
	}
	return readyUnknown
}

func statusOf(obj *unstructured.Unstructured, category string) string {
	if category == "app" {
		health := unstr.String(obj, "status", "health", "status")
		sync := unstr.String(obj, "status", "sync", "status")
		return strings.TrimSpace(health + " " + sync)
	}
	return unstr.ReadySummary(obj)
}
