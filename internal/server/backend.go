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
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/rbac"
	"github.com/sophotechlabs/spinoza/internal/reach"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

type Catalog interface {
	Resources() api.ResourceCatalog
	RefreshResources() api.ResourceCatalog
	Counts(ctx context.Context) api.ResourceCounts
	Search(ctx context.Context, query string) api.SearchResults
	Namespaces(ctx context.Context) api.Namespaces
}

type Liveness interface {
	Ping(ctx context.Context) error
	Reach() *reach.Sink
}

type Objects interface {
	Permissions
	Liveness

	Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error)
	Events(ctx context.Context, namespace, uid string) ([]api.Event, error)
	ListKind(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error)
	Schema(ctx context.Context, gvk jsonschema.GVK) (json.RawMessage, error)
}

type Permissions interface {
	RBACIndex(ctx context.Context) rbac.Index
	Access(ctx context.Context, ref api.ObjectRef) api.Access
	AccessEach(ctx context.Context, capability string, refs []api.ObjectRef) api.BulkAccess
	HelmAccess(ctx context.Context, namespace, name string) api.Access
	Scope(ctx context.Context) api.Scope
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
	Record(ctx context.Context, into resources.Timeline, kinds []resources.Kind)
	StopRecording()
}

type Views interface {
	Deliveries
	Reports
	Traffic
}

type Deliveries interface {
	Graph(ctx context.Context) api.Graph
	Topology(ctx context.Context, req topology.Request) api.Graph
	Flux(ctx context.Context) api.FluxDashboard
	Argo(ctx context.Context) api.ArgoDashboard
	FluxOverview(ctx context.Context) api.FluxOverview
}

type Reports interface {
	Overview(ctx context.Context) api.ClusterOverview
	Checks(ctx context.Context, keep checks.Filter) api.CheckReport
	CheckPage(ctx context.Context, id, after string, keep checks.Filter) (api.CheckPage, error)
	CheckFingerprint(ctx context.Context, keep checks.Filter) checks.Baseline
	CheckExport(ctx context.Context, keep checks.Filter) api.CheckReport
	Issues(ctx context.Context) api.IssueQueue
	Metrics(ctx context.Context) api.Metrics
	MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error)
}

type Traffic interface {
	TrafficSupport(ctx context.Context) api.TrafficSupport
	TrafficGraph(ctx context.Context) api.TrafficGraph
}

type Releases interface {
	ChartRepos
	HelmReleases(ctx context.Context) (api.HelmReleases, error)
	HelmRelease(ctx context.Context, namespace, name string) (api.HelmReleaseDetail, error)
	HelmSupport() api.HelmSupport
}

type ChartRepos interface {
	HelmVersions(ctx context.Context, chart string) (api.HelmChartVersions, error)
	HelmChartSearch(ctx context.Context, query string) (api.HelmChartSearch, error)
	HelmChartValues(ctx context.Context, req helm.ValuesRequest) (api.HelmChartValues, error)
}

type Gitops interface {
	GitopsApp(ctx context.Context, ref api.ObjectRef) (api.GitopsApp, error)
	GitopsAppGraph(ctx context.Context, ref api.ObjectRef) (api.Graph, error)
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
	NodeShellSupport(ctx context.Context, node string) api.NodeShellSupport
}

type ObjectWrites interface {
	Action(ctx context.Context, req actions.Request) (api.ActionResult, error)
	ApplyObject(ctx context.Context, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error)
	DeleteObject(ctx context.Context, ref api.ObjectRef) error
}

type DeliveryWrites interface {
	FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error)
	ArgoAction(ctx context.Context, ref api.ObjectRef, req argocd.Request) (api.ArgoActionResult, error)
}

type ReleaseWrites interface {
	HelmInstall(ctx context.Context, req helm.InstallRequest) (api.HelmActionResult, error)
	HelmUpgrade(ctx context.Context, req helm.UpgradeRequest) (api.HelmActionResult, error)
	HelmRollback(ctx context.Context, namespace, name string, revision int64) (api.HelmActionResult, error)
	HelmUninstall(ctx context.Context, namespace, name string) (api.HelmActionResult, error)
}

type TerminalWrites interface {
	StartDebug(ctx context.Context, req debugcontainer.Request) (api.DebugSession, error)
	StartNodeShell(ctx context.Context, node string) (api.NodeShellSession, error)
	RemoveNodeShell(ctx context.Context, pod string) error
}

type Writer interface {
	ObjectWrites
	DeliveryWrites
	ReleaseWrites
	TerminalWrites
}

type Reader interface {
	Catalog
	Objects
	Feeds
	Views
	Releases
	Gitops
	Forwarding
	Terminals
}

type Backend interface {
	Reader
	Writer
}
