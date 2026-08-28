package traffic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/prom"
)

type stubQuerier struct {
	answers map[string][]prom.Sample
	fails   map[string]error
	asked   []string
}

func (s *stubQuerier) Instant(_ context.Context, query string, _ time.Time) ([]prom.Sample, error) {
	s.asked = append(s.asked, query)
	err, failed := s.fails[query]
	if failed {
		return nil, err
	}
	return s.answers[query], nil
}

func flow(source, destination, verdict string, value float64) prom.Sample {
	return prom.Sample{
		Labels: map[string]string{
			"source_namespace":      "apps",
			"source_workload":       source,
			"destination_namespace": "apps",
			"destination_workload":  destination,
			"verdict":               verdict,
		},
		Value: value,
	}
}

func counted(value float64) []prom.Sample {
	return []prom.Sample{{Labels: map[string]string{}, Value: value}}
}

func at() time.Time {
	return time.Unix(1787933018, 0)
}

func TestGraphBuildsWorkloadEdges(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {
			flow("web", "api", forwarded, 12),
			flow("web", "api", dropped, 0.5),
			flow("beat", "api", forwarded, 3),
		},
	}}
	graph := New(querier).Graph(context.Background(), at())

	if graph.Error != "" {
		t.Fatalf("graph reported %q", graph.Error)
	}
	if graph.Source != cilium.name {
		t.Fatalf("source is %q, want %q", graph.Source, cilium.name)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("got %d nodes, want 3: %+v", len(graph.Nodes), graph.Nodes)
	}
	if graph.Nodes[0].ID != "apps/api" {
		t.Fatalf("nodes are not sorted: %+v", graph.Nodes)
	}
	if graph.Nodes[0].Workload != "api" || graph.Nodes[0].Namespace != "apps" {
		t.Fatalf("node lost its identity: %+v", graph.Nodes[0])
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("got %d edges, want 2: %+v", len(graph.Edges), graph.Edges)
	}
	if graph.Edges[0].From != "apps/beat" {
		t.Fatalf("edges are not sorted: %+v", graph.Edges)
	}
	web := graph.Edges[1]
	if web.From != "apps/web" || web.To != "apps/api" {
		t.Fatalf("edge points the wrong way: %+v", web)
	}
	if web.Rate != 12 {
		t.Fatalf("forwarded rate is %v, want 12", web.Rate)
	}
	if web.Dropped != 0.5 {
		t.Fatalf("dropped rate is %v, want 0.5", web.Dropped)
	}
}

func TestGraphSortsEdgesBySourceThenDestination(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {
			flow("web", "store", forwarded, 1),
			flow("web", "api", forwarded, 1),
		},
	}}
	graph := New(querier).Graph(context.Background(), at())

	if graph.Edges[0].To != "apps/api" {
		t.Fatalf("edges with one source are not sorted: %+v", graph.Edges)
	}
}

func TestGraphIgnoresOtherVerdicts(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {
			flow("web", "api", forwarded, 4),
			flow("web", "api", "TRACED", 99),
		},
	}}
	graph := New(querier).Graph(context.Background(), at())

	if len(graph.Edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(graph.Edges))
	}
	if graph.Edges[0].Rate != 4 {
		t.Fatalf("a traced flow was counted: %+v", graph.Edges[0])
	}
}

func TestGraphSkipsFlowsWithoutBothWorkloads(t *testing.T) {
	outbound := flow("web", "", forwarded, 5)
	inbound := flow("", "api", forwarded, 5)
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {outbound, inbound, flow("web", "api", forwarded, 1)},
	}}
	graph := New(querier).Graph(context.Background(), at())

	if len(graph.Edges) != 1 {
		t.Fatalf("got %d edges, want only the workload-to-workload one: %+v", len(graph.Edges), graph.Edges)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2: %+v", len(graph.Nodes), graph.Nodes)
	}
}

func TestGraphAndSupportSayWhatToConfigure(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows:   {{Labels: map[string]string{"verdict": forwarded}, Value: 138}},
		cilium.present: counted(12),
	}}
	reader := New(querier)

	support := reader.Support(context.Background(), at())
	if support.Available {
		t.Fatal("unlabeled metrics were reported as available")
	}
	if support.Reason != cilium.hint {
		t.Fatalf("reason is %q, want the configuration hint", support.Reason)
	}
	if support.Source != cilium.name {
		t.Fatalf("source is %q, want %q", support.Source, cilium.name)
	}

	graph := reader.Graph(context.Background(), at())
	if graph.Error != cilium.hint {
		t.Fatalf("graph error is %q, want the configuration hint", graph.Error)
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("an unlabeled read produced edges: %+v", graph.Edges)
	}
	if graph.Nodes == nil || graph.Edges == nil {
		t.Fatalf("a refused graph sent null instead of an empty list: %+v", graph)
	}
}

func TestLabelledButIdleIsAvailable(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.labeled: counted(4),
	}}
	reader := New(querier)

	support := reader.Support(context.Background(), at())
	if !support.Available {
		t.Fatalf("a quiet cluster was reported unavailable: %q", support.Reason)
	}

	graph := reader.Graph(context.Background(), at())
	if graph.Error != "" {
		t.Fatalf("graph reported %q", graph.Error)
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("got edges from no samples: %+v", graph.Edges)
	}
	if graph.Source != cilium.name {
		t.Fatalf("source is %q, want %q", graph.Source, cilium.name)
	}
}

func TestNoMeshFound(t *testing.T) {
	reader := New(&stubQuerier{})

	support := reader.Support(context.Background(), at())
	if support.Available {
		t.Fatal("an empty Prometheus was reported as available")
	}
	if support.Source != "" {
		t.Fatalf("source is %q, want none", support.Source)
	}
	if support.Reason == "" {
		t.Fatal("no reason was given")
	}

	graph := reader.Graph(context.Background(), at())
	if graph.Error != support.Reason {
		t.Fatalf("graph and support disagree: %q and %q", graph.Error, support.Reason)
	}
}

func TestHappyPathAsksOneQuery(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.flows: {flow("web", "api", forwarded, 1)},
	}}
	New(querier).Graph(context.Background(), at())

	if len(querier.asked) != 1 {
		t.Fatalf("asked %d queries, want 1: %v", len(querier.asked), querier.asked)
	}
}

func TestPrometheusFailuresSurface(t *testing.T) {
	cases := []struct {
		name  string
		fails map[string]error
		seed  map[string][]prom.Sample
	}{
		{
			name:  "flows",
			fails: map[string]error{cilium.flows: errors.New("flows are unreachable")},
		},
		{
			name:  "labeled",
			fails: map[string]error{cilium.labeled: errors.New("labeled is unreachable")},
		},
		{
			name:  "present",
			fails: map[string]error{cilium.present: errors.New("present is unreachable")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			querier := &stubQuerier{answers: tc.seed, fails: tc.fails}
			reader := New(querier)

			support := reader.Support(context.Background(), at())
			if support.Available {
				t.Fatal("a failed read was reported as available")
			}
			if support.Reason == "" {
				t.Fatal("a failed read gave no reason")
			}

			graph := reader.Graph(context.Background(), at())
			if graph.Error == "" {
				t.Fatal("a failed read produced no error")
			}
		})
	}
}

func TestMeshNamesListsEveryMesh(t *testing.T) {
	names := meshNames()
	for _, entry := range meshes {
		if !strings.Contains(names, entry.name) {
			t.Fatalf("%q is missing from %q", entry.name, names)
		}
	}
}
