package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
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

type notStubbed struct {
	t *testing.T
}

func (n notStubbed) missing(method string) {
	complaint := fmt.Sprintf(
		"this stub does not implement Backend.%s, so the code under test reached a method "+
			"nobody wrote for it; add it to the stub instead of letting it answer with nothing",
		method,
	)
	if n.t == nil {
		panic(complaint)
	}
	n.t.Errorf("%s", complaint)
}

func (n notStubbed) Access(_ context.Context, _ api.ObjectRef) (r0 api.Access) {
	n.missing("Access")
	return r0
}

func (n notStubbed) AccessEach(_ context.Context, _ string, _ []api.ObjectRef) (r0 api.BulkAccess) {
	n.missing("AccessEach")
	return r0
}

func (n notStubbed) Action(_ context.Context, _ actions.Request) (r0 api.ActionResult, r1 error) {
	n.missing("Action")
	return r0, r1
}

func (n notStubbed) ApplyObject(_ context.Context, _ api.ObjectRef, _ []byte) (r0 api.ObjectDetail, r1 error) {
	n.missing("ApplyObject")
	return r0, r1
}

func (n notStubbed) Argo(_ context.Context) (r0 api.ArgoDashboard) {
	n.missing("Argo")
	return r0
}

func (n notStubbed) ArgoAction(_ context.Context, _ api.ObjectRef, _ argocd.Request) (r0 api.ArgoActionResult, r1 error) {
	n.missing("ArgoAction")
	return r0, r1
}

func (n notStubbed) CheckPage(_ context.Context, _, _ string, _ checks.Filter) (r0 api.CheckPage, r1 error) {
	n.missing("CheckPage")
	return r0, r1
}

func (n notStubbed) CheckFingerprint(_ context.Context, _ checks.Filter) (r0 checks.Baseline) {
	n.missing("CheckFingerprint")
	return r0
}

func (n notStubbed) CheckExport(_ context.Context, _ checks.Filter) (r0 api.CheckReport) {
	n.missing("CheckExport")
	return r0
}

func (n notStubbed) Checks(_ context.Context, _ checks.Filter) (r0 api.CheckReport) {
	n.missing("Checks")
	return r0
}

func (n notStubbed) Counts(_ context.Context) (r0 api.ResourceCounts) {
	n.missing("Counts")
	return r0
}

func (n notStubbed) DebugSupport(_ context.Context, _, _ string) (r0 api.DebugSupport) {
	n.missing("DebugSupport")
	return r0
}

func (n notStubbed) DeleteObject(_ context.Context, _ api.ObjectRef) (r0 error) {
	n.missing("DeleteObject")
	return r0
}

func (n notStubbed) Events(_ context.Context, _, _ string) (r0 []api.Event, r1 error) {
	n.missing("Events")
	return r0, r1
}

func (n notStubbed) ExecSupport(_ context.Context, _ exec.Request) (r0 api.ExecSupport, r1 error) {
	n.missing("ExecSupport")
	return r0, r1
}

func (n notStubbed) Flux(_ context.Context) (r0 api.FluxDashboard) {
	n.missing("Flux")
	return r0
}

func (n notStubbed) FluxAction(_ context.Context, _ api.ObjectRef, _ flux.Action) (r0 api.FluxActionResult, r1 error) {
	n.missing("FluxAction")
	return r0, r1
}

func (n notStubbed) FluxOverview(_ context.Context) (r0 api.FluxOverview) {
	n.missing("FluxOverview")
	return r0
}

func (n notStubbed) Forwards() (r0 []api.PortForward) {
	n.missing("Forwards")
	return r0
}

func (n notStubbed) GitopsApp(_ context.Context, _ api.ObjectRef) (r0 api.GitopsApp, r1 error) {
	n.missing("GitopsApp")
	return r0, r1
}

func (n notStubbed) GitopsAppGraph(_ context.Context, _ api.ObjectRef) (r0 api.Graph, r1 error) {
	n.missing("GitopsAppGraph")
	return r0, r1
}

func (n notStubbed) Graph(_ context.Context) (r0 api.Graph) {
	n.missing("Graph")
	return r0
}

func (n notStubbed) HelmAccess(_ context.Context, _, _ string) (r0 api.Access) {
	n.missing("HelmAccess")
	return r0
}

func (n notStubbed) HelmChartSearch(_ context.Context, _ string) (r0 api.HelmChartSearch, r1 error) {
	n.missing("HelmChartSearch")
	return r0, r1
}

func (n notStubbed) HelmChartValues(_ context.Context, _ helm.ValuesRequest) (r0 api.HelmChartValues, r1 error) {
	n.missing("HelmChartValues")
	return r0, r1
}

func (n notStubbed) HelmInstall(_ context.Context, _ helm.InstallRequest) (r0 api.HelmActionResult, r1 error) {
	n.missing("HelmInstall")
	return r0, r1
}

func (n notStubbed) HelmRelease(_ context.Context, _, _ string) (r0 api.HelmReleaseDetail, r1 error) {
	n.missing("HelmRelease")
	return r0, r1
}

func (n notStubbed) HelmReleases(_ context.Context) (r0 api.HelmReleases, r1 error) {
	n.missing("HelmReleases")
	return r0, r1
}

func (n notStubbed) HelmRollback(_ context.Context, _, _ string, _ int64) (r0 api.HelmActionResult, r1 error) {
	n.missing("HelmRollback")
	return r0, r1
}

func (n notStubbed) HelmSupport() (r0 api.HelmSupport) {
	n.missing("HelmSupport")
	return r0
}

func (n notStubbed) HelmUninstall(_ context.Context, _, _ string) (r0 api.HelmActionResult, r1 error) {
	n.missing("HelmUninstall")
	return r0, r1
}

func (n notStubbed) HelmUpgrade(_ context.Context, _ helm.UpgradeRequest) (r0 api.HelmActionResult, r1 error) {
	n.missing("HelmUpgrade")
	return r0, r1
}

func (n notStubbed) HelmVersions(_ context.Context, _ string) (r0 api.HelmChartVersions, r1 error) {
	n.missing("HelmVersions")
	return r0, r1
}

func (n notStubbed) Issues(_ context.Context) (r0 api.IssueQueue) {
	n.missing("Issues")
	return r0
}

func (n notStubbed) ListKind(_ context.Context, _ api.ObjectRef) (r0 []*unstructured.Unstructured, r1 error) {
	n.missing("ListKind")
	return r0, r1
}

func (n notStubbed) Logs(_ context.Context, _ logs.Request) (r0 *logs.Stream, r1 error) {
	n.missing("Logs")
	return r0, r1
}

func (n notStubbed) MetricHistory(_ context.Context, _, _ string, _ time.Duration) (r0 api.MetricHistory, r1 error) {
	n.missing("MetricHistory")
	return r0, r1
}

func (n notStubbed) Metrics(_ context.Context) (r0 api.Metrics) {
	n.missing("Metrics")
	return r0
}

func (n notStubbed) Namespaces(_ context.Context) (r0 api.Namespaces) {
	n.missing("Namespaces")
	return r0
}

func (n notStubbed) NodeShellSupport(_ context.Context, _ string) (r0 api.NodeShellSupport) {
	n.missing("NodeShellSupport")
	return r0
}

func (n notStubbed) Object(_ context.Context, _ api.ObjectRef) (r0 api.ObjectDetail, r1 error) {
	n.missing("Object")
	return r0, r1
}

func (n notStubbed) Overview(_ context.Context) (r0 api.ClusterOverview) {
	n.missing("Overview")
	return r0
}

func (n notStubbed) Ping(_ context.Context) (r0 error) {
	n.missing("Ping")
	return r0
}

func (n notStubbed) PodSelector(_ context.Context, _ api.ObjectRef) (r0 string, r1 error) {
	n.missing("PodSelector")
	return r0, r1
}

func (n notStubbed) Record(_ context.Context, _ resources.Timeline, _ []resources.Kind) {
	n.missing("Record")
}

func (n notStubbed) StopRecording() {
	n.missing("StopRecording")
}

func (n notStubbed) RBACIndex(_ context.Context) (r0 rbac.Index) {
	n.missing("RBACIndex")
	return r0
}

func (n notStubbed) Reach() (r0 *reach.Sink) {
	n.missing("Reach")
	return r0
}

func (n notStubbed) RefreshResources() (r0 api.ResourceCatalog) {
	n.missing("RefreshResources")
	return r0
}

func (n notStubbed) RemoveNodeShell(_ context.Context, _ string) error {
	n.missing("RemoveNodeShell")
	return nil
}

func (n notStubbed) Resources() (r0 api.ResourceCatalog) {
	n.missing("Resources")
	return r0
}

func (n notStubbed) Schema(_ context.Context, _ jsonschema.GVK) (r0 json.RawMessage, r1 error) {
	n.missing("Schema")
	return r0, r1
}

func (n notStubbed) Scope(_ context.Context) (r0 api.Scope) {
	n.missing("Scope")
	return r0
}

func (n notStubbed) Search(_ context.Context, _ string) (r0 api.SearchResults) {
	n.missing("Search")
	return r0
}

func (n notStubbed) StartDebug(_ context.Context, _ debugcontainer.Request) (r0 api.DebugSession, r1 error) {
	n.missing("StartDebug")
	return r0, r1
}

func (n notStubbed) StartExec(_ context.Context, _ exec.Request, _ io.Writer) (r0 *exec.Session, r1 error) {
	n.missing("StartExec")
	return r0, r1
}

func (n notStubbed) StartForward(_ context.Context, _ portforward.Target, _ int32) (r0 api.PortForward, r1 error) {
	n.missing("StartForward")
	return r0, r1
}

func (n notStubbed) StartNodeShell(_ context.Context, _ string) (r0 api.NodeShellSession, r1 error) {
	n.missing("StartNodeShell")
	return r0, r1
}

func (n notStubbed) StopForward(_ string) (r0 error) {
	n.missing("StopForward")
	return r0
}

func (n notStubbed) Subscribe(_ context.Context, _, _, _, _ string, _ int, _ []api.RowFilter) (r0 *resources.Subscription, r1 error) {
	n.missing("Subscribe")
	return r0, r1
}

func (n notStubbed) Topology(_ context.Context, _ topology.Request) (r0 api.Graph) {
	n.missing("Topology")
	return r0
}

func (n notStubbed) TrafficGraph(_ context.Context) (r0 api.TrafficGraph) {
	n.missing("TrafficGraph")
	return r0
}

func (n notStubbed) TrafficSupport(_ context.Context) (r0 api.TrafficSupport) {
	n.missing("TrafficSupport")
	return r0
}
