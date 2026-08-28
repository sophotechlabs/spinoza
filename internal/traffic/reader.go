package traffic

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

type Querier interface {
	Instant(ctx context.Context, query string, at time.Time) ([]prom.Sample, error)
}

type Reader struct {
	prom   Querier
	meshes []mesh
}

func New(client Querier) *Reader {
	return &Reader{prom: client, meshes: meshes}
}

type state int

const (
	missing state = iota
	unlabeled
	ready
)

type reading struct {
	mesh    mesh
	samples []prom.Sample
	state   state
}

func (r *Reader) Support(ctx context.Context, at time.Time) api.TrafficSupport {
	found, err := r.read(ctx, at)
	if err != nil {
		return api.TrafficSupport{Reason: err.Error()}
	}
	if found.state == ready {
		return api.TrafficSupport{Available: true, Source: found.mesh.name}
	}
	return api.TrafficSupport{Reason: reasonFor(found), Source: sourceFor(found)}
}

func (r *Reader) Graph(ctx context.Context, at time.Time) api.TrafficGraph {
	found, err := r.read(ctx, at)
	if err != nil {
		return nothing("", err.Error())
	}
	if found.state != ready {
		return nothing(sourceFor(found), reasonFor(found))
	}
	nodes, edges := build(found.mesh, found.samples)
	if len(nodes) <= nodeBudget {
		return api.TrafficGraph{Source: found.mesh.name, Nodes: nodes, Edges: edges}
	}
	districts, between := foldToNamespaces(nodes, edges)
	return api.TrafficGraph{
		Source:    found.mesh.name,
		Nodes:     districts,
		Edges:     between,
		Folded:    true,
		Workloads: len(nodes),
	}
}

func nothing(source, reason string) api.TrafficGraph {
	return api.TrafficGraph{
		Source: source,
		Nodes:  []api.TrafficNode{},
		Edges:  []api.TrafficEdge{},
		Error:  reason,
	}
}

func reasonFor(found reading) string {
	if found.state == unlabeled {
		return found.mesh.hint
	}
	return fmt.Sprintf("no service mesh flow metrics were found in Prometheus; spinoza reads %s", meshNames())
}

func foldToNamespaces(nodes []api.TrafficNode, edges []api.TrafficEdge) ([]api.TrafficNode, []api.TrafficEdge) {
	district := map[string]string{}
	kept := map[string]api.TrafficNode{}
	for _, node := range nodes {
		id := node.Namespace
		if id == "" {
			id = node.ID
		}
		district[node.ID] = id
		kept[id] = api.TrafficNode{ID: id, Namespace: node.Namespace}
	}
	between := map[string]api.TrafficEdge{}
	for _, edge := range edges {
		from := district[edge.From]
		to := district[edge.To]
		key := from + "->" + to
		folded, seen := between[key]
		if !seen {
			folded = api.TrafficEdge{From: from, To: to}
		}
		folded.Rate += edge.Rate
		folded.Dropped += edge.Dropped
		between[key] = folded
	}
	return sortedNodes(kept), sortedEdges(between)
}

func sourceFor(found reading) string {
	if found.state == missing {
		return ""
	}
	return found.mesh.name
}

func (r *Reader) read(ctx context.Context, at time.Time) (reading, error) {
	bounded, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	found := reading{state: missing}
	for _, entry := range r.meshes {
		samples, err := r.prom.Instant(bounded, entry.flows, at)
		if err != nil {
			return reading{}, err
		}
		if anyLabelled(entry, samples) {
			return reading{mesh: entry, samples: samples, state: ready}, nil
		}
		entryState, err := r.probe(bounded, entry, at)
		if err != nil {
			return reading{}, err
		}
		if entryState == ready {
			return reading{mesh: entry, state: ready}, nil
		}
		if entryState == unlabeled && found.state == missing {
			found = reading{mesh: entry, state: unlabeled}
		}
	}
	return found, nil
}

func (r *Reader) probe(ctx context.Context, entry mesh, at time.Time) (state, error) {
	labeled, err := r.prom.Instant(ctx, entry.labeled, at)
	if err != nil {
		return missing, err
	}
	if len(labeled) > 0 {
		return ready, nil
	}
	present, err := r.prom.Instant(ctx, entry.present, at)
	if err != nil {
		return missing, err
	}
	if len(present) > 0 {
		return unlabeled, nil
	}
	return missing, nil
}

func anyLabelled(entry mesh, samples []prom.Sample) bool {
	for _, sample := range samples {
		_, ok := edgeEnds(entry, sample)
		if ok {
			return true
		}
	}
	return false
}

type ends struct {
	from api.TrafficNode
	to   api.TrafficNode
}

func edgeEnds(entry mesh, sample prom.Sample) (ends, bool) {
	from, ok := nodeOf(entry.from, sample)
	if !ok {
		return ends{}, false
	}
	to, ok := nodeOf(entry.to, sample)
	if !ok {
		return ends{}, false
	}
	return ends{from: from, to: to}, true
}

func nodeOf(side endpoint, sample prom.Sample) (api.TrafficNode, bool) {
	workload := sample.Labels[side.workload]
	if workload == "" {
		return api.TrafficNode{}, false
	}
	namespace := sample.Labels[side.namespace]
	return api.TrafficNode{ID: namespace + "/" + workload, Namespace: namespace, Workload: workload}, true
}

func build(entry mesh, samples []prom.Sample) ([]api.TrafficNode, []api.TrafficEdge) {
	nodes := map[string]api.TrafficNode{}
	edges := map[string]api.TrafficEdge{}
	for _, sample := range samples {
		pair, ok := edgeEnds(entry, sample)
		if !ok {
			continue
		}
		verdict := sample.Labels[entry.verdict]
		if verdict != forwarded && verdict != dropped {
			continue
		}
		nodes[pair.from.ID] = pair.from
		nodes[pair.to.ID] = pair.to
		key := pair.from.ID + "->" + pair.to.ID
		edge, seen := edges[key]
		if !seen {
			edge = api.TrafficEdge{From: pair.from.ID, To: pair.to.ID}
		}
		if verdict == forwarded {
			edge.Rate += sample.Value
		}
		if verdict == dropped {
			edge.Dropped += sample.Value
		}
		edges[key] = edge
	}
	return sortedNodes(nodes), sortedEdges(edges)
}

func sortedNodes(found map[string]api.TrafficNode) []api.TrafficNode {
	out := make([]api.TrafficNode, 0, len(found))
	for _, node := range found {
		out = append(out, node)
	}
	slices.SortFunc(out, func(a, b api.TrafficNode) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return out
}

func sortedEdges(found map[string]api.TrafficEdge) []api.TrafficEdge {
	out := make([]api.TrafficEdge, 0, len(found))
	for _, edge := range found {
		out = append(out, edge)
	}
	slices.SortFunc(out, func(a, b api.TrafficEdge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return out
}
