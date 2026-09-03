package helm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

type controlledValuesRunner struct {
	started  chan struct{}
	proceed  chan struct{}
	once     sync.Once
	calls    atomic.Int32
	deadline time.Time
}

func (r *controlledValuesRunner) Run(ctx context.Context, _, _ []string) (string, error) {
	r.calls.Add(1)
	deadline, ok := ctx.Deadline()
	if ok {
		r.deadline = deadline
	}
	r.once.Do(func() { close(r.started) })
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-r.proceed:
		return "replicaCount: 1\n", nil
	}
}

func (r *controlledValuesRunner) Available() error {
	return nil
}

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
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)

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

func TestChartValuesCacheUsesOneBoundedFetchForConcurrentReaders(t *testing.T) {
	runner := &controlledValuesRunner{started: make(chan struct{}), proceed: make(chan struct{})}
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)
	req := ValuesRequest{Chart: "podinfo", Version: "6.10.0", RepoURL: "https://charts.example.com"}
	startedAt := time.Now()

	const callers = 16
	errs := make(chan error, callers)
	for range callers {
		go func() {
			_, err := svc.ChartValues(t.Context(), req)
			errs <- err
		}()
	}
	<-runner.started
	close(runner.proceed)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("values: %v", err)
		}
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("helm calls = %d, want 1", runner.calls.Load())
	}
	if runner.deadline.Before(startedAt.Add(valuesTimeout - time.Second)) {
		t.Fatalf("helm deadline = %s, want about %s", runner.deadline.Sub(startedAt), valuesTimeout)
	}
	if runner.deadline.After(startedAt.Add(valuesTimeout + time.Second)) {
		t.Fatalf("helm deadline = %s, want at most %s", runner.deadline.Sub(startedAt), valuesTimeout)
	}
}

func TestChartValuesWaiterCanCancelWithoutStartingAnotherFetch(t *testing.T) {
	runner := &controlledValuesRunner{started: make(chan struct{}), proceed: make(chan struct{})}
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{},
	)
	req := ValuesRequest{Chart: "podinfo", Version: "6.10.0", RepoURL: "https://charts.example.com"}
	leader := make(chan error, 1)
	go func() {
		_, err := svc.ChartValues(t.Context(), req)
		leader <- err
	}()
	<-runner.started
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := svc.ChartValues(canceled, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter error = %v, want context cancellation", err)
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("helm calls = %d, want 1", runner.calls.Load())
	}
	close(runner.proceed)
	if err := <-leader; err != nil {
		t.Fatalf("leader values: %v", err)
	}
}

func TestChartValuesCacheExpiresAtTheTTLBoundary(t *testing.T) {
	runner := &stubRunner{out: "{}"}
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{},
	)
	now := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	svc.valuesNow = func() time.Time { return now }
	req := ValuesRequest{Chart: "podinfo", Version: "6.10.0", RepoURL: "https://charts.example.com"}

	if _, err := svc.ChartValues(t.Context(), req); err != nil {
		t.Fatalf("first values: %v", err)
	}
	now = now.Add(valuesTTL - time.Nanosecond)
	if _, err := svc.ChartValues(t.Context(), req); err != nil {
		t.Fatalf("cached values: %v", err)
	}
	if len(runner.args) != 1 {
		t.Fatalf("helm calls inside ttl = %d, want 1", len(runner.args))
	}
	now = now.Add(time.Nanosecond)
	if _, err := svc.ChartValues(t.Context(), req); err != nil {
		t.Fatalf("values at ttl: %v", err)
	}
	if len(runner.args) != 2 {
		t.Fatalf("helm calls at ttl = %d, want 2", len(runner.args))
	}
}

func TestChartValuesCacheHasAFixedCapacity(t *testing.T) {
	runner := &stubRunner{out: "{}"}
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{},
	)
	now := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	svc.valuesNow = func() time.Time { return now }

	for at := range valuesCapacity + 1 {
		req := ValuesRequest{
			Chart:   "podinfo",
			Version: fmt.Sprintf("1.0.%d", at),
			RepoURL: "https://charts.example.com",
		}
		if _, err := svc.ChartValues(t.Context(), req); err != nil {
			t.Fatalf("values %d: %v", at, err)
		}
		now = now.Add(time.Second)
	}
	if len(svc.values) != valuesCapacity {
		t.Fatalf("cache size = %d, want %d", len(svc.values), valuesCapacity)
	}
	oldest := ValuesRequest{Chart: "podinfo", Version: "1.0.0", RepoURL: "https://charts.example.com"}
	if _, ok := svc.values[oldest]; ok {
		t.Fatal("the oldest values entry survived capacity eviction")
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
			svc := NewService(
				k8sfake.NewClientset(),
				nil,
				runner,
				nil,
				actionRepositories(),
				api.ContextRef{Name: "kind-spinoza"},
			)

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
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)

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
	svc := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)

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

func TestARegistryIsAskedForTheNameThatWasTyped(t *testing.T) {
	index := &stubCharts{lists: map[string][]string{
		"oci://ghcr.io/acme/charts|podinfo": {"6.11.0", "6.10.2"},
	}}
	svc := searcher(t, index, []RepoEntry{
		{Name: "acme", Repo: charts.Repo{URL: "oci://ghcr.io/acme/charts", OCI: true}},
	})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 1 {
		t.Fatalf("hits = %+v, want the registry's chart", found.Hits)
	}
	if found.Hits[0].Chart != "podinfo" || found.Hits[0].Version != "6.11.0" {
		t.Fatalf("hit = %+v, want the newest tag", found.Hits[0])
	}
	if found.Hits[0].URL != "oci://ghcr.io/acme/charts" || found.Hits[0].Repo != "acme" {
		t.Fatalf("hit = %+v, want the registry it came from", found.Hits[0])
	}
	if len(index.searched) != 0 {
		t.Fatalf("searched = %v, want no index listing for a registry", index.searched)
	}
}

func TestARegistryIsNotAskedAboutSomethingThatIsNotAChartName(t *testing.T) {
	index := &stubCharts{}
	svc := searcher(t, index, []RepoEntry{
		{Name: "acme", Repo: charts.Repo{URL: "oci://ghcr.io/acme/charts", OCI: true}},
	})

	found, err := svc.SearchCharts(t.Context(), "Pod Info")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %+v", found.Hits)
	}
	if len(index.asked) != 0 {
		t.Fatalf("asked = %v, want the registry left alone", index.asked)
	}
}

func TestASearchReportsARegistryItCouldNotRead(t *testing.T) {
	index := &stubCharts{
		catalog:  map[string][]charts.Chart{"https://one.example.com": {{Name: "podinfo", Version: "6.14.1"}}},
		failures: map[string]error{"oci://ghcr.io/acme/charts|podinfo": errors.New("status 403")},
	}
	svc := searcher(t, index, []RepoEntry{
		{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}},
		{Name: "acme", Repo: charts.Repo{URL: "oci://ghcr.io/acme/charts", OCI: true}},
	})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 1 || found.Hits[0].Repo != "one" {
		t.Fatalf("hits = %+v, want only the index repository", found.Hits)
	}
	if !strings.Contains(found.Error, "acme: status 403") {
		t.Fatalf("error = %q, want the failing registry and reason", found.Error)
	}
}

func TestARegistryWithNoTagsForThatNameAddsNoHit(t *testing.T) {
	index := &stubCharts{lists: map[string][]string{"oci://ghcr.io/acme/charts|podinfo": {}}}
	svc := searcher(t, index, []RepoEntry{
		{Name: "acme", Repo: charts.Repo{URL: "oci://ghcr.io/acme/charts", OCI: true}},
	})

	found, err := svc.SearchCharts(t.Context(), "podinfo")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 0 {
		t.Fatalf("hits = %+v", found.Hits)
	}
}

func TestTheSameChartInTwoRepositoriesIsListedOnceEach(t *testing.T) {
	index := &stubCharts{catalog: map[string][]charts.Chart{
		"https://two.example.com": {{Name: "redis", Version: "20.0.1"}},
		"https://one.example.com": {{Name: "redis", Version: "19.5.0"}},
	}}
	svc := searcher(t, index, []RepoEntry{
		{Name: "zeta", Repo: charts.Repo{URL: "https://two.example.com"}},
		{Name: "alpha", Repo: charts.Repo{URL: "https://one.example.com"}},
	})

	found, err := svc.SearchCharts(t.Context(), "redis")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 2 {
		t.Fatalf("hits = %+v, want one per repository", found.Hits)
	}
	if found.Hits[0].Repo != "alpha" || found.Hits[1].Repo != "zeta" {
		t.Fatalf("hits = %+v, want the same name settled by repository", found.Hits)
	}
}

func TestAnExactNameBeatsOneThatMerelyContainsIt(t *testing.T) {
	index := &stubCharts{catalog: map[string][]charts.Chart{
		"https://one.example.com": {
			{Name: "kube-redis", Version: "1.0.0"},
			{Name: "redis-cluster", Version: "13.0.4"},
			{Name: "redis", Version: "20.0.1"},
		},
	}}
	svc := searcher(t, index, []RepoEntry{{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}}})

	found, err := svc.SearchCharts(t.Context(), "redis")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	order := make([]string, 0, len(found.Hits))
	for _, hit := range found.Hits {
		order = append(order, hit.Chart)
	}
	want := []string{"redis", "redis-cluster", "kube-redis"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestAnEmptyQueryStillListsWhatEachRepositoryHas(t *testing.T) {
	index := &stubCharts{catalog: map[string][]charts.Chart{
		"https://one.example.com": {{Name: "podinfo", Version: "6.14.1"}, {Name: "redis", Version: "20.0.1"}},
	}}
	svc := searcher(t, index, []RepoEntry{{Name: "one", Repo: charts.Repo{URL: "https://one.example.com"}}})

	found, err := svc.SearchCharts(t.Context(), "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found.Hits) != 2 {
		t.Fatalf("hits = %+v, want everything the repository offers", found.Hits)
	}
}
