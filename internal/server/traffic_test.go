package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type stubTrafficProxy struct {
	answers map[string]string
}

func (s *stubTrafficProxy) Get(_ context.Context, _ prom.Target, path string, params map[string]string) ([]byte, error) {
	if path != "api/v1/query" {
		return []byte(`{"status":"success"}`), nil
	}
	body, found := s.answers[params["query"]]
	if !found {
		return []byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`), nil
	}
	return []byte(body), nil
}

func trafficServer(t *testing.T, answers map[string]string) *httptest.Server {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := prom.NewClientWithProxy(
		k8sfake.NewClientset(),
		&stubTrafficProxy{answers: answers},
		prom.Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"},
	)
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:    dyn,
		Clientset:  k8sfake.NewClientset(),
		Prometheus: client,
	})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func flowsQuery(t *testing.T) string {
	t.Helper()
	return `sum by (source_namespace, source_workload, destination_namespace, destination_workload, verdict) ` +
		`(rate(hubble_flows_processed_total[5m]))`
}

func decodeInto(t *testing.T, base, path string, into any) *http.Response {
	t.Helper()
	res, err := http.Get(base + path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return res
}

func TestTrafficEndpointsWithoutPrometheus(t *testing.T) {
	ts := dashboardServer(t)

	var found api.Capabilities
	res := decodeInto(t, ts.URL, "/api/capabilities", &found)
	support := found.Traffic
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", res.Header.Get("Content-Type"))
	}
	if support.Available {
		t.Fatal("traffic was offered on a cluster with no prometheus")
	}
	if support.Reason == "" {
		t.Fatal("no reason was given")
	}

	var graph api.TrafficGraph
	decodeInto(t, ts.URL, "/api/traffic", &graph)
	if graph.Error == "" {
		t.Fatal("the graph came back without an error")
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("edges = %d, want none", len(graph.Edges))
	}
}

func TestTrafficEndpointsServeTheGraph(t *testing.T) {
	flows := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"source_namespace":"apps","source_workload":"web","destination_namespace":"apps",
		"destination_workload":"api","verdict":"FORWARDED"},"value":[1787933018.510,"9"]}
	]}}`
	ts := trafficServer(t, map[string]string{flowsQuery(t): flows})

	var found api.Capabilities
	decodeInto(t, ts.URL, "/api/capabilities", &found)
	support := found.Traffic
	if !support.Available {
		t.Fatalf("traffic was refused: %q", support.Reason)
	}
	if support.Source != "Cilium Hubble" {
		t.Fatalf("source = %q, want Cilium Hubble", support.Source)
	}

	var graph api.TrafficGraph
	decodeInto(t, ts.URL, "/api/traffic", &graph)
	if len(graph.Edges) != 1 {
		t.Fatalf("edges = %d, want 1: %+v", len(graph.Edges), graph.Edges)
	}
	if graph.Edges[0].From != "apps/web" || graph.Edges[0].To != "apps/api" {
		t.Fatalf("edge = %+v", graph.Edges[0])
	}
	if graph.Edges[0].Rate != 9 {
		t.Fatalf("rate = %v, want 9", graph.Edges[0].Rate)
	}
}

func TestTrafficSupportNamesTheMissingLabels(t *testing.T) {
	unlabelled := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"verdict":"FORWARDED"},"value":[1787933018.510,"138"]}
	]}}`
	present := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{},"value":[1787933018.510,"12"]}
	]}}`
	ts := trafficServer(t, map[string]string{
		flowsQuery(t):                         unlabelled,
		`count(hubble_flows_processed_total)`: present,
	})

	var found api.Capabilities
	decodeInto(t, ts.URL, "/api/capabilities", &found)
	support := found.Traffic
	if support.Available {
		t.Fatal("unlabelled metrics were offered as a graph")
	}
	if !strings.Contains(support.Reason, "labelsContext") {
		t.Fatalf("reason = %q, want the cilium configuration line", support.Reason)
	}
}

func TestTheTrafficEndpointsRefuseAWrongMethod(t *testing.T) {
	ts := dashboardServer(t)

	for _, path := range []string{"/api/traffic", "/api/capabilities"} {
		t.Run(path, func(t *testing.T) {
			res, err := http.Post(ts.URL+path, "application/json", http.NoBody)
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			t.Cleanup(func() { _ = res.Body.Close() })
			if res.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", res.StatusCode)
			}
		})
	}
}

func TestTheTrafficGraphFoldsPastTheBudget(t *testing.T) {
	samples := make([]string, 0, 402)
	for i := range 402 {
		samples = append(samples, fmt.Sprintf(
			`{"metric":{"source_namespace":"team-%d","source_workload":"web-%d",`+
				`"destination_namespace":"data","destination_workload":"postgres","verdict":"FORWARDED"},`+
				`"value":[1787933018.510,"1"]}`, i%2, i,
		))
	}
	flows := `{"status":"success","data":{"resultType":"vector","result":[` + strings.Join(samples, ",") + `]}}`
	ts := trafficServer(t, map[string]string{flowsQuery(t): flows})

	var graph api.TrafficGraph
	decodeInto(t, ts.URL, "/api/traffic", &graph)

	if !graph.Folded {
		t.Fatalf("nodes = %d, want the graph folded to namespaces", len(graph.Nodes))
	}
	if graph.Workloads != 403 {
		t.Fatalf("workloads = %d, want the count before folding", graph.Workloads)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("districts = %d, want team-0, team-1 and data: %+v", len(graph.Nodes), graph.Nodes)
	}
}
