package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const fleetSearchCap = 200

func eachCluster[T any](
	ctx context.Context, srv *Server, read func(context.Context, Backend) T,
) []clusterAnswer[T] {
	return eachOpenCluster(ctx, srv, func(ctx context.Context, _ api.OpenCluster, backend Backend) T {
		return read(ctx, backend)
	})
}

func eachOpenCluster[T any](
	ctx context.Context, srv *Server, read func(context.Context, api.OpenCluster, Backend) T,
) []clusterAnswer[T] {
	open := srv.cluster.Opened()
	found := make([]clusterAnswer[T], len(open))
	var asking sync.WaitGroup
	for at, one := range open {
		asking.Add(1)
		go func(at int, one api.OpenCluster) {
			defer asking.Done()
			found[at] = clusterAnswer[T]{cluster: one.ID, context: nameOf(one)}
			defer func() {
				found[at].failure = recovered("asking "+nameOf(one), recover())
			}()
			backend := srv.managerOf(one.ID)
			if backend == nil {
				return
			}
			found[at].answer = read(ctx, one, backend)
		}(at, one)
	}
	asking.Wait()
	return found
}

func recovered(what string, caught any) string {
	if caught == nil {
		return ""
	}
	safe.Log(what, caught)
	return "panicked: " + fmt.Sprint(caught)
}

type clusterAnswer[T any] struct {
	cluster string
	context string
	failure string
	answer  T
}

func (s *Server) fleetSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) api.SearchResults {
		return backend.Search(ctx, query)
	})
	writeJSON(w, mergeSearch(found))
}

func mergeSearch(found []clusterAnswer[api.SearchResults]) api.SearchResults {
	merged := api.SearchResults{Hits: []api.SearchHit{}}
	trouble := map[string]string{}
	for _, one := range found {
		for _, hit := range one.answer.Hits {
			hit.Cluster = one.cluster
			merged.Hits = append(merged.Hits, hit)
		}
		merged.Truncated = merged.Truncated || one.answer.Truncated
		for where, why := range one.answer.Errors {
			trouble[one.context+": "+where] = why
		}
		if one.failure != "" {
			trouble[one.context] = one.failure
		}
	}
	slices.SortStableFunc(merged.Hits, byWhere)
	if len(merged.Hits) > fleetSearchCap {
		merged.Hits = merged.Hits[:fleetSearchCap]
		merged.Truncated = true
	}
	if len(trouble) > 0 {
		merged.Errors = trouble
	}
	return merged
}

func byWhere(left, right api.SearchHit) int {
	if left.Name != right.Name {
		return strings.Compare(left.Name, right.Name)
	}
	if left.Cluster != right.Cluster {
		return strings.Compare(left.Cluster, right.Cluster)
	}
	return strings.Compare(left.Namespace, right.Namespace)
}

func (s *Server) fleetHelm(w http.ResponseWriter, r *http.Request) {
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) api.HelmReleases {
		held, err := backend.HelmReleases(ctx)
		if err != nil {
			return api.HelmReleases{Error: err.Error()}
		}
		return held
	})
	writeJSON(w, mergeReleases(found))
}

func mergeReleases(found []clusterAnswer[api.HelmReleases]) api.HelmReleases {
	merged := api.HelmReleases{Releases: []api.HelmRelease{}}
	trouble := []string{}
	for _, one := range found {
		for _, release := range one.answer.Releases {
			release.Cluster = one.cluster
			merged.Releases = append(merged.Releases, release)
		}
		if one.answer.Error != "" {
			trouble = append(trouble, one.context+": "+one.answer.Error)
		}
		if one.failure != "" {
			trouble = append(trouble, one.context+": "+one.failure)
		}
	}
	markSkew(merged.Releases)
	slices.SortStableFunc(merged.Releases, byChart)
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	return merged
}

func markSkew(releases []api.HelmRelease) {
	versions := map[string]map[string]struct{}{}
	for _, one := range releases {
		if versions[one.Chart] == nil {
			versions[one.Chart] = map[string]struct{}{}
		}
		versions[one.Chart][one.ChartVersion] = struct{}{}
	}
	for at := range releases {
		held := versions[releases[at].Chart]
		if len(held) < 2 {
			continue
		}
		releases[at].Skew = strings.Join(sortedKeys(held), " · ")
	}
}

func sortedKeys(held map[string]struct{}) []string {
	out := make([]string, 0, len(held))
	for one := range held {
		out = append(out, one)
	}
	slices.Sort(out)
	return out
}

func byChart(left, right api.HelmRelease) int {
	if left.Chart != right.Chart {
		return strings.Compare(left.Chart, right.Chart)
	}
	if left.Cluster != right.Cluster {
		return strings.Compare(left.Cluster, right.Cluster)
	}
	return strings.Compare(left.Namespace+"/"+left.Name, right.Namespace+"/"+right.Name)
}
