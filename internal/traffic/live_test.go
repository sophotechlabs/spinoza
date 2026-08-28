package traffic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/sophotechlabs/spinoza/internal/prom"
)

type promServer struct {
	asked   []string
	answers map[string]string
}

func (p *promServer) handle(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/status/buildinfo") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"version":"3.13.2"}}`))
		return
	}
	query := r.URL.Query().Get("query")
	p.asked = append(p.asked, query)
	body, found := p.answers[query]
	if !found {
		body = emptyVector
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

const emptyVector = `{"status":"success","data":{"resultType":"vector","result":[]}}`

func vector(t *testing.T, samples ...map[string]any) string {
	t.Helper()
	result := make([]map[string]any, 0, len(samples))
	result = append(result, samples...)
	body := map[string]any{
		"status": "success",
		"data":   map[string]any{"resultType": "vector", "result": result},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}

func sample(labels map[string]string, value string) map[string]any {
	return map[string]any{"metric": labels, "value": []any{1787933018.510, value}}
}

func liveReader(t *testing.T, answers map[string]string) (*Reader, *promServer) {
	t.Helper()
	server := &promServer{answers: answers}
	apiserver := httptest.NewServer(http.HandlerFunc(server.handle))
	t.Cleanup(apiserver.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	client := prom.NewClient(cs, prom.Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"})
	return New(client), server
}

// the queries that actually reach prometheus, over a real client and a real socket

func TestAGraphIsOneQueryAgainstARealPrometheus(t *testing.T) {
	answers := map[string]string{
		cilium.flows: vector(
			t,
			sample(map[string]string{
				"source_namespace":      "apps",
				"source_workload":       "web",
				"destination_namespace": "data",
				"destination_workload":  "postgres",
				"verdict":               forwarded,
			}, "12.5"),
		),
	}
	reader, server := liveReader(t, answers)

	graph := reader.Graph(t.Context(), time.Unix(1787933018, 0))

	if graph.Error != "" {
		t.Fatalf("graph reported %q", graph.Error)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edges = %d, want 1: %+v", len(graph.Edges), graph.Edges)
	}
	if graph.Edges[0].From != "apps/web" || graph.Edges[0].To != "data/postgres" {
		t.Fatalf("edge = %+v", graph.Edges[0])
	}
	if graph.Edges[0].Rate != 12.5 {
		t.Fatalf("rate = %v, want 12.5", graph.Edges[0].Rate)
	}
	if len(server.asked) != 1 {
		t.Fatalf("prometheus was asked %d queries, want 1: %v", len(server.asked), server.asked)
	}
	if server.asked[0] != cilium.flows {
		t.Fatalf("query = %q, want the flow query", server.asked[0])
	}
}

func TestAnUnlabeledMeshCostsThreeQueriesInThisOrder(t *testing.T) {
	answers := map[string]string{
		cilium.flows:   vector(t, sample(map[string]string{"verdict": forwarded}, "138")),
		cilium.present: vector(t, sample(map[string]string{}, "12")),
	}
	reader, server := liveReader(t, answers)

	support := reader.Support(t.Context(), time.Unix(1787933018, 0))

	if support.Available {
		t.Fatalf("unlabeled metrics were offered: %+v", support)
	}
	if support.Reason != cilium.hint {
		t.Fatalf("reason = %q, want the configuration hint", support.Reason)
	}
	want := []string{cilium.flows, cilium.labeled, cilium.present}
	if len(server.asked) != len(want) {
		t.Fatalf("queries = %v, want %v", server.asked, want)
	}
	for i, query := range want {
		if server.asked[i] != query {
			t.Fatalf("query %d = %q, want %q", i, server.asked[i], query)
		}
	}
}

func TestPrometheusRefusingAQuerySurfacesItsMessage(t *testing.T) {
	answers := map[string]string{
		cilium.flows: `{"status":"error","errorType":"bad_data","error":"parse error at char 5"}`,
	}
	reader, _ := liveReader(t, answers)

	graph := reader.Graph(t.Context(), time.Unix(1787933018, 0))

	if !strings.Contains(graph.Error, "parse error at char 5") {
		t.Fatalf("error = %q, want prometheus's own message", graph.Error)
	}
	if graph.Nodes == nil || graph.Edges == nil {
		t.Fatalf("a refused read sent null instead of an empty list: %+v", graph)
	}
}

func TestAnApiserverThatWillNotProxyIsReported(t *testing.T) {
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","reason":"Forbidden","code":403}`))
	}))
	t.Cleanup(apiserver.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	client := prom.NewClient(cs, prom.Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"})

	support := New(client).Support(t.Context(), time.Unix(1787933018, 0))

	if support.Available {
		t.Fatal("a forbidden proxy was reported as available")
	}
	if !strings.Contains(support.Reason, "may not proxy services") {
		t.Fatalf("reason = %q, want the proxy refusal", support.Reason)
	}
}
