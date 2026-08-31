package server

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/issues"
)

func (s *Server) fleetIssues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, pagedQueue(mergeQueues(s.everyClustersIssues(r.Context())), r))
}

func (s *Server) everyClustersIssues(ctx context.Context) []clusterAnswer[api.IssueQueue] {
	return eachCluster(ctx, s, func(ctx context.Context, backend Backend) api.IssueQueue {
		return backend.Issues(ctx)
	})
}

func nameOf(one api.OpenCluster) string {
	if one.Label != "" {
		return one.Label
	}
	return one.Context
}

func mergeQueues(found []clusterAnswer[api.IssueQueue]) api.IssueQueue {
	merged := api.IssueQueue{Rows: []api.Issue{}}
	trouble := []string{}
	for _, one := range found {
		for _, row := range one.answer.Rows {
			row.Cluster = one.cluster
			merged.Rows = append(merged.Rows, row)
		}
		merged.Dropped += one.answer.Dropped
		if one.answer.Error != "" {
			trouble = append(trouble, one.context+": "+one.answer.Error)
		}
		if one.failure != "" {
			trouble = append(trouble, one.context+": "+one.failure)
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
