package helm

import (
	"context"
	"fmt"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func (s *Service) Versions(ctx context.Context, chart string) (api.HelmChartVersions, error) {
	if !nameFormat.MatchString(chart) {
		return api.HelmChartVersions{}, fmt.Errorf("%q is not a chart name", chart)
	}
	out := api.HelmChartVersions{Chart: chart, Repos: []api.HelmRepoVersions{}}
	if len(s.repos) == 0 {
		out.Error = "no chart repositories are configured; add one with helm repo add"
		return out, nil
	}
	if s.index == nil {
		out.Error = "chart repositories are not wired up"
		return out, nil
	}
	failures := []string{}
	for _, entry := range s.repos {
		list, err := s.index.Versions(ctx, entry.Repo, chart)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", repoLabel(entry), err))
			continue
		}
		if len(list) == 0 {
			continue
		}
		out.Repos = append(out.Repos, api.HelmRepoVersions{
			Name:     entry.Name,
			URL:      entry.Repo.URL,
			OCI:      entry.Repo.OCI,
			Versions: list,
		})
	}
	out.Error = strings.Join(failures, "; ")
	return out, nil
}

func repoLabel(entry RepoEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	return entry.Repo.URL
}
