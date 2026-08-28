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
	"github.com/sophotechlabs/spinoza/internal/helm"
)

type stubViews struct {
	Backend

	overview   api.ClusterOverview
	issues     api.IssueQueue
	releases   api.HelmReleases
	detail     api.HelmReleaseDetail
	support    api.HelmSupport
	helmErr    error
	detailErr  error
	actionErr  error
	action     api.HelmActionResult
	rollbacks  []int64
	uninstalls []string
	calls      int

	versions      api.HelmChartVersions
	versionsAsked []string
	upgrades      []helm.UpgradeRequest

	search      api.HelmChartSearch
	searchAsked []string
	chartValues api.HelmChartValues
	valuesAsked []helm.ValuesRequest
	installs    []helm.InstallRequest
}

func (s *stubViews) HelmChartSearch(_ context.Context, query string) (api.HelmChartSearch, error) {
	s.searchAsked = append(s.searchAsked, query)
	if s.helmErr != nil {
		return api.HelmChartSearch{}, s.helmErr
	}
	return s.search, nil
}

func (s *stubViews) HelmChartValues(_ context.Context, req helm.ValuesRequest) (api.HelmChartValues, error) {
	s.valuesAsked = append(s.valuesAsked, req)
	if s.helmErr != nil {
		return api.HelmChartValues{}, s.helmErr
	}
	return s.chartValues, nil
}

func (s *stubViews) HelmInstall(_ context.Context, req helm.InstallRequest) (api.HelmActionResult, error) {
	s.installs = append(s.installs, req)
	if s.actionErr != nil {
		return api.HelmActionResult{}, s.actionErr
	}
	return s.action, nil
}

func (s *stubViews) HelmRelease(_ context.Context, namespace, name string) (api.HelmReleaseDetail, error) {
	s.calls++
	if s.detailErr != nil {
		return api.HelmReleaseDetail{}, s.detailErr
	}
	s.detail.Release.Namespace = namespace
	s.detail.Release.Name = name
	return s.detail, nil
}

func (s *stubViews) HelmSupport() api.HelmSupport {
	return s.support
}

func (s *stubViews) HelmRollback(_ context.Context, _, _ string, revision int64) (api.HelmActionResult, error) {
	s.rollbacks = append(s.rollbacks, revision)
	if s.actionErr != nil {
		return api.HelmActionResult{}, s.actionErr
	}
	return s.action, nil
}

func (s *stubViews) HelmUninstall(_ context.Context, _, name string) (api.HelmActionResult, error) {
	s.uninstalls = append(s.uninstalls, name)
	if s.actionErr != nil {
		return api.HelmActionResult{}, s.actionErr
	}
	return s.action, nil
}

func (s *stubViews) HelmUpgrade(_ context.Context, req helm.UpgradeRequest) (api.HelmActionResult, error) {
	s.upgrades = append(s.upgrades, req)
	if s.actionErr != nil {
		return api.HelmActionResult{}, s.actionErr
	}
	return s.action, nil
}

func (s *stubViews) HelmVersions(_ context.Context, chart string) (api.HelmChartVersions, error) {
	s.versionsAsked = append(s.versionsAsked, chart)
	if s.helmErr != nil {
		return api.HelmChartVersions{}, s.helmErr
	}
	return s.versions, nil
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

func (s *stubViews) Issues(_ context.Context) api.IssueQueue {
	return s.issues
}

func TestTheIssuesEndpointServesTheQueueTheBackendBuilt(t *testing.T) {
	backend := &stubViews{issues: api.IssueQueue{
		Rows: []api.Issue{{
			ID:       "pod-startup/uid-web",
			Severity: api.SeverityFatal,
			Detector: "pod-startup",
			Title:    "CrashLoopBackOff",
			Object:   api.ObjectRef{Version: "v1", Resource: "deployments", Namespace: "web", Name: "api"},
			Kind:     "Deployment",
			Folded:   200,
		}},
		Dropped: 3,
	}}
	ts := stubbedServer(t, backend)

	var got api.IssueQueue
	resp := getJSON(t, ts.URL+"/api/issues", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].Title != "CrashLoopBackOff" || got.Rows[0].Folded != 200 {
		t.Fatalf("row = %+v, want the folded crashloop", got.Rows[0])
	}
	if got.Dropped != 3 {
		t.Fatalf("dropped = %d, want 3", got.Dropped)
	}
}

func TestTheIssuesEndpointPassesAPartialAnswerThrough(t *testing.T) {
	backend := &stubViews{issues: api.IssueQueue{Rows: []api.Issue{}, Error: "pods is forbidden"}}
	ts := stubbedServer(t, backend)

	var got api.IssueQueue
	resp := getJSON(t, ts.URL+"/api/issues", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, a partial queue is still an answer", resp.StatusCode)
	}
	if got.Error != "pods is forbidden" {
		t.Fatalf("error = %q, want the failure carried through", got.Error)
	}
}

func TestTheOverviewEndpointPassesAPartialAnswerThrough(t *testing.T) {
	backend := &stubViews{overview: api.ClusterOverview{Error: "nodes is forbidden"}}
	ts := stubbedServer(t, backend)

	var got api.ClusterOverview
	resp := getJSON(t, ts.URL+"/api/overview", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, a partial overview is still an answer", resp.StatusCode)
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

	for _, path := range []string{"/api/overview", "/api/issues", "/api/helm"} {
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

func post(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		t.Fatalf("POST %s: %v", url, doErr)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestTheReleaseEndpointServesTheDetail(t *testing.T) {
	backend := &stubViews{detail: api.HelmReleaseDetail{
		Driver:   "secret",
		Values:   "replicaCount: 2\n",
		Notes:    "thanks",
		Manifest: "kind: ConfigMap\n",
		Resources: []api.HelmResource{
			{APIVersion: "v1", Kind: "ConfigMap", Name: "cm", Resource: "configmaps", Version: "v1"},
		},
		History: []api.HelmRevision{{Revision: 2, Status: "deployed"}},
	}}
	ts := stubbedServer(t, backend)

	var got api.HelmReleaseDetail
	resp := getJSON(t, ts.URL+"/api/helm/release?namespace=demo&name=podinfo", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got.Release.Name != "podinfo" || got.Release.Namespace != "demo" {
		t.Fatalf("release = %+v, want the one asked for", got.Release)
	}
	if got.Values != "replicaCount: 2\n" {
		t.Fatalf("values = %q", got.Values)
	}
	if len(got.Resources) != 1 || got.Resources[0].Resource != "configmaps" {
		t.Fatalf("resources = %+v", got.Resources)
	}
	if len(got.History) != 1 {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestTheReleaseEndpointNeedsBothCoordinates(t *testing.T) {
	ts := stubbedServer(t, &stubViews{})

	for _, query := range []string{"", "?namespace=demo", "?name=podinfo"} {
		resp := getJSON(t, ts.URL+"/api/helm/release"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %q = %d, want 400", query, resp.StatusCode)
		}
	}
}

func TestTheReleaseEndpointReportsAMissingRelease(t *testing.T) {
	backend := &stubViews{detailErr: fmt.Errorf("%w: demo/ghost", helm.ErrNoRelease)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/release?namespace=demo&name=ghost", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a missing release answered 200")
	}
}

func TestTheSupportEndpointSaysWhetherHelmIsThere(t *testing.T) {
	backend := &stubViews{support: api.HelmSupport{Available: false, Reason: "helm was not found on PATH", Binary: "helm"}}
	ts := stubbedServer(t, backend)

	var got api.HelmSupport
	resp := getJSON(t, ts.URL+"/api/helm/support", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got.Available {
		t.Fatal("support said helm is available")
	}
	if got.Reason == "" {
		t.Fatal("support gave no reason")
	}
}

func TestTheActionEndpointRollsBackToTheGivenRevision(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "rollback", Message: "done", Revision: 2}}
	ts := stubbedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=rollback&revision=2")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(backend.rollbacks) != 1 || backend.rollbacks[0] != 2 {
		t.Fatalf("rollbacks = %v, want revision 2", backend.rollbacks)
	}
}

func TestTheActionEndpointUninstalls(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "uninstall", Message: "gone"}}
	ts := stubbedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=uninstall")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(backend.uninstalls) != 1 || backend.uninstalls[0] != "podinfo" {
		t.Fatalf("uninstalls = %v", backend.uninstalls)
	}
}

func TestTheActionEndpointRefusesAnythingElse(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	cases := []string{
		"?namespace=demo&name=podinfo&action=install",
		"?namespace=demo&name=podinfo&action=rollback",
		"?namespace=demo&name=podinfo&action=rollback&revision=latest",
		"?name=podinfo&action=uninstall",
	}
	for _, query := range cases {
		resp := post(t, ts.URL+"/api/helm/action"+query)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %q = %d, want 400", query, resp.StatusCode)
		}
	}
	if len(backend.rollbacks) != 0 || len(backend.uninstalls) != 0 {
		t.Fatal("a refused request still reached the backend")
	}
}

func TestTheActionEndpointReportsAFailedRun(t *testing.T) {
	backend := &stubViews{actionErr: errors.New("release: not found")}
	ts := stubbedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=uninstall")

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a failed action answered 200")
	}
}
