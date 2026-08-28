package topology

import (
	"maps"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	nodeBudget    = 400
	rootDepth     = 2
	maxFoldDepth  = 16
	districtIDTag = "namespace/"
)

var foldableKinds = map[string]bool{
	kindPod:        true,
	kindReplicaSet: true,
	kindJob:        true,
}

func (b *builder) graph(req Request) api.Graph {
	ids := b.ids()
	parents := b.foldParents(ids)
	expanded := setOf(req.Expanded)
	visible := visibleSet(ids, parents, expanded)
	if b.crowded(visible) {
		ids, parents, visible = b.districts(ids, parents, expanded)
	}
	nodes := b.nodesFor(ids, parents, visible)
	edges := b.edgesFor(parents, visible)
	if req.Root.Name != "" {
		nodes, edges = b.neighborhood(req.Root, parents, visible, nodes, edges)
	}
	slices.SortFunc(nodes, func(left, right api.GraphNode) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(edges, func(left, right api.GraphEdge) int {
		if left.From != right.From {
			return strings.Compare(left.From, right.From)
		}
		if left.To != right.To {
			return strings.Compare(left.To, right.To)
		}
		return strings.Compare(left.Kind, right.Kind)
	})
	return api.Graph{Nodes: nodes, Edges: edges, Error: b.failures.Message()}
}

func (b *builder) ids() []string {
	out := make([]string, 0, len(b.objects))
	for id := range b.objects {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out[value] = true
	}
	return out
}

func (b *builder) foldParents(ids []string) map[string]string {
	parents := map[string]string{}
	for _, id := range ids {
		if !foldableKinds[b.objects[id].node.Kind] {
			continue
		}
		owner, managed := b.controller[id]
		if !managed {
			continue
		}
		if _, known := b.objects[owner]; !known {
			continue
		}
		parents[id] = owner
	}
	return parents
}

func visibleSet(ids []string, parents map[string]string, expanded map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		if shown(id, parents, expanded) {
			out[id] = true
		}
	}
	return out
}

func shown(id string, parents map[string]string, expanded map[string]bool) bool {
	for range maxFoldDepth {
		parent, folded := parents[id]
		if !folded {
			return true
		}
		if !expanded[parent] {
			return false
		}
		id = parent
	}
	return false
}

func anchorOf(id string, parents map[string]string, visible map[string]bool) string {
	for range maxFoldDepth {
		parent, folded := parents[id]
		if !folded {
			return ""
		}
		if visible[parent] {
			return parent
		}
		id = parent
	}
	return ""
}

func (b *builder) crowded(visible map[string]bool) bool {
	if len(visible) <= nodeBudget {
		return false
	}
	drawn := map[string]bool{}
	for id := range visible {
		drawn[b.objects[id].node.Namespace] = true
	}
	return len(drawn) > 1
}

func (b *builder) districts(
	ids []string,
	parents map[string]string,
	expanded map[string]bool,
) ([]string, map[string]string, map[string]bool) {
	wider := map[string]string{}
	maps.Copy(wider, parents)
	exposed := map[string]bool{}
	for _, id := range ids {
		if _, folded := wider[id]; folded {
			continue
		}
		district := b.district(b.objects[id].node.Namespace)
		wider[id] = district
		if expanded[district] {
			continue
		}
		if b.objects[id].node.Ready == readyFalse {
			exposed[id] = true
		}
	}
	withDistricts := b.ids()
	visible := visibleSet(withDistricts, wider, expanded)
	for id := range exposed {
		visible[id] = true
	}
	return withDistricts, wider, visible
}

func (b *builder) district(namespace string) string {
	id := districtIDTag + namespace
	_, known := b.objects[id]
	if known {
		return id
	}
	name := namespace
	if name == "" {
		name = "cluster-scoped"
	}
	b.objects[id] = &object{node: api.GraphNode{
		ID:        id,
		Kind:      kindNamespace,
		Group:     "",
		Version:   "v1",
		Resource:  namespaceResource(namespace),
		Name:      name,
		Namespace: "",
		Status:    "",
		Ready:     readyUnknown,
		Category:  categoryNamespace,
	}}
	return id
}

func namespaceResource(namespace string) string {
	if namespace == "" {
		return ""
	}
	return "namespaces"
}

func (b *builder) nodesFor(ids []string, parents map[string]string, visible map[string]bool) []api.GraphNode {
	contains := map[string]int{}
	unhealthy := map[string]int{}
	for _, id := range ids {
		if visible[id] {
			continue
		}
		anchor := anchorOf(id, parents, visible)
		if anchor == "" {
			continue
		}
		contains[anchor]++
		if b.objects[id].node.Ready == readyFalse {
			unhealthy[anchor]++
		}
	}
	out := make([]api.GraphNode, 0, len(visible))
	for _, id := range ids {
		if !visible[id] {
			continue
		}
		node := b.objects[id].node
		node.Contains = contains[id]
		node.Unhealthy = unhealthy[id]
		if node.Unhealthy > 0 {
			node.Ready = readyFalse
		}
		out = append(out, node)
	}
	return out
}

func (b *builder) edgesFor(parents map[string]string, visible map[string]bool) []api.GraphEdge {
	kept := map[string]api.GraphEdge{}
	for _, edge := range b.edges {
		from := resolve(edge.From, parents, visible)
		to := resolve(edge.To, parents, visible)
		if from == "" || to == "" || from == to {
			continue
		}
		kept[from+"|"+to+"|"+edge.Kind] = api.GraphEdge{From: from, To: to, Kind: edge.Kind}
	}
	out := make([]api.GraphEdge, 0, len(kept))
	for _, edge := range kept {
		out = append(out, edge)
	}
	return out
}

func resolve(id string, parents map[string]string, visible map[string]bool) string {
	if visible[id] {
		return id
	}
	return anchorOf(id, parents, visible)
}

func (b *builder) neighborhood(
	root api.ObjectRef,
	parents map[string]string,
	visible map[string]bool,
	nodes []api.GraphNode,
	edges []api.GraphEdge,
) ([]api.GraphNode, []api.GraphEdge) {
	start := b.rootID(root, parents, visible)
	if start == "" {
		return nil, nil
	}
	near := map[string]bool{start: true}
	frontier := map[string]bool{start: true}
	for range rootDepth {
		next := map[string]bool{}
		for _, edge := range edges {
			for _, id := range step(edge, near, frontier) {
				next[id] = true
			}
		}
		for id := range next {
			near[id] = true
		}
		frontier = next
	}
	keptNodes := make([]api.GraphNode, 0, len(near))
	for _, node := range nodes {
		if !near[node.ID] {
			continue
		}
		keptNodes = append(keptNodes, node)
	}
	keptEdges := make([]api.GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if !near[edge.From] || !near[edge.To] {
			continue
		}
		keptEdges = append(keptEdges, edge)
	}
	return keptNodes, keptEdges
}

func step(edge api.GraphEdge, near, frontier map[string]bool) []string {
	out := []string{}
	if frontier[edge.From] && !near[edge.To] {
		out = append(out, edge.To)
	}
	if frontier[edge.To] && !near[edge.From] {
		out = append(out, edge.From)
	}
	return out
}

func (b *builder) rootID(root api.ObjectRef, parents map[string]string, visible map[string]bool) string {
	kind := b.kindFor[root.Group+"/"+root.Resource]
	if kind == "" {
		return ""
	}
	id, known := b.identity[identityKey(root.Group, kind, root.Namespace, root.Name)]
	if !known {
		return ""
	}
	return resolve(id, parents, visible)
}
