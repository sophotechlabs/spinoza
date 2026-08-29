package mcp

import (
	"context"
	"errors"
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

type fakeCluster struct {
	overview  api.ClusterOverview
	counts    api.ResourceCounts
	queue     api.IssueQueue
	report    api.CheckReport
	catalog   api.ResourceCatalog
	spaces    api.Namespaces
	listed    []*unstructured.Unstructured
	listErr   error
	detail    api.ObjectDetail
	detailErr error
	events    []api.Event
	eventsErr error
	hits      api.SearchResults
	graph     api.Graph
	usage     api.Metrics
	lines     []string
	linesErr  error
	selector  string
	selErr    error
	releases  api.HelmReleases
	relErr    error
	release   api.HelmReleaseDetail
	oneRelErr error
	refused   api.Access
	acted     []actions.Request
	actResult api.ActionResult
	actErr    error
	applied   []byte
	applyErr  error
	fluxCalls []flux.Action
	argoCalls []argocd.Request
	gitopsErr error
	lastKind  api.ObjectRef
	lastTopo  topology.Request
	lastLogs  logs.Request
}

func (f *fakeCluster) Overview(context.Context) api.ClusterOverview          { return f.overview }
func (f *fakeCluster) Counts(context.Context) api.ResourceCounts             { return f.counts }
func (f *fakeCluster) Issues(context.Context) api.IssueQueue                 { return f.queue }
func (f *fakeCluster) Checks(context.Context, checks.Filter) api.CheckReport { return f.report }
func (f *fakeCluster) Resources() api.ResourceCatalog                        { return f.catalog }
func (f *fakeCluster) Namespaces(context.Context) api.Namespaces             { return f.spaces }

func (f *fakeCluster) ListKind(_ context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error) {
	f.lastKind = ref
	return f.listed, f.listErr
}

func (f *fakeCluster) Object(context.Context, api.ObjectRef) (api.ObjectDetail, error) {
	return f.detail, f.detailErr
}

func (f *fakeCluster) Events(context.Context, string, string) ([]api.Event, error) {
	return f.events, f.eventsErr
}

func (f *fakeCluster) Search(context.Context, string) api.SearchResults { return f.hits }

func (f *fakeCluster) Topology(_ context.Context, req topology.Request) api.Graph {
	f.lastTopo = req
	return f.graph
}

func (f *fakeCluster) Metrics(context.Context) api.Metrics { return f.usage }

func (f *fakeCluster) LogLines(_ context.Context, req logs.Request) ([]string, error) {
	f.lastLogs = req
	return f.lines, f.linesErr
}

func (f *fakeCluster) PodSelector(context.Context, api.ObjectRef) (string, error) {
	return f.selector, f.selErr
}

func (f *fakeCluster) HelmReleases(context.Context) (api.HelmReleases, error) {
	return f.releases, f.relErr
}

func (f *fakeCluster) HelmRelease(context.Context, string, string) (api.HelmReleaseDetail, error) {
	return f.release, f.oneRelErr
}

func (f *fakeCluster) Access(context.Context, api.ObjectRef) api.Access { return f.refused }

func (f *fakeCluster) Action(_ context.Context, req actions.Request) (api.ActionResult, error) {
	f.acted = append(f.acted, req)
	return f.actResult, f.actErr
}

func (f *fakeCluster) ApplyObject(_ context.Context, _ api.ObjectRef, doc []byte) (api.ObjectDetail, error) {
	f.applied = doc
	return f.detail, f.applyErr
}

func (f *fakeCluster) FluxAction(_ context.Context, _ api.ObjectRef, action flux.Action) (api.FluxActionResult, error) {
	f.fluxCalls = append(f.fluxCalls, action)
	return api.FluxActionResult{}, f.gitopsErr
}

func (f *fakeCluster) ArgoAction(_ context.Context, _ api.ObjectRef, req argocd.Request) (api.ArgoActionResult, error) {
	f.argoCalls = append(f.argoCalls, req)
	return api.ArgoActionResult{}, f.gitopsErr
}

type fakeProm struct {
	samples []prom.Sample
	err     error
	asked   string
}

func (p *fakeProm) Instant(_ context.Context, query string, _ time.Time) ([]prom.Sample, error) {
	p.asked = query
	return p.samples, p.err
}

var errRefused = errors.New("the apiserver refused")

func catalogOf(descs ...api.ResourceDescriptor) api.ResourceCatalog {
	return api.ResourceCatalog{Categories: []api.Category{{Name: "Workloads", Resources: descs}}}
}

func descriptor(group, version, resource, kind string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      group,
		Version:    version,
		Resource:   resource,
		Kind:       kind,
		Namespaced: true,
	}
}

func serverFor(cluster Cluster, opts Options) *Server {
	if opts.Version == "" {
		opts.Version = "v0.0.0-test"
	}
	if opts.Context == "" {
		opts.Context = "kind-test"
	}
	return New(cluster, opts)
}
