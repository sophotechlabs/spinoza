package traffic

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/prom"
)

func TestGraphSaturatesANegativelyOverflowingDroppedRate(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {
			flow("web", "api", dropped, -math.MaxFloat64),
			flow("web", "api", dropped, -math.MaxFloat64),
		},
	}}

	graph := New(querier).Graph(context.Background(), at())

	if len(graph.Edges) != 1 || graph.Edges[0].Dropped != -math.MaxFloat64 {
		t.Fatalf("edges = %+v, want a finite saturated dropped rate", graph.Edges)
	}
	if _, err := json.Marshal(graph); err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
}
