package server

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func (s *Server) fleetOverview(w http.ResponseWriter, r *http.Request) {
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) api.ClusterOverview {
		return backend.Overview(ctx)
	})
	writeJSON(w, mergeOverviews(found))
}

func mergeOverviews(found []clusterAnswer[api.ClusterOverview]) api.FleetOverview {
	merged := api.FleetOverview{Clusters: []api.FleetCluster{}}
	usageKnown := len(found) > 0
	trouble := make([]string, 0, len(found))
	for _, one := range found {
		merged.Clusters = append(merged.Clusters, lineFor(one))
		if !one.answer.Nodes.UsageKnown {
			usageKnown = false
		}
		addNodes(&merged.Nodes, one.answer.Nodes)
		addPods(&merged.Pods, one.answer.Pods)
		if one.answer.Error != "" {
			trouble = append(trouble, one.context+": "+one.answer.Error)
		}
		if one.failure != "" {
			trouble = append(trouble, one.context+": "+one.failure)
		}
	}
	slices.SortStableFunc(merged.Clusters, func(left, right api.FleetCluster) int {
		return strings.Compare(left.Context, right.Context)
	})
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	merged.Nodes.UsageKnown = usageKnown
	if !usageKnown {
		merged.Nodes.CPUUsedMilli = 0
		merged.Nodes.MemUsedMi = 0
	}
	return merged
}

func lineFor(one clusterAnswer[api.ClusterOverview]) api.FleetCluster {
	return api.FleetCluster{
		Cluster:  one.cluster,
		Context:  one.context,
		Version:  one.answer.Version,
		Nodes:    one.answer.Nodes,
		Pods:     one.answer.Pods,
		Warnings: one.answer.WarningCount,
		Reason:   reasonOf(one),
	}
}

func reasonOf(one clusterAnswer[api.ClusterOverview]) string {
	if one.failure != "" {
		return one.failure
	}
	return one.answer.Error
}

func addNodes(into *api.NodeSummary, one api.NodeSummary) {
	into.Total += one.Total
	into.Ready += one.Ready
	into.Unschedulable += one.Unschedulable
	into.CPUAllocatableMilli += one.CPUAllocatableMilli
	into.MemAllocatableMi += one.MemAllocatableMi
	if !one.UsageKnown {
		return
	}
	into.CPUUsedMilli += one.CPUUsedMilli
	into.MemUsedMi += one.MemUsedMi
}

func addPods(into *api.PodSummary, one api.PodSummary) {
	if !one.Known {
		return
	}
	into.Known = true
	into.Total += one.Total
	into.Running += one.Running
	into.Pending += one.Pending
	into.Failed += one.Failed
	into.Succeeded += one.Succeeded
	for _, capped := range one.Capped {
		if !slices.Contains(into.Capped, capped) {
			into.Capped = append(into.Capped, capped)
		}
	}
}
