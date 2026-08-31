package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type meshless struct {
	notStubbed

	graph api.TrafficGraph
}

func (m *meshless) TrafficGraph(context.Context) api.TrafficGraph {
	return m.graph
}

func trafficGraphServer(t *testing.T, graph api.TrafficGraph) string {
	t.Helper()
	held := &fleet{
		held:     []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active:   mk1,
		backends: map[string]Backend{mk1: &meshless{graph: graph}},
	}
	return fleetServer(t, held).URL
}

func TestATrafficGraphThatCouldNotBeReadIsNotAnswered200(t *testing.T) {
	at := trafficGraphServer(t, api.TrafficGraph{
		Nodes: []api.TrafficNode{},
		Edges: []api.TrafficEdge{},
		Error: "prometheus is unavailable",
	})

	resp, body := doRequest(t, http.MethodGet, at+"/api/traffic", nil)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want a failure the view can show; body %s", resp.StatusCode, body)
	}
}

func TestATrafficGraphWithRowsAndAWarningIsStillAnAnswer(t *testing.T) {
	at := trafficGraphServer(t, api.TrafficGraph{
		Source: "cilium",
		Nodes:  []api.TrafficNode{{ID: "one"}},
		Edges:  []api.TrafficEdge{},
		Error:  "some samples were dropped",
	})

	resp, body := doRequest(t, http.MethodGet, at+"/api/traffic", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the partial graph; body %s", resp.StatusCode, body)
	}
}
