package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubCatalog struct {
	notStubbed

	catalog   api.ResourceCatalog
	counts    api.ResourceCounts
	refreshed int
}

func (s *stubCatalog) Resources() api.ResourceCatalog {
	return s.catalog
}

func (s *stubCatalog) RefreshResources() api.ResourceCatalog {
	s.refreshed++
	return s.catalog
}

func (s *stubCatalog) Counts(context.Context) api.ResourceCounts {
	return s.counts
}

type stubBackendCluster struct {
	backend   Backend
	protected bool
}

func (s *stubBackendCluster) Manager(string) Backend {
	return s.backend
}

func (s *stubBackendCluster) Contexts() api.ContextList {
	return api.ContextList{
		Current:     api.ContextRef{Name: "p-mk1"},
		Kubeconfigs: []api.Kubeconfig{{Label: "default", Contexts: []api.KubeContext{{Name: "p-mk1", Cluster: "p-mk1"}}}},
	}
}

func (s *stubBackendCluster) Use(api.ContextRef) error {
	return errors.New("this stub has one context")
}

func (s *stubBackendCluster) AddKubeconfig(string) error {
	return errors.New("this stub reads one kubeconfig")
}

func (s *stubBackendCluster) RemoveKubeconfig(string) error {
	return errors.New("this stub reads one kubeconfig")
}

func (s *stubBackendCluster) Protect(string, bool) error {
	return errors.New("this stub protects nothing")
}

func (s *stubBackendCluster) Read(context.Context, api.ContextRef, api.ObjectRef) (string, error) {
	return "", errors.New("this stub reads one context")
}

func (s *stubBackendCluster) List(
	context.Context,
	api.ContextRef,
	api.ObjectRef,
) ([]*unstructured.Unstructured, error) {
	return nil, errors.New("this stub reads one context")
}

func (s *stubBackendCluster) Protected(string) bool {
	return s.protected
}

func stubbedServer(t *testing.T, backend Backend) *httptest.Server {
	t.Helper()
	return clusterServer(t, &stubBackendCluster{backend: backend})
}

func protectedServer(t *testing.T, backend Backend) *httptest.Server {
	t.Helper()
	return clusterServer(t, &stubBackendCluster{backend: backend, protected: true})
}

func clusterServer(t *testing.T, cluster Cluster) *httptest.Server {
	t.Helper()
	srv := New(cluster, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestThePickerStaysReachableWithoutACluster(t *testing.T) {
	ts := stubbedServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the picker served so a dead default context can be escaped: %s", resp.StatusCode, body)
	}
	var list api.ContextList
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(list.Kubeconfigs) == 0 || len(list.Kubeconfigs[0].Contexts) == 0 {
		t.Fatal("the picker offered no contexts")
	}
}

func TestTheUIIsStillServedWithoutACluster(t *testing.T) {
	ts := stubbedServer(t, nil)

	for _, path := range []string{"/", "/healthz", "/api/version"} {
		t.Run(path, func(t *testing.T) {
			resp, body := doRequest(t, http.MethodGet, ts.URL+path, nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, body)
			}
		})
	}
}

func TestClusterWorkSaysThereIsNoClusterYet(t *testing.T) {
	ts := stubbedServer(t, nil)

	for _, path := range []string{"/api/resources", "/api/flux", "/api/metrics", "/api/resources/counts"} {
		t.Run(path, func(t *testing.T) {
			resp, body := doRequest(t, http.MethodGet, ts.URL+path, nil)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 rather than a panic: %s", resp.StatusCode, body)
			}
		})
	}
}

func TestTheCatalogEndpointNeedsNoClusterAtAll(t *testing.T) {
	backend := &stubCatalog{catalog: api.ResourceCatalog{
		Categories: []api.Category{{Name: "Workloads"}},
		Error:      "discovery came back partial",
	}}
	ts := stubbedServer(t, backend)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/resources", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	var catalog api.ResourceCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(catalog.Categories) != 1 || catalog.Categories[0].Name != "Workloads" {
		t.Fatalf("categories = %v", catalog.Categories)
	}
	if catalog.Error != "discovery came back partial" {
		t.Fatalf("error = %q", catalog.Error)
	}
}

func TestAPostRefreshesTheCatalogThroughTheNarrowInterface(t *testing.T) {
	backend := &stubCatalog{catalog: api.ResourceCatalog{Categories: []api.Category{{Name: "Workloads"}}}}
	ts := stubbedServer(t, backend)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/resources", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if backend.refreshed != 1 {
		t.Fatalf("refreshed %d times, want 1", backend.refreshed)
	}
}

func TestCountsCarryTheirReasonsThroughTheHandler(t *testing.T) {
	backend := &stubCatalog{counts: api.ResourceCounts{
		Counts: map[string]int{"apps/v1/deployments": -1},
		Errors: map[string]string{"apps/v1/deployments": "deployments is forbidden"},
	}}
	ts := stubbedServer(t, backend)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/resources/counts", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	var counts api.ResourceCounts
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if counts.Errors["apps/v1/deployments"] != "deployments is forbidden" {
		t.Fatalf("errors = %v, want the reason carried to the browser", counts.Errors)
	}
}

func (s *stubBackendCluster) ID() string {
	return "https://p-mk1:6443"
}

func (s *stubBackendCluster) Open(api.ContextRef) (string, error) {
	return s.ID(), nil
}

func (s *stubBackendCluster) Activate(string) error {
	return nil
}

func (s *stubBackendCluster) Opened() []api.OpenCluster {
	return []api.OpenCluster{{ID: s.ID(), Active: true, Protection: api.ProtectionUnknown}}
}

func (s *stubBackendCluster) Close(string) error {
	return nil
}
