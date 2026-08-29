package traffic

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
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

func TestLabeledButIdleIsAvailable(t *testing.T) {
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

// what a crowded cluster gets instead of a refusal

func crowded(pairs int) []prom.Sample {
	samples := make([]prom.Sample, 0, pairs)
	for i := range pairs {
		samples = append(samples, prom.Sample{
			Labels: map[string]string{
				"source_namespace":      fmt.Sprintf("team-%d", i%3),
				"source_workload":       fmt.Sprintf("web-%d", i),
				"destination_namespace": "data",
				"destination_workload":  "postgres",
				"verdict":               forwarded,
			},
			Value: 1,
		})
	}
	return samples
}

func TestAGraphAtTheBudgetIsNotFolded(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{cilium.flows: crowded(nodeBudget - 1)}}

	graph := New(querier).Graph(context.Background(), at())

	if graph.Folded {
		t.Fatalf("a graph of %d workloads was folded", len(graph.Nodes))
	}
	if len(graph.Nodes) != nodeBudget {
		t.Fatalf("nodes = %d, want the %d sources plus the one destination", len(graph.Nodes), nodeBudget-1)
	}
	if graph.Workloads != 0 {
		t.Fatalf("workloads = %d, want it left out when nothing was folded", graph.Workloads)
	}
}

func TestPastTheBudgetTheGraphFoldsToNamespaces(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{cilium.flows: crowded(nodeBudget + 10)}}

	graph := New(querier).Graph(context.Background(), at())

	if !graph.Folded {
		t.Fatal("a graph past the budget was not folded")
	}
	if graph.Workloads != nodeBudget+11 {
		t.Fatalf("workloads = %d, want the count before folding", graph.Workloads)
	}
	ids := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		ids = append(ids, node.ID)
	}
	want := []string{"data", "team-0", "team-1", "team-2"}
	if !slices.Equal(ids, want) {
		t.Fatalf("districts = %v, want %v", ids, want)
	}
	if len(graph.Edges) != 3 {
		t.Fatalf("edges = %d, want one per source namespace: %+v", len(graph.Edges), graph.Edges)
	}
}

func TestFoldingSumsTheRatesItMerges(t *testing.T) {
	samples := crowded(nodeBudget + 10)
	for i := range samples {
		samples[i].Value = 2
	}
	querier := &stubQuerier{answers: map[string][]prom.Sample{cilium.flows: samples}}

	graph := New(querier).Graph(context.Background(), at())

	total := 0.0
	for _, edge := range graph.Edges {
		total += edge.Rate
	}
	want := float64(2 * (nodeBudget + 10))
	if total != want {
		t.Fatalf("folded rate = %v, want the %v the workloads carried", total, want)
	}
}

func TestFoldingKeepsAWorkloadWithNoNamespaceApart(t *testing.T) {
	samples := crowded(nodeBudget + 10)
	samples = append(samples, prom.Sample{
		Labels: map[string]string{
			"source_workload":       "orphan",
			"destination_namespace": "data",
			"destination_workload":  "postgres",
			"verdict":               forwarded,
		},
		Value: 1,
	})
	querier := &stubQuerier{answers: map[string][]prom.Sample{cilium.flows: samples}}

	graph := New(querier).Graph(context.Background(), at())

	for _, node := range graph.Nodes {
		if node.ID == "" {
			t.Fatalf("folding produced a district with no id: %+v", graph.Nodes)
		}
	}
	if !slices.ContainsFunc(graph.Nodes, func(node api.TrafficNode) bool { return node.ID == "/orphan" }) {
		t.Fatalf("the namespaceless workload lost its identity: %+v", graph.Nodes)
	}
}

// a second mesh is a row in the table, and the reader has to reach it

func TestTheReaderFallsThroughToTheNextMesh(t *testing.T) {
	second := mesh{
		name:    "Another Mesh",
		present: "count(other_flows_total)",
		labeled: `count(other_flows_total{src!=""})`,
		flows:   "sum by (src, dst, verdict) (rate(other_flows_total[5m]))",
		from:    endpoint{namespace: "src_ns", workload: "src"},
		to:      endpoint{namespace: "dst_ns", workload: "dst"},
		verdict: "verdict",
		hint:    "add labels to the other mesh",
	}
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		second.flows: {{
			Labels: map[string]string{
				"src_ns":  "apps",
				"src":     "web",
				"dst_ns":  "data",
				"dst":     "redis",
				"verdict": forwarded,
			},
			Value: 4,
		}},
	}}
	reader := &Reader{prom: querier, meshes: []mesh{cilium, second}}

	graph := reader.Graph(context.Background(), at())

	if graph.Source != second.name {
		t.Fatalf("source = %q, want the mesh that answered", graph.Source)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edges = %d, want the one the second mesh reported", len(graph.Edges))
	}
	if graph.Edges[0].From != "apps/web" {
		t.Fatalf("edge = %+v, want it read through the second mapping", graph.Edges[0])
	}
}

func TestAnUnlabeledFirstMeshIsReportedWhenNoneAnswer(t *testing.T) {
	second := mesh{
		name:    "Another Mesh",
		present: "count(other_flows_total)",
		labeled: `count(other_flows_total{src!=""})`,
		flows:   "sum by (src, dst) (rate(other_flows_total[5m]))",
		from:    endpoint{namespace: "src_ns", workload: "src"},
		to:      endpoint{namespace: "dst_ns", workload: "dst"},
		verdict: "verdict",
		hint:    "add labels to the other mesh",
	}
	querier := &stubQuerier{answers: map[string][]prom.Sample{cilium.present: counted(12)}}
	reader := &Reader{prom: querier, meshes: []mesh{cilium, second}}

	support := reader.Support(context.Background(), at())

	if support.Source != cilium.name {
		t.Fatalf("source = %q, want the mesh that was at least present", support.Source)
	}
	if support.Reason != cilium.hint {
		t.Fatalf("reason = %q, want that mesh's hint", support.Reason)
	}
}

// the read carries its own deadline, so a slow prometheus cannot hold a request open

type deadlineQuerier struct {
	bounded bool
	seen    bool
}

func (d *deadlineQuerier) Instant(ctx context.Context, _ string, _ time.Time) ([]prom.Sample, error) {
	d.seen = true
	_, d.bounded = ctx.Deadline()
	return nil, nil
}

func TestTheReadBoundsWhateverContextItIsGiven(t *testing.T) {
	querier := &deadlineQuerier{}

	New(querier).Support(context.Background(), at())

	if !querier.seen {
		t.Fatal("prometheus was never asked")
	}
	if !querier.bounded {
		t.Fatal("the query ran without a deadline, so a slow prometheus holds the request open")
	}
}

func TestACancelledContextStopsTheRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	querier := &contextQuerier{}

	graph := New(querier).Graph(ctx, at())

	if graph.Error == "" {
		t.Fatal("a canceled read produced no error")
	}
}

type contextQuerier struct{}

func (contextQuerier) Instant(ctx context.Context, _ string, _ time.Time) ([]prom.Sample, error) {
	return nil, ctx.Err()
}

func TestAClusterMidRolloutIsNotCalledReady(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.present: counted(302),
		cilium.labeled: counted(12),
	}}
	reader := New(querier)

	support := reader.Support(context.Background(), at())

	if support.Available {
		t.Fatal(
			"12 labeled series out of 302 was called ready; one restarted agent would draw " +
				"a near-empty graph with nothing saying why",
		)
	}
	if !strings.Contains(support.Reason, "labelsContext") {
		t.Fatalf("reason = %q, want it to name the fix", support.Reason)
	}
}

func TestAClusterThatFinishedItsRolloutIsReady(t *testing.T) {
	querier := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.present: counted(302),
		cilium.labeled: counted(195),
	}}
	reader := New(querier)

	support := reader.Support(context.Background(), at())

	if !support.Available {
		t.Fatalf(
			"195 of 302 series carry workload labels, which is what p-mk2 answered once its "+
				"agent restarted, and it was reported unavailable: %q", support.Reason,
		)
	}
}

func TestFlowsWithNoLabelsAtAllStaySeparateFromNoFlows(t *testing.T) {
	unlabeled := &stubQuerier{answers: map[string][]prom.Sample{
		cilium.present: counted(12),
	}}
	nothing := &stubQuerier{answers: map[string][]prom.Sample{}}

	first := New(unlabeled).Support(context.Background(), at())
	second := New(nothing).Support(context.Background(), at())

	if first.Available || second.Available {
		t.Fatal("a cluster with no workload labels was reported available")
	}
	if first.Reason == second.Reason {
		t.Fatalf(
			"a cluster exporting unlabeled flows and one exporting none read the same: %q",
			first.Reason,
		)
	}
}
