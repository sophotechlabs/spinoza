package server

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type Catalog interface {
	Resources() api.ResourceCatalog
	RefreshResources() api.ResourceCatalog
	Counts(ctx context.Context) api.ResourceCounts
	Search(ctx context.Context, query string) api.SearchResults
	Namespaces(ctx context.Context) api.Namespaces
}

type Objects interface {
	Ping(ctx context.Context) error
	Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error)
	Access(ctx context.Context, ref api.ObjectRef) api.Access
	ApplyObject(ctx context.Context, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error)
	DeleteObject(ctx context.Context, ref api.ObjectRef) error
	Events(ctx context.Context, namespace, uid string) ([]api.Event, error)
	ListKind(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error)
	Schema(ctx context.Context, gvk jsonschema.GVK) (json.RawMessage, error)
}

type Feeds interface {
	Subscribe(
		ctx context.Context,
		group, version, resource, namespace string,
		limit int,
		filters []api.RowFilter,
	) (*resources.Subscription, error)
	Logs(ctx context.Context, req logs.Request) (*logs.Stream, error)
	PodSelector(ctx context.Context, ref api.ObjectRef) (string, error)
}

type Views interface {
	Graph(ctx context.Context) api.Graph
	Flux(ctx context.Context) api.FluxDashboard
	Argo(ctx context.Context) api.ArgoDashboard
	FluxOverview(ctx context.Context) api.FluxOverview
	Metrics(ctx context.Context) api.Metrics
	MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error)
	Overview(ctx context.Context) api.ClusterOverview
}

type Releases interface {
	ChartRepos
	HelmReleases(ctx context.Context) (api.HelmReleases, error)
	HelmRelease(ctx context.Context, namespace, name string) (api.HelmReleaseDetail, error)
	HelmSupport() api.HelmSupport
	HelmRollback(ctx context.Context, namespace, name string, revision int64) (api.HelmActionResult, error)
	HelmUninstall(ctx context.Context, namespace, name string) (api.HelmActionResult, error)
	HelmUpgrade(ctx context.Context, req helm.UpgradeRequest) (api.HelmActionResult, error)
}

type ChartRepos interface {
	HelmVersions(ctx context.Context, chart string) (api.HelmChartVersions, error)
	HelmChartSearch(ctx context.Context, query string) (api.HelmChartSearch, error)
	HelmChartValues(ctx context.Context, req helm.ValuesRequest) (api.HelmChartValues, error)
	HelmInstall(ctx context.Context, req helm.InstallRequest) (api.HelmActionResult, error)
}

type Changes interface {
	Action(ctx context.Context, req actions.Request) (api.ActionResult, error)
	FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error)
	ArgoAction(ctx context.Context, ref api.ObjectRef, action argocd.Action) (api.ArgoActionResult, error)
}

type Forwarding interface {
	Forwards() []api.PortForward
	StartForward(ctx context.Context, target portforward.Target, port int32) (api.PortForward, error)
	StopForward(id string) error
}

type Terminals interface {
	ExecSupport(ctx context.Context, req exec.Request) (api.ExecSupport, error)
	StartExec(ctx context.Context, req exec.Request, stdout io.Writer) (*exec.Session, error)
	DebugSupport(ctx context.Context, namespace, pod string) api.DebugSupport
	StartDebug(ctx context.Context, req debugcontainer.Request) (api.DebugSession, error)
	NodeShellSupport(ctx context.Context, node string) api.NodeShellSupport
	StartNodeShell(ctx context.Context, node string) (api.NodeShellSession, error)
	RemoveNodeShell(ctx context.Context, pod string)
}

type Backend interface {
	Catalog
	Objects
	Feeds
	Views
	Releases
	Changes
	Forwarding
	Terminals
}
