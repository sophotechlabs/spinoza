package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

type listing struct {
	notStubbed

	hits      api.SearchResults
	releases  api.HelmReleases
	relErr    error
	report    api.CheckReport
	page      api.CheckPage
	pageErr   error
	pageID    string
	pageAfter string
	pageKeep  checks.Filter
	pageCalls int
	flux      api.FluxDashboard
	argo      api.ArgoDashboard
}

func (l *listing) Search(context.Context, string) api.SearchResults {
	return l.hits
}

func (l *listing) HelmReleases(context.Context) (api.HelmReleases, error) {
	return l.releases, l.relErr
}

func (l *listing) Checks(context.Context, checks.Filter) api.CheckReport {
	return l.report
}

func (l *listing) CheckPage(_ context.Context, id, after string, keep checks.Filter) (api.CheckPage, error) {
	l.pageCalls++
	l.pageID = id
	l.pageAfter = after
	l.pageKeep = keep
	return l.page, l.pageErr
}

func (l *listing) Flux(context.Context) api.FluxDashboard {
	return l.flux
}

func (l *listing) Argo(context.Context) api.ArgoDashboard {
	return l.argo
}

func listServer(t *testing.T, first, second Backend) *httptest.Server {
	t.Helper()
	held := &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active:   mk1,
		backends: map[string]Backend{mk1: first, mk2: second},
	}
	srv := New(held, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func hit(name string) api.SearchHit {
	return api.SearchHit{Version: "v1", Resource: "pods", Kind: "Pod", Namespace: "web", Name: name}
}

func TestSearchReachesEveryOpenCluster(t *testing.T) {
	ts := listServer(t,
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("api")}}},
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("worker")}}})

	var got api.SearchResults
	readFleet(t, ts, "/api/search/fleet?q=a", &got)

	if len(got.Hits) != 2 {
		t.Fatalf("hits = %d, want both clusters", len(got.Hits))
	}
}

func TestEverySearchHitSaysWhichClusterItIsOn(t *testing.T) {
	ts := listServer(t,
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("api")}}},
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("worker")}}})

	var got api.SearchResults
	readFleet(t, ts, "/api/search/fleet?q=a", &got)

	for _, one := range got.Hits {
		if one.Cluster == "" {
			t.Fatalf("a hit came back with no cluster: %+v", one)
		}
	}
}

func TestASearchThatFailedOnOneClusterNamesIt(t *testing.T) {
	ts := listServer(t,
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("api")}}},
		&listing{hits: api.SearchResults{Errors: map[string]string{"pods": "forbidden"}}})

	var got api.SearchResults
	readFleet(t, ts, "/api/search/fleet?q=a", &got)

	if len(got.Errors) != 1 {
		t.Fatalf("errors = %+v", got.Errors)
	}
	for where := range got.Errors {
		if where != "p-mk2: pods" {
			t.Fatalf("the failure was filed under %q", where)
		}
	}
}

func TestASearchNamesAnOpenClusterWithoutABackend(t *testing.T) {
	ts := listServer(
		t,
		&listing{hits: api.SearchResults{Hits: []api.SearchHit{hit("api")}}},
		nil,
	)

	var got api.SearchResults
	readFleet(t, ts, "/api/search/fleet?q=a", &got)

	if got.Errors["p-mk2"] != "cluster is unavailable" {
		t.Fatalf("errors = %+v, want the unavailable cluster named", got.Errors)
	}
}

func TestFleetSearchCapsCombinedResultsAfterStableSorting(t *testing.T) {
	answers := make([]clusterAnswer[api.SearchResults], 0, 3)
	for _, cluster := range []string{"mk3", "mk1", "mk2"} {
		hits := make([]api.SearchHit, 0, 100)
		for index := range 100 {
			hits = append(hits, hit(fmt.Sprintf("workload-%03d", index)))
		}
		answers = append(answers, clusterAnswer[api.SearchResults]{
			cluster: cluster,
			context: cluster,
			answer:  api.SearchResults{Hits: hits},
		})
	}

	got := mergeSearch(answers)

	if len(got.Hits) != fleetSearchCap {
		t.Fatalf("hits = %d, want %d", len(got.Hits), fleetSearchCap)
	}
	if !got.Truncated {
		t.Fatal("the combined fleet result did not say it was truncated")
	}
	last := got.Hits[len(got.Hits)-1]
	if last.Name != "workload-066" || last.Cluster != "mk2" {
		t.Fatalf("last hit = %+v, want the deterministic cap boundary", last)
	}
}

func TestFleetSearchIncludesAClusterPanicAlongsideResourceErrors(t *testing.T) {
	got := mergeSearch([]clusterAnswer[api.SearchResults]{
		{
			cluster: "mk1",
			context: "p-mk1",
			answer:  api.SearchResults{Errors: map[string]string{"pods": "forbidden"}},
		},
		{cluster: "mk2", context: "p-mk2", failure: "panicked: informer cache changed"},
	})

	if got.Errors["p-mk1: pods"] != "forbidden" {
		t.Fatalf("errors = %v, want the resource refusal", got.Errors)
	}
	if got.Errors["p-mk2"] != "panicked: informer cache changed" {
		t.Fatalf("errors = %v, want the panicking cluster", got.Errors)
	}
}

func TestARecoveredClusterPanicDoesNotExposeItsPayload(t *testing.T) {
	payload := strings.Repeat("registry-token", 10_000)
	got := recovered("asking p-mk2", payload)

	if got != fleetReadFailure {
		t.Fatalf("failure = %q, want the bounded generic message", got)
	}
	if strings.Contains(got, "registry-token") {
		t.Fatal("the panic payload was exposed to the fleet response")
	}
}

func release(chart, version string) api.HelmRelease {
	return api.HelmRelease{Name: chart, Namespace: "default", Chart: chart, ChartVersion: version}
}

func TestTheFleetReleaseListHoldsEveryCluster(t *testing.T) {
	ts := listServer(t,
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.1.0")}}},
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.2.0")}}})

	var got api.HelmReleases
	readFleet(t, ts, "/api/helm/fleet", &got)

	if len(got.Releases) != 2 {
		t.Fatalf("releases = %d", len(got.Releases))
	}
}

func TestTheSameChartAtTwoVersionsIsMarkedAsSkew(t *testing.T) {
	ts := listServer(t,
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.1.0")}}},
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.2.0")}}})

	var got api.HelmReleases
	readFleet(t, ts, "/api/helm/fleet", &got)

	for _, one := range got.Releases {
		if one.Skew != "6.1.0 · 6.2.0" {
			t.Fatalf("skew = %q on %s", one.Skew, one.Cluster)
		}
	}
}

func TestAChartAtOneVersionEverywhereIsNotSkew(t *testing.T) {
	ts := listServer(t,
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.1.0")}}},
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.1.0")}}})

	var got api.HelmReleases
	readFleet(t, ts, "/api/helm/fleet", &got)

	for _, one := range got.Releases {
		if one.Skew != "" {
			t.Fatalf("skew = %q where every cluster agrees", one.Skew)
		}
	}
}

func TestAClusterWhoseReleasesCouldNotBeReadIsNamed(t *testing.T) {
	ts := listServer(t,
		&listing{releases: api.HelmReleases{Releases: []api.HelmRelease{release("loki", "6.1.0")}}},
		&listing{relErr: errors.New("helm is not installed")})

	var got api.HelmReleases
	readFleet(t, ts, "/api/helm/fleet", &got)

	if got.Error != "p-mk2: helm is not installed" {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestFleetReleasesHaveStableNestedOrderingAndErrors(t *testing.T) {
	got := mergeReleases([]clusterAnswer[api.HelmReleases]{
		{
			cluster: "mk2",
			context: "p-mk2",
			answer: api.HelmReleases{
				Releases: []api.HelmRelease{
					{Name: "worker", Namespace: "shop", Chart: "zinc", ChartVersion: "1.0.0"},
					{Name: "web", Namespace: "shop", Chart: "loki", ChartVersion: "6.1.0"},
				},
				Error: "helm timed out",
			},
		},
		{
			cluster: "mk1",
			context: "p-mk1",
			answer: api.HelmReleases{Releases: []api.HelmRelease{
				{Name: "api", Namespace: "apps", Chart: "loki", ChartVersion: "6.1.0"},
				{Name: "web", Namespace: "shop", Chart: "loki", ChartVersion: "6.1.0"},
			}},
			failure: "panicked: cache changed",
		},
	})

	want := []string{"loki/mk1/apps/api", "loki/mk1/shop/web", "loki/mk2/shop/web", "zinc/mk2/shop/worker"}
	if len(got.Releases) != len(want) {
		t.Fatalf("releases = %+v", got.Releases)
	}
	for index, one := range got.Releases {
		key := one.Chart + "/" + one.Cluster + "/" + one.Namespace + "/" + one.Name
		if key != want[index] {
			t.Fatalf("release %d = %q, want %q", index, key, want[index])
		}
	}
	wantError := "p-mk1: panicked: cache changed · p-mk2: helm timed out"
	if got.Error != wantError {
		t.Fatalf("error = %q, want %q", got.Error, wantError)
	}
}

func readFleet(t *testing.T, ts *httptest.Server, path string, into any) {
	t.Helper()
	resp, body := doRequest(t, http.MethodGet, ts.URL+path, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
}

func group(id string, refs ...int) api.CheckGroup {
	findings := make([]api.CheckFinding, 0, len(refs))
	for _, ref := range refs {
		findings = append(findings, api.CheckFinding{Ref: ref, Detail: "no limits set"})
	}
	return api.CheckGroup{ID: id, Title: id, Severity: "high", Total: len(refs), Findings: findings}
}

func object(name string) api.CheckObject {
	return api.CheckObject{Version: "v1", Resource: "pods", Namespace: "web", Name: name, Kind: "Pod"}
}

func reportOf(name string, ids ...string) api.CheckReport {
	held := api.CheckReport{Objects: []api.CheckObject{object(name)}, Scanned: 1}
	for _, id := range ids {
		held.Groups = append(held.Groups, group(id, 0))
	}
	return held
}

func TestOneRuleAcrossTheFleetIsOneRow(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportOf("api", "limits-missing")},
		&listing{report: reportOf("worker", "limits-missing")})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if len(got.Groups) != 1 {
		t.Fatalf("groups = %d, want the rule folded into one", len(got.Groups))
	}
	if got.Groups[0].Total != 2 {
		t.Fatalf("total = %d, want both clusters counted", got.Groups[0].Total)
	}
}

func TestAFleetFindingStillPointsAtItsOwnObject(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportOf("api", "limits-missing")},
		&listing{report: reportOf("worker", "limits-missing")})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	found := got.Groups[0].Findings
	if len(found) != 2 {
		t.Fatalf("findings = %d", len(found))
	}
	names := []string{got.Objects[found[0].Ref].Name, got.Objects[found[1].Ref].Name}
	if names[0] == names[1] {
		t.Fatalf("both findings point at %q, so the refs did not move", names[0])
	}
}

func TestEveryCheckedObjectSaysWhichClusterItIsOn(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportOf("api", "limits-missing")},
		&listing{report: reportOf("worker", "limits-missing")})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	for _, one := range got.Objects {
		if one.Cluster == "" {
			t.Fatalf("an object came back with no cluster: %+v", one)
		}
	}
}

func TestARuleOnlyOneClusterReportedStillLands(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportOf("api", "limits-missing")},
		&listing{report: reportOf("worker", "limits-missing", "runs-as-root")})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if len(got.Groups) != 2 {
		t.Fatalf("groups = %d, want the rule nobody else reported kept", len(got.Groups))
	}
}

func TestWhatEachClusterScannedIsCountedTogether(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportOf("api", "limits-missing")},
		&listing{report: reportOf("worker", "limits-missing")})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Scanned != 2 {
		t.Fatalf("scanned = %d", got.Scanned)
	}
}

func TestTheFleetNamespaceBreakdownNamesItsClusters(t *testing.T) {
	shared := []api.NamespaceCount{{Namespace: "shop", Total: 3, High: 1, Medium: 1, Low: 1}}
	only := []api.NamespaceCount{{Namespace: "flux-system", Total: 2, High: 2}}
	got := mergeReports([]clusterAnswer[api.CheckReport]{
		{context: "p-mk1", answer: api.CheckReport{Namespaces: append(slices.Clone(shared), only...)}},
		{context: "p-mk2", answer: api.CheckReport{Namespaces: shared}},
	})

	rows := map[string]api.NamespaceCount{}
	for _, one := range got.Namespaces {
		rows[one.Namespace] = one
	}
	shop, held := rows["shop"]
	if !held {
		t.Fatalf("the merged report carries no shop row: %+v", got.Namespaces)
	}
	if shop.Total != 6 || shop.High != 2 || shop.Medium != 2 || shop.Low != 2 {
		t.Fatalf("shop = %+v, want both clusters added in every column", shop)
	}
	want := []string{"p-mk1", "p-mk2"}
	if !slices.Equal(shop.Clusters, want) {
		t.Fatalf("shop clusters = %v, want %v", shop.Clusters, want)
	}
	if flux := rows["flux-system"]; !slices.Equal(flux.Clusters, []string{"p-mk1"}) {
		t.Fatalf("flux-system clusters = %v, want only p-mk1", flux.Clusters)
	}
}

func TestAClusterThatCouldNotBeCheckedIsNamed(t *testing.T) {
	failed := reportOf("worker", "limits-missing")
	failed.Error = "3 of 8 resource types could not be listed"
	ts := listServer(t, &listing{report: reportOf("api", "limits-missing")}, &listing{report: failed})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Error != "p-mk2: 3 of 8 resource types could not be listed" {
		t.Fatalf("error = %q", got.Error)
	}
}

func fluxOne(name string) api.FluxDashboard {
	return api.FluxDashboard{Groups: []api.FluxGroup{{
		Name: "Kustomizations",
		Resources: []api.FluxResource{{
			Kind: "Kustomization", Group: "kustomize.toolkit.fluxcd.io", Version: "v1",
			Resource: "kustomizations", Name: name, Namespace: "flux-system", Ready: "True",
		}},
	}}}
}

func argoOne(name string) api.ArgoDashboard {
	return api.ArgoDashboard{Apps: []api.ArgoApp{{
		Kind: "Application", Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
		Name: name, Namespace: "argocd", Sync: "Synced", Health: "Healthy",
	}}}
}

func TestBothEnginesLandInOneFleetList(t *testing.T) {
	ts := listServer(t,
		&listing{flux: fluxOne("platform")},
		&listing{argo: argoOne("root")})

	var got api.FleetGitops
	readFleet(t, ts, "/api/gitops/fleet", &got)

	if len(got.Apps) != 2 {
		t.Fatalf("apps = %d, want one from each engine", len(got.Apps))
	}
	engines := map[string]bool{}
	for _, one := range got.Apps {
		engines[one.Engine] = true
	}
	if !engines[api.EngineFlux] || !engines[api.EngineArgo] {
		t.Fatalf("engines = %+v", engines)
	}
}

func TestAnAppOnTwoClustersSaysHowFarItSpreads(t *testing.T) {
	ts := listServer(t, &listing{flux: fluxOne("platform")}, &listing{flux: fluxOne("platform")})

	var got api.FleetGitops
	readFleet(t, ts, "/api/gitops/fleet", &got)

	for _, one := range got.Apps {
		if one.Spread != 2 {
			t.Fatalf("spread = %d on %s", one.Spread, one.Cluster)
		}
	}
}

func TestAnAppOnOneClusterSaysSo(t *testing.T) {
	ts := listServer(t, &listing{flux: fluxOne("platform")}, &listing{flux: fluxOne("other")})

	var got api.FleetGitops
	readFleet(t, ts, "/api/gitops/fleet", &got)

	for _, one := range got.Apps {
		if one.Spread != 1 {
			t.Fatalf("spread = %d on %s", one.Spread, one.Name)
		}
	}
}

func TestAGitopsEngineThatFailedIsNamed(t *testing.T) {
	ts := listServer(t,
		&listing{flux: fluxOne("platform")},
		&listing{argo: api.ArgoDashboard{Error: "argocd is not installed"}})

	var got api.FleetGitops
	readFleet(t, ts, "/api/gitops/fleet", &got)

	if got.Error != "p-mk2: argocd is not installed" {
		t.Fatalf("error = %q", got.Error)
	}
}
