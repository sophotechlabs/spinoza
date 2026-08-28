package gitops

import (
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
)

func AppGraph(app api.GitopsApp) api.Graph {
	root := api.GraphNode{
		ID:        nodeID(app.Ref.Group, app.Kind, app.Namespace, app.Name),
		Kind:      app.Kind,
		Group:     app.Ref.Group,
		Version:   app.Ref.Version,
		Resource:  app.Ref.Resource,
		Name:      app.Name,
		Namespace: app.Namespace,
		Status:    rootStatus(app.State),
		Ready:     rootReady(app.State),
		Category:  rootCategory(app.Controller),
	}
	nodes := make([]api.GraphNode, 0, 1+len(app.Resources))
	nodes = append(nodes, root)
	edges := make([]api.GraphEdge, 0, len(app.Resources))
	for _, one := range app.Resources {
		child := managedNode(one)
		nodes = append(nodes, child)
		edges = append(edges, api.GraphEdge{From: root.ID, To: child.ID, Kind: "manages"})
	}
	slices.SortFunc(nodes[1:], func(left, right api.GraphNode) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(edges, func(left, right api.GraphEdge) int {
		return strings.Compare(left.To, right.To)
	})
	return api.Graph{Nodes: nodes, Edges: edges, Error: app.Error}
}

func rootCategory(controller string) string {
	if controller == api.ControllerArgo {
		return "app"
	}
	return "applier"
}

func rootStatus(state api.GitopsState) string {
	return strings.TrimSpace(state.Sync + " " + state.Health)
}

func rootReady(state api.GitopsState) string {
	if state.Health == "Degraded" || state.Health == "Missing" {
		return readyFalse
	}
	if state.Health == "Healthy" && state.Sync == "Synced" {
		return readyTrue
	}
	return readyUnknown
}

func managedNode(one api.GitopsResource) api.GraphNode {
	return api.GraphNode{
		ID:        nodeID(one.Group, one.Kind, one.Namespace, one.Name),
		Kind:      one.Kind,
		Group:     one.Group,
		Version:   one.Version,
		Resource:  one.Resource,
		Name:      one.Name,
		Namespace: one.Namespace,
		Status:    managedStatus(one),
		Ready:     managedReady(one),
		Category:  managedCategory(one),
	}
}

func managedCategory(one api.GitopsResource) string {
	if one.Group == argocd.Group && one.Kind == argocd.IsApplication {
		return "app"
	}
	return categoryManaged
}

func managedStatus(one api.GitopsResource) string {
	if one.Terminating {
		return "Terminating"
	}
	return strings.TrimSpace(one.Sync + " " + one.Health)
}

func managedReady(one api.GitopsResource) string {
	if one.Terminating {
		return readyFalse
	}
	if one.Health == "Degraded" || one.Health == "Missing" {
		return readyFalse
	}
	if one.Health == "Healthy" || one.Sync == "Synced" {
		return readyTrue
	}
	return readyUnknown
}
