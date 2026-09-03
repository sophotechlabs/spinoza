package server

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type delivery struct {
	flux api.FluxDashboard
	argo api.ArgoDashboard
}

func (s *Server) fleetGitops(w http.ResponseWriter, r *http.Request) {
	found := eachCluster(r.Context(), s, func(ctx context.Context, backend Backend) delivery {
		return delivery{flux: backend.Flux(ctx), argo: backend.Argo(ctx)}
	})
	writeJSON(w, mergeGitops(found))
}

func mergeGitops(found []clusterAnswer[delivery]) api.FleetGitops {
	merged := api.FleetGitops{Apps: []api.FleetApp{}}
	trouble := make([]string, 0, 2*len(found))
	for _, one := range found {
		merged.Apps = append(merged.Apps, fluxApps(one)...)
		merged.Apps = append(merged.Apps, argoApps(one)...)
		trouble = append(trouble, named(one.context, one.answer.flux.Error)...)
		trouble = append(trouble, named(one.context, one.answer.argo.Error)...)
		trouble = append(trouble, named(one.context, one.failure)...)
	}
	markSpread(merged.Apps)
	slices.SortStableFunc(merged.Apps, byApp)
	slices.Sort(trouble)
	merged.Error = strings.Join(trouble, " · ")
	return merged
}

func named(where, why string) []string {
	if why == "" {
		return nil
	}
	return []string{where + ": " + why}
}

func fluxApps(one clusterAnswer[delivery]) []api.FleetApp {
	out := []api.FleetApp{}
	for _, group := range one.answer.flux.Groups {
		for _, held := range group.Resources {
			out = append(out, api.FleetApp{
				Cluster:   one.cluster,
				Engine:    api.EngineFlux,
				Kind:      held.Kind,
				Group:     held.Group,
				Version:   held.Version,
				Resource:  held.Resource,
				Name:      held.Name,
				Namespace: held.Namespace,
				Ready:     held.Ready,
				Revision:  held.Revision,
				Source:    held.Source,
				Message:   held.Message,
				Suspended: held.Suspended,
			})
		}
	}
	return out
}

func argoApps(one clusterAnswer[delivery]) []api.FleetApp {
	out := make([]api.FleetApp, 0, len(one.answer.argo.Apps))
	for _, held := range one.answer.argo.Apps {
		out = append(out, api.FleetApp{
			Cluster:   one.cluster,
			Engine:    api.EngineArgo,
			Kind:      held.Kind,
			Group:     held.Group,
			Version:   held.Version,
			Resource:  held.Resource,
			Name:      held.Name,
			Namespace: held.Namespace,
			Ready:     held.Health,
			Sync:      held.Sync,
			Revision:  held.Revision,
			Source:    held.Repo,
			Message:   held.Message,
		})
	}
	return out
}

func markSpread(apps []api.FleetApp) {
	on := map[string]map[string]struct{}{}
	for _, one := range apps {
		key := appKey(one)
		if on[key] == nil {
			on[key] = map[string]struct{}{}
		}
		on[key][one.Cluster] = struct{}{}
	}
	for at := range apps {
		apps[at].Spread = len(on[appKey(apps[at])])
	}
}

func appKey(app api.FleetApp) string {
	return strings.Join([]string{app.Group, app.Resource, app.Namespace, app.Name}, "/")
}

func byApp(left, right api.FleetApp) int {
	if left.Name != right.Name {
		return strings.Compare(left.Name, right.Name)
	}
	if left.Cluster != right.Cluster {
		return strings.Compare(left.Cluster, right.Cluster)
	}
	return strings.Compare(left.Namespace, right.Namespace)
}
