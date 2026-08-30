package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/issues"
)

type clusterQueue struct {
	cluster string
	context string
	queue   api.IssueQueue
}

func (s *Server) fleetIssues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, pagedQueue(mergeQueues(s.everyClustersIssues(r.Context())), r))
}

func (s *Server) everyClustersIssues(ctx context.Context) []clusterQueue {
	open := s.cluster.Opened()
	found := make([]clusterQueue, len(open))
	var asking sync.WaitGroup
	for at, one := range open {
		asking.Add(1)
		go func(at int, one api.OpenCluster) {
			defer asking.Done()
			found[at] = clusterQueue{cluster: one.ID, context: nameOf(one)}
			backend := s.managerOf(one.ID)
			if backend == nil {
				return
			}
			found[at].queue = backend.Issues(ctx)
		}(at, one)
	}
	asking.Wait()
	return found
}

func nameOf(one api.OpenCluster) string {
	if one.Label != "" {
		return one.Label
	}
	return one.Context
}

// The merged queue is ordered and capped the way one cluster's is, so the worst
// thing in the fleet is at the top whichever cluster it is on.
func mergeQueues(found []clusterQueue) api.IssueQueue {
	merged := api.IssueQueue{Rows: []api.Issue{}}
	trouble := []string{}
	for _, one := range found {
		for _, row := range one.queue.Rows {
			row.Cluster = one.cluster
			merged.Rows = append(merged.Rows, row)
		}
		merged.Dropped += one.queue.Dropped
		if one.queue.Error != "" {
			trouble = append(trouble, one.context+": "+one.queue.Error)
		}
	}
	issues.Rank(merged.Rows, issues.ByWorst)
	if len(merged.Rows) > issues.MaxRows {
		merged.Dropped += len(merged.Rows) - issues.MaxRows
		merged.Rows = merged.Rows[:issues.MaxRows]
	}
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	return merged
}
