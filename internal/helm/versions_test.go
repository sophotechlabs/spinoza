package helm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestVersionsAsksEveryRepository(t *testing.T) {
	index := &stubCharts{lists: map[string][]string{
		"https://one.example.com|podinfo": {"6.15.1", "6.14.0"},
		"https://two.example.com|podinfo": {"7.0.0"},
	}}
	repos := []RepoEntry{
		{Name: "one", Repo: entriesOf("https://one.example.com")[0].Repo},
		{Name: "two", Repo: entriesOf("https://two.example.com")[0].Repo},
	}
	service := newService(k8sfake.NewClientset(), index, repos)

	got, err := service.Versions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if len(index.asked) != 2 {
		t.Fatalf("asked %v, want both repositories", index.asked)
	}
	if len(got.Repos) != 2 {
		t.Fatalf("repos = %d, want both listed", len(got.Repos))
	}
	if got.Repos[0].Name != "one" {
		t.Fatalf("name = %q, want the helm alias carried through", got.Repos[0].Name)
	}
	if !slices.Equal(got.Repos[0].Versions, []string{"6.15.1", "6.14.0"}) {
		t.Fatalf("versions = %v", got.Repos[0].Versions)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none", got.Error)
	}
}

func TestVersionsSkipsARepositoryWithNothingForThatChart(t *testing.T) {
	index := &stubCharts{lists: map[string][]string{
		"https://two.example.com|podinfo": {"7.0.0"},
	}}
	service := newService(k8sfake.NewClientset(), index, entriesOf("https://one.example.com", "https://two.example.com"))

	got, err := service.Versions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if len(got.Repos) != 1 {
		t.Fatalf("repos = %d, want only the one that carries the chart", len(got.Repos))
	}
	if got.Repos[0].URL != "https://two.example.com" {
		t.Fatalf("url = %q", got.Repos[0].URL)
	}
}

func TestVersionsReportsAFailingRepositoryAndKeepsGoing(t *testing.T) {
	index := &stubCharts{
		lists: map[string][]string{
			"https://two.example.com|podinfo": {"7.0.0"},
		},
		failures: map[string]error{
			"https://one.example.com|podinfo": errors.New("index fetch failed"),
		},
	}
	repos := []RepoEntry{
		{Name: "broken", Repo: entriesOf("https://one.example.com")[0].Repo},
		{Repo: entriesOf("https://two.example.com")[0].Repo},
	}
	service := newService(k8sfake.NewClientset(), index, repos)

	got, err := service.Versions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if len(got.Repos) != 1 {
		t.Fatalf("repos = %d, want the healthy one kept", len(got.Repos))
	}
	if !strings.Contains(got.Error, "broken") {
		t.Fatalf("error = %q, want the failing repo named by its alias", got.Error)
	}
}

func TestVersionsSaysWhenNoRepositoriesAreConfigured(t *testing.T) {
	service := newService(k8sfake.NewClientset(), &stubCharts{}, nil)

	got, err := service.Versions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if len(got.Repos) != 0 {
		t.Fatalf("repos = %d, want none", len(got.Repos))
	}
	if !strings.Contains(got.Error, "helm repo add") {
		t.Fatalf("error = %q, want it to say how to add a repository", got.Error)
	}
}

func TestVersionsSaysWhenTheIndexIsNotWired(t *testing.T) {
	service := newService(k8sfake.NewClientset(), nil, entriesOf("https://one.example.com"))

	got, err := service.Versions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if got.Error == "" {
		t.Fatal("expected a note about the missing index")
	}
}

func TestVersionsRefusesABadChartName(t *testing.T) {
	index := &stubCharts{}
	service := newService(k8sfake.NewClientset(), index, entriesOf("https://one.example.com"))

	_, err := service.Versions(context.Background(), "--post-renderer")

	if err == nil {
		t.Fatal("a flag-shaped chart name was accepted")
	}
	if len(index.asked) != 0 {
		t.Fatal("the index was asked about a refused chart name")
	}
}
