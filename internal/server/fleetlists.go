package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const fleetSearchCap = 200

const fleetReadFailure = "spinoza could not finish reading this cluster"

// One cluster that accepts the connection and never answers must not decide how
// long the whole fleet takes, so every cluster gets its own deadline.
const perClusterTimeout = 30 * time.Second

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
	return eachOpenClusterWithin(ctx, srv, perClusterTimeout, read)
}

func eachOpenClusterWithin[T any](
	ctx context.Context,
	srv *Server,
	timeout time.Duration,
	read func(context.Context, api.OpenCluster, Backend) T,
) []clusterAnswer[T] {
	open := srv.cluster.Opened()
	found := make([]clusterAnswer[T], len(open))
	var asking sync.WaitGroup
	for at, one := range open {
		index := at
		cluster := one
		asking.Add(1)
		safe.Go("reading fleet cluster "+nameOf(cluster), func() {
			defer asking.Done()
			found[index] = clusterAnswer[T]{cluster: cluster.ID, context: nameOf(cluster)}
			defer func() {
				failure := recovered("reading fleet cluster "+nameOf(cluster), recover())
				if failure != "" {
					found[index].failure = failure
				}
			}()
			backend := srv.managerOf(cluster.ID)
			if backend == nil {
				found[index].failure = "cluster is unavailable"
				return
			}
			asked, giveUp := context.WithTimeout(ctx, timeout)
			defer giveUp()
			answered := make(chan clusterRead[T], 1)
			safe.Go("asking "+nameOf(cluster), func() {
				result := clusterRead[T]{}
				defer func() {
					result.failure = recovered("asking "+nameOf(cluster), recover())
					answered <- result
				}()
				result.answer = read(asked, cluster, backend)
			})
			select {
			case result := <-answered:
				found[index].answer = result.answer
				found[index].failure = result.failure
			case <-asked.Done():
				if ctx.Err() != nil {
					found[index].failure = ctx.Err().Error()
					return
				}
				found[index].failure = "cluster stopped answering before the fleet deadline"
			}
		})
	}
	asking.Wait()
	return found
}

type clusterRead[T any] struct {
	answer  T
	failure string
}

func recovered(what string, caught any) string {
	if caught == nil {
		return ""
	}
	safe.Log(what, caught)
	return fleetReadFailure
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
	release, claimed := s.releaseReads.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm release reads are busy; try again")
		return
	}
	defer release()
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
