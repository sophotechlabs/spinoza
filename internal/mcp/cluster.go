package mcp

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

type Health interface {
	Overview(ctx context.Context) api.ClusterOverview
	Counts(ctx context.Context) api.ResourceCounts
	Issues(ctx context.Context) api.IssueQueue
	Checks(ctx context.Context, keep checks.Filter) api.CheckReport
}

type Inventory interface {
	Resources() api.ResourceCatalog
	Namespaces(ctx context.Context) api.Namespaces
	ListKind(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error)
	Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error)
	Events(ctx context.Context, namespace, uid string) ([]api.Event, error)
	Search(ctx context.Context, query string) api.SearchResults
}

type Shape interface {
	Topology(ctx context.Context, req topology.Request) api.Graph
	Metrics(ctx context.Context) api.Metrics
}

type Streams interface {
	LogLines(ctx context.Context, req logs.Request) ([]string, error)
	PodSelector(ctx context.Context, ref api.ObjectRef) (string, error)
}

type Releases interface {
	HelmReleases(ctx context.Context) (api.HelmReleases, error)
	HelmRelease(ctx context.Context, namespace, name string) (api.HelmReleaseDetail, error)
}

type Writes interface {
	Access(ctx context.Context, ref api.ObjectRef) api.Access
	Action(ctx context.Context, req actions.Request) (api.ActionResult, error)
	ApplyObject(ctx context.Context, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error)
	FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error)
	ArgoAction(ctx context.Context, ref api.ObjectRef, req argocd.Request) (api.ArgoActionResult, error)
}

type Cluster interface {
	Health
	Inventory
	Shape
	Streams
	Releases
	Writes
}

type Prometheus interface {
	Instant(ctx context.Context, query string, at time.Time) ([]prom.Sample, error)
}
