package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

func (m *Manager) FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error) {
	return flux.Do(ctx, m.dyn, m.descriptors(), ref, action, time.Now())
}

func (m *Manager) ArgoAction(ctx context.Context, ref api.ObjectRef, req argocd.Request) (api.ArgoActionResult, error) {
	if m.dyn == nil {
		return api.ArgoActionResult{}, fmt.Errorf("%w: no kubernetes client is wired up", api.ErrInternal)
	}
	return argocd.Do(ctx, m.dyn, ref, req)
}

func (m *Manager) TrafficSupport(ctx context.Context) api.TrafficSupport {
	if m.traffic == nil {
		return api.TrafficSupport{Reason: "prometheus is not wired up"}
	}
	value, ok := shared(ctx, &m.meshes, m.now, m.limits.TrafficTTL, func(ctx context.Context) (api.TrafficSupport, bool) {
		support := m.traffic.Support(ctx, m.now())
		return support, true
	})
	if !ok {
		return api.TrafficSupport{Reason: ctx.Err().Error()}
	}
	return value
}

func (m *Manager) TrafficGraph(ctx context.Context) api.TrafficGraph {
	if m.traffic == nil {
		return api.TrafficGraph{Error: "prometheus is not wired up"}
	}
	return m.traffic.Graph(ctx, m.now())
}

func (m *Manager) Graph(ctx context.Context) api.Graph {
	return gitops.Build(ctx, m, m.descriptors())
}

func (m *Manager) Topology(ctx context.Context, req topology.Request) api.Graph {
	return topology.Build(ctx, m, m.descriptors(), req)
}

func (m *Manager) GitopsApp(ctx context.Context, ref api.ObjectRef) (api.GitopsApp, error) {
	if m.dyn == nil {
		return api.GitopsApp{}, fmt.Errorf("%w: no kubernetes client is wired up", api.ErrInternal)
	}
	return gitops.Detail(ctx, m.dyn, m.descriptors(), ref)
}

func (m *Manager) GitopsAppGraph(ctx context.Context, ref api.ObjectRef) (api.Graph, error) {
	if m.dyn == nil {
		return api.Graph{}, fmt.Errorf("%w: no kubernetes client is wired up", api.ErrInternal)
	}
	app, err := gitops.Shape(ctx, m.dyn, m.descriptors(), ref)
	if err != nil {
		return api.Graph{}, err
	}
	return gitops.AppGraph(app), nil
}

func (m *Manager) Flux(ctx context.Context) api.FluxDashboard {
	return flux.Build(ctx, m, m.descriptors(), m.charts)
}

func (m *Manager) FluxOverview(ctx context.Context) api.FluxOverview {
	return flux.Overview(ctx, m.cs, m, m.descriptors(), flux.Cluster{
		Kubernetes: m.serverVersion(),
		Nodes:      m.nodeCount(ctx),
		Usage:      m.Metrics(ctx).Pods,
	})
}

func (m *Manager) Argo(ctx context.Context) api.ArgoDashboard {
	return argocd.Build(ctx, m, m.descriptors())
}
