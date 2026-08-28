package gitops

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func appFor(resources ...api.GitopsResource) api.GitopsApp {
	return api.GitopsApp{
		Ref: api.ObjectRef{
			Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
			Namespace: "argocd", Name: "podinfo",
		},
		Controller: api.ControllerArgo,
		Kind:       "Application",
		Name:       "podinfo",
		Namespace:  "argocd",
		State:      api.GitopsState{Sync: "Synced", Health: "Healthy"},
		Resources:  resources,
	}
}

func nodeNamed(graph api.Graph, name string) (api.GraphNode, bool) {
	for _, one := range graph.Nodes {
		if one.Name == name {
			return one, true
		}
	}
	return api.GraphNode{}, false
}

func TestAppGraphPutsTheApplicationAtTheRoot(t *testing.T) {
	graph := AppGraph(appFor())

	if len(graph.Nodes) != 1 {
		t.Fatalf("nodes = %+v, want only the application", graph.Nodes)
	}
	root := graph.Nodes[0]
	if root.Category != "app" || root.Name != "podinfo" {
		t.Fatalf("root = %+v", root)
	}
	if root.Status != "Synced Healthy" || root.Ready != readyTrue {
		t.Fatalf("root state = %+v", root)
	}
}

func TestAFluxApplierIsTheRootToo(t *testing.T) {
	app := appFor()
	app.Controller = api.ControllerFlux
	app.Kind = "Kustomization"

	graph := AppGraph(app)

	if graph.Nodes[0].Category != "applier" {
		t.Fatalf("category = %q, want applier", graph.Nodes[0].Category)
	}
}

func TestEveryManagedResourceHangsOffTheRoot(t *testing.T) {
	graph := AppGraph(appFor(
		api.GitopsResource{Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "shop", Sync: "Synced", Health: "Healthy"},
		api.GitopsResource{Version: "v1", Kind: "Service", Name: "web", Namespace: "shop", Sync: "Synced"},
	))

	if len(graph.Nodes) != 3 {
		t.Fatalf("nodes = %d, want the root and its two resources", len(graph.Nodes))
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("edges = %+v, want one per resource", graph.Edges)
	}
	for _, edge := range graph.Edges {
		if edge.From != graph.Nodes[0].ID || edge.Kind != "manages" {
			t.Fatalf("edge = %+v, want it to come from the root", edge)
		}
	}
	deployment, found := nodeNamed(graph, "web")
	if !found || deployment.Category != categoryManaged {
		t.Fatalf("deployment = %+v, want a managed node", deployment)
	}
}

func TestAChildApplicationReadsAsAnApplicationNotAResource(t *testing.T) {
	graph := AppGraph(appFor(api.GitopsResource{
		Group: "argoproj.io", Version: "v1alpha1", Kind: "Application",
		Name: "child", Namespace: "argocd", Sync: "Synced", Health: "Healthy",
	}))

	child, found := nodeNamed(graph, "child")

	if !found || child.Category != "app" {
		t.Fatalf("child = %+v, want it to read as an application", child)
	}
}

func TestATerminatingResourceSaysSoAndReadsAsNotReady(t *testing.T) {
	graph := AppGraph(appFor(api.GitopsResource{
		Version: "v1", Kind: "Service", Name: "web", Namespace: "shop",
		Sync: "Synced", Health: "Healthy", Terminating: true,
	}))

	node, _ := nodeNamed(graph, "web")

	if node.Status != "Terminating" || node.Ready != readyFalse {
		t.Fatalf("node = %+v, want it marked as going away", node)
	}
}

func TestABrokenResourceReadsAsNotReady(t *testing.T) {
	graph := AppGraph(appFor(api.GitopsResource{
		Version: "v1", Kind: "Service", Name: "web", Namespace: "shop", Health: "Degraded",
	}))

	node, _ := nodeNamed(graph, "web")

	if node.Ready != readyFalse {
		t.Fatalf("ready = %q, want False", node.Ready)
	}
}

func TestAResourceWithNoVerdictAtAllReadsAsUnknown(t *testing.T) {
	graph := AppGraph(appFor(api.GitopsResource{
		Version: "v1", Kind: "Service", Name: "web", Namespace: "shop",
	}))

	node, _ := nodeNamed(graph, "web")

	if node.Ready != readyUnknown {
		t.Fatalf("ready = %q, want Unknown", node.Ready)
	}
	if node.Status != "" {
		t.Fatalf("status = %q, want nothing invented", node.Status)
	}
}

func TestAnUnhealthyApplicationReadsAsNotReady(t *testing.T) {
	app := appFor()
	app.State = api.GitopsState{Sync: "OutOfSync", Health: "Degraded"}

	if got := AppGraph(app).Nodes[0].Ready; got != readyFalse {
		t.Fatalf("ready = %q, want False", got)
	}
}

func TestAnApplicationStillProgressingReadsAsUnknown(t *testing.T) {
	app := appFor()
	app.State = api.GitopsState{Sync: "OutOfSync", Health: "Progressing"}

	if got := AppGraph(app).Nodes[0].Ready; got != readyUnknown {
		t.Fatalf("ready = %q, want Unknown", got)
	}
}

func TestTheGraphCarriesWhateverWentWrongReadingTheApplication(t *testing.T) {
	app := appFor()
	app.Error = "some kinds could not be listed"

	if got := AppGraph(app).Error; got != app.Error {
		t.Fatalf("error = %q, want it carried through", got)
	}
}

func TestManagedNodesComeBackInAStableOrder(t *testing.T) {
	graph := AppGraph(appFor(
		api.GitopsResource{Version: "v1", Kind: "Service", Name: "zeta", Namespace: "shop"},
		api.GitopsResource{Version: "v1", Kind: "Service", Name: "alpha", Namespace: "shop"},
	))

	if graph.Nodes[1].Name != "alpha" || graph.Nodes[2].Name != "zeta" {
		t.Fatalf("nodes = %+v, want them sorted", graph.Nodes)
	}
	if graph.Edges[0].To != graph.Nodes[1].ID {
		t.Fatalf("edges = %+v, want them sorted with the nodes", graph.Edges)
	}
}
