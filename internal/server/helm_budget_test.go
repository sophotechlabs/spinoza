package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func requireBusyResponse(t *testing.T, recorded *httptest.ResponseRecorder, message string) {
	t.Helper()
	if recorded.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusTooManyRequests)
	}
	var failure api.Failure
	if err := json.Unmarshal(recorded.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if failure.Message != message {
		t.Fatalf("error = %q, want %q", failure.Message, message)
	}
}

func TestHelmValuesReadsFailPromptlyAtTheIdentityBudget(t *testing.T) {
	backend := &stubViews{}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	release, ok := srv.helmProcesses.claim("local", 1)
	if !ok {
		t.Fatal("could not reserve the values budget")
	}
	path := "/api/helm/values?chart=podinfo&repo=https://charts.example.com&version=6.10.0"

	recorded := httptest.NewRecorder()
	srv.handleHelmChartValues(recorded, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	requireBusyResponse(t, recorded, "helm values reads are busy; try again")
	if len(backend.valuesAsked) != 0 {
		t.Fatal("a saturated values request reached the backend")
	}

	release()
	recorded = httptest.NewRecorder()
	srv.handleHelmChartValues(recorded, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	if recorded.Code != http.StatusOK {
		t.Fatalf("status after release = %d, want %d", recorded.Code, http.StatusOK)
	}
	if len(backend.valuesAsked) != 1 {
		t.Fatalf("backend calls after release = %d, want 1", len(backend.valuesAsked))
	}
}

func TestHelmChartLookupsFailPromptlyAtTheIdentityBudget(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(*Server, http.ResponseWriter, *http.Request)
	}{
		{name: "search", path: "/api/helm/charts?query=podinfo", call: (*Server).handleHelmCharts},
		{name: "versions", path: "/api/helm/versions?chart=podinfo", call: (*Server).handleHelmVersions},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &stubViews{}
			srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
			release, ok := srv.chartFetches.claim("local", 2)
			if !ok {
				t.Fatal("could not reserve the chart lookup budget")
			}
			defer release()
			recorded := httptest.NewRecorder()
			test.call(srv, recorded, httptest.NewRequest(http.MethodGet, test.path, http.NoBody))
			requireBusyResponse(t, recorded, "helm chart searches are busy; try again")
			if len(backend.searchAsked) != 0 || len(backend.versionsAsked) != 0 {
				t.Fatal("a saturated chart lookup reached the backend")
			}
		})
	}
}

func TestHelmReleaseReadsFailPromptlyAtTheIdentityBudget(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(*Server, http.ResponseWriter, *http.Request)
	}{
		{name: "list", path: "/api/helm", call: (*Server).handleHelm},
		{name: "detail", path: "/api/helm/release?namespace=demo&name=podinfo", call: (*Server).handleHelmRelease},
		{name: "history", path: "/api/helm/history?namespace=demo&name=podinfo", call: (*Server).handleHelmHistory},
		{name: "fleet", path: "/api/helm/fleet", call: (*Server).fleetHelm},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &stubViews{}
			srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
			release, ok := srv.releaseReads.claim("local", 1)
			if !ok {
				t.Fatal("could not reserve the release read budget")
			}
			defer release()
			recorded := httptest.NewRecorder()
			test.call(srv, recorded, httptest.NewRequest(http.MethodGet, test.path, http.NoBody))
			requireBusyResponse(t, recorded, "helm release reads are busy; try again")
			if backend.calls != 0 || len(backend.revisions) != 0 || len(backend.through) != 0 {
				t.Fatal("a saturated release read reached the backend")
			}
		})
	}
}
