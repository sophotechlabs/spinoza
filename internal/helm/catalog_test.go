package helm

import (
	"errors"
	"slices"
	"strings"
	"testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

func searcher(t *testing.T, index Charts, repos []RepoEntry) *Service {
	t.Helper()
	return NewService(k8sfake.NewClientset(), nil, &stubRunner{}, index, repos, api.ContextRef{Name: "kind-spinoza"})
}

func TestASearchWithoutRepositoriesSaysHowToAddOne(t *testing.T) {
	svc := searcher(t, &stubCharts{}, nil)

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %+v", found.Hits)
	}
	if !strings.Contains(found.Error, "helm repo add") {
		t.Fatalf("error = %q", found.Error)
	}
}

func TestASearchWithoutAnIndexSaysSo(t *testing.T) {
	svc := searcher(t, nil, []RepoEntry{{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}}})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if !strings.Contains(found.Error, "not wired up") {
		t.Fatalf("error = %q", found.Error)
	}
}

func TestASearchAsksEveryRepositoryAndKeepsWhereEachHitCameFrom(t *testing.T) {
	index := &stubCharts{catalog: map[string][]charts.Chart{
		"https://one.example.com": {{Name: "podinfo", Version: "6.10.0", Description: "a tiny app"}},
		"https://two.example.com": {{Name: "podinfo-extras", Version: "1.2.3"}},
	}}
	svc := searcher(t, index, []RepoEntry{
		{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}},
		{Name: "two", Repo: charts.Repo{URL: "https://two.example.com"}},
	})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 2 {
		t.Fatalf("hits = %+v", found.Hits)
	}
	if found.Hits[0].Chart != "podinfo" || found.Hits[0].Repo != "one" {
		t.Fatalf("first hit = %+v, want the exact name first", found.Hits[0])
	}
	if found.Hits[0].Description != "a tiny app" || found.Hits[0].URL != "https://one.example.com" {
		t.Fatalf("first hit = %+v", found.Hits[0])
	}
	if found.Hits[1].Chart != "podinfo-extras" {
		t.Fatalf("second hit = %+v", found.Hits[1])
	}
	if found.Error != "" {
		t.Fatalf("error = %q", found.Error)
	}
	slices.Sort(index.searched)
	want := []string{"https://one.example.com|podinfo", "https://two.example.com|podinfo"}
	if !slices.Equal(index.searched, want) {
		t.Fatalf("searched = %v, want %v", index.searched, want)
	}
}

func TestASearchReportsTheRepositoriesItCouldNotRead(t *testing.T) {
	index := &stubCharts{
		catalog:  map[string][]charts.Chart{"https://one.example.com": {{Name: "podinfo", Version: "6.10.0"}}},
		failures: map[string]error{"https://two.example.com": errors.New("404")},
	}
	svc := searcher(t, index, []RepoEntry{
		{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}},
		{Repo: charts.Repo{URL: "https://two.example.com"}},
	})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %+v, want the working repository still answered", found.Hits)
	}
	if !strings.Contains(found.Error, "https://two.example.com: 404") {
		t.Fatalf("error = %q, want the failing repository named by its url", found.Error)
	}
}

func TestASearchSaysWhenItHeldSomeBack(t *testing.T) {
	many := make([]charts.Chart, 0, searchLimit+5)
	for i := range searchLimit + 5 {
		many = append(many, charts.Chart{Name: "chart" + string(rune('a'+i%26)) + strings.Repeat("x", i), Version: "1.0.0"})
	}
	index := &stubCharts{catalog: map[string][]charts.Chart{"https://one.example.com": many}}
	svc := searcher(t, index, []RepoEntry{{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}}})

	found, err := svc.SearchCharts(t.Context(), "chart")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != searchLimit {
		t.Fatalf("hits = %d, want the limit", len(found.Hits))
	}
	if !found.Truncated {
		t.Fatal("a capped search did not say so")
	}
}

func TestChartValuesAsksHelmForTheDefaults(t *testing.T) {
	runner := &stubRunner{out: "replicaCount: 1\n"}
	svc := NewService(k8sfake.NewClientset(), nil, runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	found, err := svc.ChartValues(t.Context(), ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "https://charts.example.com",
	})
	if err != nil {
		t.Fatalf("values: %v", err)
	}

	args := runner.args[0]
	if args[0] != "show" || args[1] != "values" || args[2] != "podinfo" {
		t.Fatalf("args = %v", args)
	}
	if !hasPair(args, "--repo", "https://charts.example.com") {
		t.Fatalf("args = %v", args)
	}
	if found.Values != "replicaCount: 1\n" {
		t.Fatalf("values = %q", found.Values)
	}
}

func TestChartValuesRefusesWhatItCannotFetch(t *testing.T) {
	cases := map[string]ValuesRequest{
		"a chart that is not a chart name": {Chart: "../etc", Version: "6.10.0", RepoURL: "https://charts.example.com"},
		"a version that is not semantic":   {Chart: "podinfo", Version: "latest", RepoURL: "https://charts.example.com"},
		"a repository on this machine":     {Chart: "podinfo", Version: "6.10.0", RepoURL: "http://localhost:8080"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &stubRunner{}
			svc := NewService(k8sfake.NewClientset(), nil, runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

			_, err := svc.ChartValues(t.Context(), req)

			if err == nil {
				t.Fatal("the request was accepted")
			}
			if len(runner.args) != 0 {
				t.Fatalf("helm was run anyway: %v", runner.args)
			}
		})
	}
}

func TestChartValuesWithoutARunnerIsRefused(t *testing.T) {
	svc := NewService(k8sfake.NewClientset(), nil, nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := svc.ChartValues(t.Context(), ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "https://charts.example.com",
	})

	if err == nil {
		t.Fatal("chart values without a runner reported success")
	}
}

func TestChartValuesReportsWhatHelmSaid(t *testing.T) {
	runner := &stubRunner{err: errors.New("chart not found")}
	svc := NewService(k8sfake.NewClientset(), nil, runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := svc.ChartValues(t.Context(), ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "https://charts.example.com",
	})

	if err == nil {
		t.Fatal("a failed lookup reported success")
	}
}

func TestChartValuesFromAnOCIRegistryCarriesTheRef(t *testing.T) {
	runner := &stubRunner{out: "{}"}
	svc := NewService(k8sfake.NewClientset(), nil, runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := svc.ChartValues(t.Context(), ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "oci://registry.example.com/charts",
		OCI:     true,
	})
	if err != nil {
		t.Fatalf("values: %v", err)
	}

	args := runner.args[0]
	if args[2] != "oci://registry.example.com/charts/podinfo" {
		t.Fatalf("chart ref = %q", args[2])
	}
	if slices.Contains(args, "--repo") {
		t.Fatalf("args = %v, want no --repo for an oci chart", args)
	}
}
