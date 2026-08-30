package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type clusterReport struct {
	cluster string
	context string
	report  api.CheckReport
}

func (s *Server) fleetChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, mergeReports(s.everyClustersChecks(r.Context(), r)))
}

func (s *Server) everyClustersChecks(ctx context.Context, r *http.Request) []clusterReport {
	open := s.cluster.Opened()
	found := make([]clusterReport, len(open))
	var asking sync.WaitGroup
	for at, one := range open {
		asking.Add(1)
		go func(at int, one api.OpenCluster) {
			defer asking.Done()
			found[at] = clusterReport{cluster: one.ID, context: nameOf(one)}
			backend := s.managerOf(one.ID)
			if backend == nil {
				return
			}
			found[at].report = backend.Checks(ctx, s.checkFilterOn(r, one.ID))
		}(at, one)
	}
	asking.Wait()
	return found
}

// The merged report is one row per rule, the way a single cluster's is: the
// question a fleet asks is which rules are failing and where, not which cluster
// has its own copy of the same rule.
func mergeReports(found []clusterReport) api.CheckReport {
	merged := api.CheckReport{Groups: []api.CheckGroup{}, Objects: []api.CheckObject{}}
	at := map[string]int{}
	trouble := []string{}
	spread := map[string]api.NamespaceCount{}
	for _, one := range found {
		offset := len(merged.Objects)
		merged.Objects = append(merged.Objects, stamped(one)...)
		foldGroups(&merged, at, one.report.Groups, offset)
		for _, count := range one.report.Namespaces {
			into := spread[count.Namespace]
			into.Namespace = count.Namespace
			into.Total += count.Total
			into.High += count.High
			into.Medium += count.Medium
			into.Low += count.Low
			spread[count.Namespace] = into
		}
		merged.Scanned += one.report.Scanned
		if one.report.Error != "" {
			trouble = append(trouble, one.context+": "+one.report.Error)
		}
	}
	merged.Namespaces = spreadOf(spread)
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	return merged
}

func stamped(one clusterReport) []api.CheckObject {
	out := make([]api.CheckObject, 0, len(one.report.Objects))
	for _, obj := range one.report.Objects {
		obj.Cluster = one.cluster
		out = append(out, obj)
	}
	return out
}

// Every cluster walks the same rule registry, so first-seen order is registry
// order and a rule nobody else reported still lands in its own place.
func foldGroups(merged *api.CheckReport, at map[string]int, groups []api.CheckGroup, offset int) {
	for _, group := range groups {
		found := shifted(group, offset)
		where, seen := at[group.ID]
		if !seen {
			at[group.ID] = len(merged.Groups)
			merged.Groups = append(merged.Groups, found)
			continue
		}
		into := &merged.Groups[where]
		into.Total += found.Total
		into.Muted += found.Muted
		into.NewCount += found.NewCount
		into.Fixed += found.Fixed
		into.Truncated = into.Truncated || found.Truncated
		into.Findings = append(into.Findings, found.Findings...)
		if into.Skipped != "" && found.Skipped == "" {
			into.Skipped = ""
		}
	}
}

// A finding points at an object by position, so merging two reports means
// moving each cluster's findings along by however many objects came before it.
func shifted(group api.CheckGroup, offset int) api.CheckGroup {
	moved := make([]api.CheckFinding, 0, len(group.Findings))
	for _, one := range group.Findings {
		one.Ref += offset
		moved = append(moved, one)
	}
	group.Findings = moved
	group.Baselined = false
	group.Next = ""
	return group
}

func spreadOf(counts map[string]api.NamespaceCount) []api.NamespaceCount {
	if len(counts) == 0 {
		return nil
	}
	out := make([]api.NamespaceCount, 0, len(counts))
	for _, count := range counts {
		out = append(out, count)
	}
	slices.SortFunc(out, func(left, right api.NamespaceCount) int {
		if left.Total != right.Total {
			return right.Total - left.Total
		}
		return strings.Compare(left.Namespace, right.Namespace)
	})
	return out
}
