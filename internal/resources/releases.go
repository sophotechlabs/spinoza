package resources

import (
	"context"
	"fmt"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func (m *Manager) HelmReleases(ctx context.Context) (api.HelmReleases, error) {
	if m.helm == nil {
		return api.HelmReleases{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	list, err := m.helm.List(ctx)
	if err != nil {
		return list, err
	}
	decorateOwners(list.Releases, m.fluxOwners(ctx))
	return list, nil
}

func (m *Manager) HelmRelease(
	ctx context.Context,
	namespace, name string,
	revision int64,
) (api.HelmReleaseDetail, error) {
	if m.helm == nil {
		return api.HelmReleaseDetail{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	detail, err := m.helm.Detail(ctx, namespace, name, revision, m.resolveKind)
	if err != nil {
		return detail, err
	}
	detail.Release.FluxRef = ownerRef(m.fluxOwners(ctx), namespace, name)
	return detail, nil
}

func (m *Manager) HelmHistory(
	ctx context.Context,
	namespace, name string,
	through int64,
) (api.HelmHistoryPage, error) {
	if m.helm == nil {
		return api.HelmHistoryPage{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.History(ctx, namespace, name, through)
}

func (m *Manager) HelmSupport() api.HelmSupport {
	if m.helm == nil {
		return api.HelmSupport{Reason: "helm is not wired up"}
	}
	return m.helm.Support()
}

func (m *Manager) HelmRollback(ctx context.Context, namespace, name string, revision int64) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Rollback(ctx, namespace, name, revision)
}

func (m *Manager) HelmUninstall(ctx context.Context, namespace, name string) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Uninstall(ctx, namespace, name)
}

func (m *Manager) HelmUpgrade(ctx context.Context, req helm.UpgradeRequest) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	owner := ownerRef(m.fluxOwners(ctx), req.Namespace, req.Name)
	if owner != nil {
		return api.HelmActionResult{}, fmt.Errorf(
			"%w: change the helmrelease object %s/%s in git instead",
			helm.ErrFluxManaged, owner.Namespace, owner.Name,
		)
	}
	return m.helm.Upgrade(ctx, req)
}

func (m *Manager) HelmVersions(ctx context.Context, chart string) (api.HelmChartVersions, error) {
	if m.helm == nil {
		return api.HelmChartVersions{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Versions(ctx, chart)
}

func (m *Manager) HelmChartSearch(ctx context.Context, query string) (api.HelmChartSearch, error) {
	if m.helm == nil {
		return api.HelmChartSearch{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.SearchCharts(ctx, query)
}

func (m *Manager) HelmChartValues(ctx context.Context, req helm.ValuesRequest) (api.HelmChartValues, error) {
	if m.helm == nil {
		return api.HelmChartValues{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.ChartValues(ctx, req)
}

func (m *Manager) HelmInstall(ctx context.Context, req helm.InstallRequest) (api.HelmActionResult, error) {
	if m.helm == nil {
		return api.HelmActionResult{}, fmt.Errorf("%w: helm is not wired up", api.ErrInternal)
	}
	return m.helm.Install(ctx, req)
}
