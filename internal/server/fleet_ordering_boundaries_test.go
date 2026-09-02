package server

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestFleetAppsWithTheSameNameAndClusterAreOrderedByNamespace(t *testing.T) {
	got := mergeGitops([]clusterAnswer[delivery]{
		{
			cluster: "p-mk1",
			answer: delivery{flux: api.FluxDashboard{Groups: []api.FluxGroup{
				{Resources: []api.FluxResource{
					{Name: "platform", Namespace: "z-system"},
					{Name: "platform", Namespace: "a-system"},
				}},
			}}},
		},
	})

	if len(got.Apps) != 2 {
		t.Fatalf("apps = %d, want 2", len(got.Apps))
	}
	if got.Apps[0].Namespace != "a-system" || got.Apps[1].Namespace != "z-system" {
		t.Fatalf("namespaces = %q, %q, want stable namespace order", got.Apps[0].Namespace, got.Apps[1].Namespace)
	}
}

func TestFleetHitsWithTheSameNameAndClusterAreOrderedByNamespace(t *testing.T) {
	got := mergeSearch([]clusterAnswer[api.SearchResults]{
		{
			cluster: "p-mk1",
			answer: api.SearchResults{Hits: []api.SearchHit{
				{Name: "web", Namespace: "z-system"},
				{Name: "web", Namespace: "a-system"},
			}},
		},
	})

	if len(got.Hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(got.Hits))
	}
	if got.Hits[0].Namespace != "a-system" || got.Hits[1].Namespace != "z-system" {
		t.Fatalf("namespaces = %q, %q, want stable namespace order", got.Hits[0].Namespace, got.Hits[1].Namespace)
	}
}
