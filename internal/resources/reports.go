package resources

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/issues"
	"github.com/sophotechlabs/spinoza/internal/metrics"
	"github.com/sophotechlabs/spinoza/internal/overview"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

func (m *Manager) MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error) {
	if !m.filter(ctx).allows(namespace) {
		return api.MetricHistory{}, fmt.Errorf("%w: %s", ErrOutOfScope, namespace)
	}
	if m.prom != nil {
		history, err := m.prom.PodHistory(ctx, namespace, pod, span, time.Now())
		if err == nil {
			return history, nil
		}
		if !errors.Is(err, prom.ErrUnavailable) {
			return api.MetricHistory{}, err
		}
	}
	if m.samples == nil {
		return api.MetricHistory{}, prom.ErrUnavailable
	}
	m.Metrics(ctx)
	return m.samples.History(namespace, pod, span, m.now()), nil
}

func (m *Manager) CheckPage(
	ctx context.Context, id, after string, keep checks.Filter,
) (api.CheckPage, error) {
	return m.surveys.Page(ctx, m, m.descriptors(), m.Metrics(ctx), id, after, keep, m.limits.CheckFindings)
}

func (m *Manager) Facts() checks.Facts {
	out := checks.Facts{}
	if m.disco == nil {
		return out
	}
	if info, err := m.disco.ServerVersion(); err == nil {
		out.ServerVersion = info.GitVersion
	}
	if groups, err := m.disco.ServerGroups(); err == nil {
		for _, group := range groups.Groups {
			for _, version := range group.Versions {
				out.ServedVersions = append(out.ServedVersions, version.GroupVersion)
			}
		}
	}
	if m.warnings != nil {
		out.Warnings = m.warnings.Seen()
	}
	return out
}

func (m *Manager) Checks(ctx context.Context, keep checks.Filter) api.CheckReport {
	return m.surveys.Run(ctx, m, m.descriptors(), m.Metrics(ctx), keep, m.limits.CheckFindings)
}

func (m *Manager) CheckExport(ctx context.Context, keep checks.Filter) api.CheckReport {
	return checks.Run(ctx, m, m.descriptors(), m.Metrics(ctx), keep, everyFinding)
}

func (m *Manager) CheckFingerprint(ctx context.Context, keep checks.Filter) checks.Baseline {
	return checks.Fingerprint(ctx, m, m.descriptors(), m.Metrics(ctx), keep)
}

func (m *Manager) Metrics(ctx context.Context) api.Metrics {
	value, ok := shared(ctx, &m.usage, m.now, m.limits.MetricsTTL, func(ctx context.Context) (api.Metrics, bool) {
		built := metrics.Build(ctx, m.dyn, m.nodeSource())
		if built.Error == "" {
			m.samples.Record(m.now(), built.Pods)
		}
		return built, built.Error == ""
	})
	if !ok {
		return api.Metrics{Error: ctx.Err().Error()}
	}
	return value
}

func (m *Manager) Overview(ctx context.Context) api.ClusterOverview {
	out := overview.Build(ctx, m.dyn, m.meta, m, m.versions(), m.descriptors())
	out.Controllers = gitops.Controllers(ctx, m.cs)
	return out
}

func (m *Manager) Issues(ctx context.Context) api.IssueQueue {
	return issues.Build(ctx, m, m, m.descriptors(), m.now, issues.Limits{})
}
