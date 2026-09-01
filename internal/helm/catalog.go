package helm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

const (
	searchLimit      = 200
	searchTimeout    = 90 * time.Second
	searchConcurrent = 8
)

type repoHits struct {
	entry   RepoEntry
	charts  []charts.Chart
	failure string
}

func (s *Service) SearchCharts(ctx context.Context, query string) (api.HelmChartSearch, error) {
	out := api.HelmChartSearch{Query: query, Hits: []api.HelmChartHit{}}
	if len(s.repos) == 0 {
		out.Error = "no chart repositories are configured; add one with helm repo add"
		return out, nil
	}
	if s.index == nil {
		out.Error = "chart repositories are not wired up"
		return out, nil
	}
	bounded, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	found := s.askEveryRepo(bounded, query)
	failures := []string{}
	for _, result := range found {
		if result.failure != "" {
			failures = append(failures, result.failure)
			continue
		}
		for _, chart := range result.charts {
			out.Hits = append(out.Hits, api.HelmChartHit{
				Chart:       chart.Name,
				Version:     chart.Version,
				Description: chart.Description,
				Repo:        result.entry.Name,
				URL:         result.entry.Repo.URL,
			})
		}
	}
	slices.SortStableFunc(out.Hits, closerFirst(query))
	if len(out.Hits) > searchLimit {
		out.Hits = out.Hits[:searchLimit]
		out.Truncated = true
	}
	out.Error = strings.Join(failures, "; ")
	return out, nil
}

func (s *Service) askEveryRepo(ctx context.Context, query string) []repoHits {
	found := make([]repoHits, len(s.repos))
	tokens := make(chan struct{}, searchConcurrent)
	var wg sync.WaitGroup
	for i, entry := range s.repos {
		wg.Go(func() {
			tokens <- struct{}{}
			defer func() { <-tokens }()
			found[i] = s.askRepo(ctx, entry, query)
		})
	}
	wg.Wait()
	return found
}

func (s *Service) askRepo(ctx context.Context, entry RepoEntry, query string) repoHits {
	if entry.Repo.OCI {
		return s.probeRegistry(ctx, entry, query)
	}
	list, err := s.index.Search(ctx, entry.Repo, query, searchLimit)
	if err != nil {
		return repoHits{entry: entry, failure: fmt.Sprintf("%s: %v", repoLabel(entry), err)}
	}
	return repoHits{entry: entry, charts: list}
}

func (s *Service) probeRegistry(ctx context.Context, entry RepoEntry, query string) repoHits {
	name := strings.TrimSpace(query)
	if !nameFormat.MatchString(name) {
		return repoHits{entry: entry}
	}
	tags, err := s.index.Versions(ctx, entry.Repo, name)
	if err != nil {
		return repoHits{entry: entry, failure: fmt.Sprintf("%s: %v", repoLabel(entry), err)}
	}
	if len(tags) == 0 {
		return repoHits{entry: entry}
	}
	return repoHits{entry: entry, charts: []charts.Chart{{Name: name, Version: tags[0]}}}
}

func closerFirst(query string) func(left, right api.HelmChartHit) int {
	needle := strings.ToLower(strings.TrimSpace(query))
	return func(left, right api.HelmChartHit) int {
		byRank := hitRank(left, needle) - hitRank(right, needle)
		if byRank != 0 {
			return byRank
		}
		if left.Chart != right.Chart {
			return strings.Compare(left.Chart, right.Chart)
		}
		return strings.Compare(left.Repo, right.Repo)
	}
}

func hitRank(hit api.HelmChartHit, needle string) int {
	if needle == "" {
		return 1
	}
	name := strings.ToLower(hit.Chart)
	if name == needle {
		return 0
	}
	if strings.HasPrefix(name, needle) {
		return 1
	}
	if strings.Contains(name, needle) {
		return 2
	}
	return 3
}

type ValuesRequest struct {
	Chart   string
	Version string
	RepoURL string
	OCI     bool
}

func (s *Service) ChartValues(ctx context.Context, req ValuesRequest) (api.HelmChartValues, error) {
	if s.runner == nil {
		return api.HelmChartValues{}, fmt.Errorf("%w: helm actions are not wired up", api.ErrInternal)
	}
	if !nameFormat.MatchString(req.Chart) {
		return api.HelmChartValues{}, fmt.Errorf("%q is not a chart name", req.Chart)
	}
	if !charts.ValidVersion(req.Version) {
		return api.HelmChartValues{}, fmt.Errorf("version %q is not a semantic version", req.Version)
	}
	repoErr := s.admitsRepository(req.RepoURL, req.OCI)
	if repoErr != nil {
		return api.HelmChartValues{}, repoErr
	}
	args := []string{"show", "values", chartRef(req.Chart, req.RepoURL, req.OCI), "--version", req.Version}
	if !req.OCI {
		args = append(args, "--repo", req.RepoURL)
	}
	out, err := s.run(ctx, args, "")
	if err != nil {
		return api.HelmChartValues{}, err
	}
	return api.HelmChartValues{Chart: req.Chart, Version: req.Version, Values: out}, nil
}
