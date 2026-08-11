package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubViews struct {
	Backend

	overview api.ClusterOverview
	releases api.HelmReleases
	helmErr  error
	calls    int
}

func (s *stubViews) Overview(context.Context) api.ClusterOverview {
	s.calls++
	return s.overview
}

func (s *stubViews) HelmReleases(context.Context) (api.HelmReleases, error) {
	s.calls++
	if s.helmErr != nil {
		return api.HelmReleases{}, s.helmErr
	}
	return s.releases, nil
}

func getJSON(t *testing.T, url string, into any) *http.Response {
	t.Helper()
	req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if reqErr != nil {
		t.Fatalf("request %s: %v", url, reqErr)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode == http.StatusOK && into != nil {
		if decodeErr := json.NewDecoder(resp.Body).Decode(into); decodeErr != nil {
			t.Fatalf("decode %s: %v", url, decodeErr)
		}
	}
	return resp
}

func TestTheOverviewEndpointServesWhatTheBackendBuilt(t *testing.T) {
	backend := &stubViews{overview: api.ClusterOverview{
		Version: "v1.36.1",
		Nodes:   api.NodeSummary{Total: 3, Ready: 3, CPUAllocatableMilli: 12000, UsageKnown: true},
		Pods:    api.PodSummary{Total: 40, Running: 40, Known: true},
		Warnings: []api.OverviewEvent{
			{Namespace: "flux-system", Object: "Pod/web-1", Reason: "BackOff", Count: 2},
		},
	}}
	ts := stubbedServer(t, backend)

	var got api.ClusterOverview
	resp := getJSON(t, ts.URL+"/api/overview", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
	}
	if got.Version != "v1.36.1" {
		t.Fatalf("version = %q, want v1.36.1", got.Version)
	}
	if got.Nodes.Total != 3 {
		t.Fatalf("nodes = %d, want 3", got.Nodes.Total)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("warnings = %d, want 1", len(got.Warnings))
	}
	if got.Warnings[0].Object != "Pod/web-1" {
		t.Fatalf("warning = %q, want Pod/web-1", got.Warnings[0].Object)
	}
}

func TestTheOverviewEndpointPassesAPartialAnswerThrough(t *testing.T) {
	backend := &stubViews{overview: api.ClusterOverview{Error: "nodes is forbidden"}}
	ts := stubbedServer(t, backend)

	var got api.ClusterOverview
	resp := getJSON(t, ts.URL+"/api/overview", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a partial overview is still an answer", resp.StatusCode)
	}
	if got.Error != "nodes is forbidden" {
		t.Fatalf("error = %q, want the failure carried through", got.Error)
	}
}

func TestTheHelmEndpointServesTheReleases(t *testing.T) {
	backend := &stubViews{releases: api.HelmReleases{Releases: []api.HelmRelease{{
		Name:         "podinfo",
		Namespace:    "demo",
		Chart:        "podinfo",
		ChartVersion: "6.9.2",
		Revision:     3,
		Status:       "deployed",
	}}}}
	ts := stubbedServer(t, backend)

	var got api.HelmReleases
	resp := getJSON(t, ts.URL+"/api/helm", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want 1", len(got.Releases))
	}
	if got.Releases[0].ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want 6.9.2", got.Releases[0].ChartVersion)
	}
}

func TestTheHelmEndpointReportsARefusedList(t *testing.T) {
	refused := apierrors.NewForbidden(
		schema.GroupResource{Resource: "secrets"}, "", errors.New("no policy matched"),
	)
	backend := &stubViews{helmErr: refused}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm", nil)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestTheHelmEndpointReportsAnInternalFailureAsFiveHundred(t *testing.T) {
	backend := &stubViews{helmErr: fmt.Errorf("%w: no kubernetes client is wired up", api.ErrInternal)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestBothViewEndpointsRefuseAnythingButGet(t *testing.T) {
	ts := stubbedServer(t, &stubViews{})

	for _, path := range []string{"/api/overview", "/api/helm"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+path, http.NoBody)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp, postErr := http.DefaultClient.Do(req)
		if postErr != nil {
			t.Fatalf("POST %s: %v", path, postErr)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s = %d, want 405", path, resp.StatusCode)
		}
	}
}
