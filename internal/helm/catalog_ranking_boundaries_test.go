package helm

import (
	"slices"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestChartNamesThatDoNotContainTheQuerySortLast(t *testing.T) {
	hits := []api.HelmChartHit{
		{Chart: "worker"},
		{Chart: "my-api"},
		{Chart: "api-gateway"},
		{Chart: "api"},
	}

	slices.SortFunc(hits, closerFirst("api"))

	want := []string{"api", "api-gateway", "my-api", "worker"}
	for at, hit := range hits {
		if hit.Chart != want[at] {
			t.Fatalf("chart %d = %q, want %q; order = %+v", at, hit.Chart, want[at], hits)
		}
	}
}
