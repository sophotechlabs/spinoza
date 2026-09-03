package resources

import (
	"context"
	"errors"
	"io"
	"maps"
	"strings"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	kubediscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/reach"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

type stubImages struct {
	digest string
}

func TestManagerTopologyReadsTheKnownResources(t *testing.T) {
	manager, cancel := newManager(t, newClient(t, newDeployment("prod", "web")))
	defer cancel()

	graph := manager.Topology(t.Context(), topology.Request{Namespace: "prod"})

	if graph.Error != "" {
		t.Fatalf("graph error = %q", graph.Error)
	}
	if len(graph.Nodes) != 1 {
		t.Fatalf("nodes = %d, want the deployment", len(graph.Nodes))
	}
	if graph.Nodes[0].Name != "web" || graph.Nodes[0].Namespace != "prod" {
		t.Fatalf("node = %+v", graph.Nodes[0])
	}
}

func TestManagerReturnsItsReachabilitySink(t *testing.T) {
	sink := reach.New()
	manager := NewManager(t.Context(), Deps{Reach: sink})

	if manager.Reach() != sink {
		t.Fatal("manager returned a different reachability sink")
	}
}

func (s stubImages) ImageID(context.Context, exec.Request) (string, error) {
	return s.digest, nil
}

type stubStreamer struct{}

func (stubStreamer) Stream(context.Context, exec.Request, exec.Options) error {
	return nil
}

type refusingDiscovery struct {
	kubediscovery.CachedDiscoveryInterface
}

func (refusingDiscovery) ServerVersion() (*version.Info, error) {
	return nil, errors.New("the apiserver did not answer")
}

type stubRunner struct{}

func (stubRunner) Run(context.Context, []string) error {
	return nil
}

func TestExecSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.ExecSupport(context.Background(), exec.Request{Namespace: "prod", Pod: "web-0"})

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal error", err)
	}
	want := "spinoza could not do that: exec is not wired up"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestStartingAShellSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	session, err := mgr.StartExec(context.Background(), exec.Request{Namespace: "prod", Pod: "web-0"}, io.Discard)

	if session != nil {
		t.Fatal("session is not nil, want nothing to start without a shell service")
	}
	want := "spinoza could not do that: exec is not wired up"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestDebugSupportSaysWhyItIsUnavailable(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	result := mgr.DebugSupport(context.Background(), "prod", "web-0")

	expected := api.DebugSupport{
		Namespace: "prod",
		Pod:       "web-0",
		Allowed:   false,
		Reason:    debugcontainer.ErrUnavailable.Error(),
	}
	if result != expected {
		t.Fatalf("support = %+v, want %+v", result, expected)
	}
}

func TestStartingADebugContainerSaysItIsUnavailable(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.StartDebug(context.Background(), debugcontainer.Request{Namespace: "prod", Pod: "web-0"})

	if !errors.Is(err, debugcontainer.ErrUnavailable) {
		t.Fatalf("error = %v, want the unavailable error", err)
	}
}

func TestMetricHistoryOnAClusterMeasuringNothingIsEmptyNotAnError(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	history, err := mgr.MetricHistory(context.Background(), "prod", "web-0", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !history.Sampled {
		t.Fatal("the answer did not say where it came from")
	}
	if len(history.CPU) != 0 {
		t.Fatalf("cpu points = %d, want none from a cluster measuring nothing", len(history.CPU))
	}
}

func TestExecSupportComesFromTheShellService(t *testing.T) {
	cs := k8sfake.NewClientset()
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Descriptors: testDescs(),
		Shells:      exec.NewService(stubStreamer{}, stubImages{digest: "sha256:abc"}),
	})

	support, err := mgr.ExecSupport(ctx, exec.Request{Namespace: "prod", Pod: "web-0"})
	if err != nil {
		t.Fatalf("ExecSupport: %v", err)
	}
	if support.Namespace != "prod" || support.Pod != "web-0" {
		t.Fatalf("support = %+v, want it to answer about prod/web-0", support)
	}
}

func TestDebugSupportComesFromTheDebugService(t *testing.T) {
	cs := k8sfake.NewClientset()
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Descriptors: testDescs(),
		Debugger:    debugcontainer.NewService(stubRunner{}, cs, "", api.ContextRef{}, access.New(cs)),
	})

	result := mgr.DebugSupport(ctx, "prod", "web-0")

	if result.Reason == debugcontainer.ErrUnavailable.Error() {
		t.Fatalf("support = %+v, want an answer from the service rather than the wiring guard", result)
	}
}

func TestActionScalesThroughTheManager(t *testing.T) {
	dyn := newClient(t, newDeployment("default", "web"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()
	request := actions.Request{
		Ref:      api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default", Name: "web"},
		Action:   actions.Scale,
		Replicas: 3,
	}

	result, err := mgr.Action(context.Background(), request)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if result.Action != string(actions.Scale) {
		t.Fatalf("action = %q, want %q", result.Action, actions.Scale)
	}
	scaled, getErr := dyn.Resource(depGVR).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("get: %v", getErr)
	}
	replicas, found, fieldErr := unstructured.NestedInt64(scaled.Object, "spec", "replicas")
	if fieldErr != nil || !found {
		t.Fatalf("replicas not readable: found=%v err=%v", found, fieldErr)
	}
	if replicas != 3 {
		t.Fatalf("replicas = %d, want 3", replicas)
	}
}

func TestFluxOverviewIsBuiltForAClusterWithoutFlux(t *testing.T) {
	scheme := runtime.NewScheme()
	kinds := listKinds()
	maps.Copy(kinds, metricsKinds())
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, newNode("node-1"))
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Millisecond},
	})

	overview := mgr.FluxOverview(ctx)

	if len(overview.Controllers) != 0 {
		t.Fatalf("controllers = %d, want none on a cluster without flux", len(overview.Controllers))
	}
	if overview.Nodes != 1 {
		t.Fatalf("nodes = %d, want the one node in the cache", overview.Nodes)
	}
}

func TestArgoDashboardIsBuiltForAClusterWithoutArgo(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	dashboard := mgr.Argo(context.Background())

	if len(dashboard.Apps) != 0 {
		t.Fatalf("apps = %d, want none on a cluster without argo", len(dashboard.Apps))
	}
}

func TestServerVersionIsEmptyWithoutDiscovery(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	if got := mgr.serverVersion(); got != "" {
		t.Fatalf("version = %q, want empty without discovery", got)
	}
}

func TestServerVersionComesFromDiscovery(t *testing.T) {
	cs := k8sfake.NewClientset()
	faked, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		t.Fatal("the fake clientset did not hand back a fake discovery")
	}
	faked.FakedServerVersion = &version.Info{GitVersion: "v1.33.1"}
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	mgr.UseDiscovery(memory.NewMemCacheClient(faked), nil)

	if got := mgr.serverVersion(); got != "v1.33.1" {
		t.Fatalf("version = %q, want v1.33.1", got)
	}
}

func TestNodeCountIsZeroWhenTheKindIsNotDiscovered(t *testing.T) {
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   k8sfake.NewClientset(),
		Descriptors: map[string]api.ResourceDescriptor{},
	})

	if got := mgr.nodeCount(ctx); got != 0 {
		t.Fatalf("nodes = %d, want 0 when nodes were never discovered", got)
	}
}

func TestNodeCountReadsTheWarmedCache(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newNode("node-1"), newNode("node-2")))
	defer cancel()

	if got := mgr.nodeCount(context.Background()); got != 2 {
		t.Fatalf("nodes = %d, want 2", got)
	}
}

func TestStartingAShellGoesToTheShellService(t *testing.T) {
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   k8sfake.NewClientset(),
		Descriptors: testDescs(),
		Shells:      exec.NewService(stubStreamer{}, stubImages{digest: "sha256:abc"}),
	})

	session, err := mgr.StartExec(ctx, exec.Request{Namespace: "prod", Pod: "web-0", Container: "app"}, io.Discard)
	if err != nil {
		t.Fatalf("StartExec: %v", err)
	}
	if session == nil {
		t.Fatal("session is nil, want the one the service opened")
	}
	session.Close()
}

func TestStartingADebugContainerGoesToTheDebugService(t *testing.T) {
	cs := k8sfake.NewClientset()
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Descriptors: testDescs(),
		Debugger:    debugcontainer.NewService(stubRunner{}, cs, "", api.ContextRef{}, access.New(cs)),
	})

	_, err := mgr.StartDebug(ctx, debugcontainer.Request{Namespace: "prod", Pod: "web-0"})

	if errors.Is(err, debugcontainer.ErrUnavailable) {
		t.Fatal("error = unavailable, want the service to have been asked")
	}
}

func TestServerVersionIsEmptyWhenDiscoveryRefuses(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	mgr.UseDiscovery(refusingDiscovery{}, nil)

	if got := mgr.serverVersion(); got != "" {
		t.Fatalf("version = %q, want empty when discovery refuses", got)
	}
}

func TestNodeCountIsZeroWhenTheCacheCannotBeRead(t *testing.T) {
	dyn := newClient(t)
	dyn.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("nodes are forbidden")
	})
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Descriptors: testDescs(),
		Limits:      Limits{SyncTimeout: 50 * time.Millisecond, IdleGrace: time.Millisecond},
	})

	if got := mgr.nodeCount(ctx); got != 0 {
		t.Fatalf("nodes = %d, want 0 when the cache could not be read", got)
	}
}

func TestHelmReleasesSayTheyAreNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.HelmReleases(context.Background())

	want := "spinoza could not do that: helm is not wired up"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestHelmHistorySaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.HelmHistory(t.Context(), "demo", "podinfo", 7)

	want := "spinoza could not do that: helm is not wired up"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

type stubCacheLister struct {
	objects []runtime.Object
	err     error
}

func (s stubCacheLister) List(labels.Selector) ([]runtime.Object, error) {
	return s.objects, s.err
}

func (s stubCacheLister) Get(string) (runtime.Object, error) {
	return nil, errors.New("not used")
}

func (s stubCacheLister) ByNamespace(string) cache.GenericNamespaceLister {
	return nil
}

func TestAnUnreadableCacheCountsNothingAsFailing(t *testing.T) {
	entry := watchedType{kind: "Kustomization", lister: stubCacheLister{err: errors.New("cache is gone")}}

	if got := failingInCache(entry); got != 0 {
		t.Fatalf("failing = %d, want 0 when the cache could not be read", got)
	}
}

func TestOnlyRealObjectsInTheCacheAreJudged(t *testing.T) {
	notReady := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"status": map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": "False"}},
		},
	}}
	entry := watchedType{
		kind:   "Kustomization",
		lister: stubCacheLister{objects: []runtime.Object{notReady, &metav1.Status{}}},
	}

	if got := failingInCache(entry); got != 1 {
		t.Fatalf("failing = %d, want the one unready object", got)
	}
}

func TestNonPodStreamsAreWatchedForFailures(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "default", 0, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	watched := mgr.watchedTypes()

	if _, held := watched["apps/v1/deployments"]; !held {
		t.Fatalf("watched = %v, want the deployment stream", watched)
	}
}

func TestPodStreamsAreLeftToThePodCounter(t *testing.T) {
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	pod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "web-0",
			"namespace": "default",
		},
	}}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
		pod,
	)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	mgr := NewManager(ctx, Deps{
		Dynamic:   dyn,
		Clientset: k8sfake.NewClientset(),
		Descriptors: map[string]api.ResourceDescriptor{
			discovery.Key("", "v1", "pods"): {
				Version:    "v1",
				Resource:   "pods",
				Kind:       "Pod",
				Namespaced: true,
			},
		},
	})
	sub, err := mgr.Subscribe(t.Context(), "", "v1", "pods", "default", 0, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if watched := mgr.watchedTypes(); len(watched) != 0 {
		t.Fatalf("watched = %v, want pods excluded from the cache failure tally", watched)
	}
}

func TestRefreshingWithoutDiscoveryHandsBackWhatIsKnown(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	catalog := mgr.RefreshResources()

	if len(catalog.Categories) != 1 {
		t.Fatalf("categories = %d, want the ones already known", len(catalog.Categories))
	}
}

func TestAnEmptyDiscoveryCarriesTheReasonWhenThereIsOne(t *testing.T) {
	withReason := emptyDiscovery(errors.New("the apiserver refused"))
	if !strings.Contains(withReason.Error(), "refused") {
		t.Fatalf("error = %q, want the reason kept", withReason.Error())
	}
	bare := emptyDiscovery(nil)
	if strings.Contains(bare.Error(), "refused") {
		t.Fatalf("error = %q, want no reason invented", bare.Error())
	}
}

func TestWatchedFailuresDoNotOverwriteCountedOnes(t *testing.T) {
	counted := api.ResourceCounts{
		Counts:  map[string]int{"/v1/pods": 3},
		Failing: map[string]int{"/v1/pods": 1},
	}

	merged := withWatched(counted, map[string]int{
		"/v1/pods": 9,
		"kustomize.toolkit.fluxcd.io/v1/kustomizations": 2,
	})

	if merged.Failing["/v1/pods"] != 1 {
		t.Fatalf("pods = %d, want the counted answer kept", merged.Failing["/v1/pods"])
	}
	if merged.Failing["kustomize.toolkit.fluxcd.io/v1/kustomizations"] != 2 {
		t.Fatalf("kustomizations = %d, want the watched answer added", merged.Failing["kustomize.toolkit.fluxcd.io/v1/kustomizations"])
	}
}

func TestCountsComeBackEmptyOnceTheCallerHasGoneAway(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	stop()

	counts := mgr.Counts(ctx)

	if len(counts.Counts) != 0 {
		t.Fatalf("counts = %v, want nothing once the caller has gone", counts.Counts)
	}
}

func TestVersionsAreUnknownWithoutDiscovery(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	if got := mgr.versions(); got != nil {
		t.Fatalf("versions = %v, want nothing without discovery", got)
	}
}

func TestASchemaSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.Schema(context.Background(), jsonschema.GVK{Group: "apps", Version: "v1", Kind: "Deployment"})

	want := "spinoza could not do that: schemas are not wired up"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestForwardingSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, startErr := mgr.StartForward(context.Background(), portforward.Target{Namespace: "prod", Name: "web"}, 8080)
	if startErr == nil {
		t.Fatal("StartForward returned nil error with no registry")
	}
	if len(mgr.Forwards()) != 0 {
		t.Fatal("Forwards listed something with no registry")
	}
	if stopErr := mgr.StopForward("pf-1"); stopErr == nil {
		t.Fatal("StopForward returned nil error with no registry")
	}
}

func TestAnEventObjectWithNoKindIsJustTheName(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"involvedObject": map[string]any{"name": "web-0"},
	}}

	if got := eventObject(item); got != "web-0" {
		t.Fatalf("object = %q, want just the name", got)
	}
}

func TestHelmReleasesAreOnlyLookedForInTheFluxKind(t *testing.T) {
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   k8sfake.NewClientset(),
		Descriptors: testDescs(),
		Limits:      Limits{SyncTimeout: 50 * time.Millisecond, IdleGrace: time.Millisecond},
	})

	owners := mgr.fluxOwners(ctx)

	if len(owners) != 0 {
		t.Fatalf("owners = %v, want none when the flux kind was never discovered", owners)
	}
}

func TestTheChartEndpointsSayTheyAreNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	want := "spinoza could not do that: helm is not wired up"

	_, searchErr := mgr.HelmChartSearch(context.Background(), "podinfo")
	_, valuesErr := mgr.HelmChartValues(context.Background(), helm.ValuesRequest{Chart: "podinfo"})
	_, installErr := mgr.HelmInstall(context.Background(), helm.InstallRequest{Name: "podinfo"})

	for name, err := range map[string]error{
		"search":  searchErr,
		"values":  valuesErr,
		"install": installErr,
	} {
		if err == nil || err.Error() != want {
			t.Fatalf("%s error = %v, want %q", name, err, want)
		}
	}
}

func TestNodeShellsSayTheyAreNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	support := mgr.NodeShellSupport(context.Background(), "p-mk1")
	_, startErr := mgr.StartNodeShell(context.Background(), "p-mk1")

	if support.Enabled || support.Node != "p-mk1" {
		t.Fatalf("support = %+v, want it off but still about the node asked for", support)
	}
	if !strings.Contains(support.Reason, "not wired up") {
		t.Fatalf("reason = %q", support.Reason)
	}
	want := "spinoza could not do that: node shells are not wired up"
	if startErr == nil || startErr.Error() != want {
		t.Fatalf("error = %v, want %q", startErr, want)
	}
}

func TestRemovingANodeShellThatWasNeverWiredUpIsQuiet(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	if err := mgr.RemoveNodeShell(context.Background(), "spinoza-node-shell-abc"); err != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestAnArgoActionReachesTheObjectItNames(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": "web", "namespace": "argocd"},
	}}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
	}, app)
	mgr, cancel := newManager(t, dyn)
	defer cancel()
	ref := api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: "argocd",
		Name:      "web",
	}

	result, err := mgr.ArgoAction(context.Background(), ref, argocd.Request{Action: argocd.Refresh})
	if err != nil {
		t.Fatalf("argo action: %v", err)
	}

	if result.Action != string(argocd.Refresh) {
		t.Fatalf("result = %+v, want the refresh it was asked for", result)
	}
	live, getErr := dyn.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).Namespace("argocd").Get(context.Background(), "web", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("get: %v", getErr)
	}
	annotations := live.GetAnnotations()
	if _, ok := annotations["argocd.argoproj.io/refresh"]; !ok {
		t.Fatalf("annotations = %v, want the refresh argo watches for", annotations)
	}
}

func shellClientset(t *testing.T) *k8sfake.Clientset {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		pod, ok := create.GetObject().(*corev1.Pod)
		if !ok {
			return false, nil, nil
		}
		pod.Name = "spinoza-node-shell-abc"
		pod.Status.Phase = corev1.PodRunning
		return false, pod, nil
	})
	return cs
}

func managerWithNodeShells(t *testing.T, cs *k8sfake.Clientset) (*Manager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(ctx, Deps{
		Dynamic:   newClient(t),
		Clientset: cs,
		NodeShells: nodeshell.NewService(
			cs,
			"busybox:1.37",
			nodeshell.DefaultNamespace,
			func() bool { return true },
			access.New(cs),
		),
		Limits: Limits{IdleGrace: time.Millisecond},
	})
	return mgr, cancel
}

func TestTheManagerHandsANodeShellToTheServiceThatRunsIt(t *testing.T) {
	cs := shellClientset(t)
	mgr, cancel := managerWithNodeShells(t, cs)
	defer cancel()

	support := mgr.NodeShellSupport(context.Background(), "p-mk1")
	session, err := mgr.StartNodeShell(context.Background(), "p-mk1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if !support.Enabled || !support.Allowed {
		t.Fatalf("support = %+v, want the service's own answer", support)
	}
	if session.Pod != "spinoza-node-shell-abc" || session.Node != "p-mk1" {
		t.Fatalf("session = %+v", session)
	}

	if err := mgr.RemoveNodeShell(context.Background(), session.Pod); err != nil {
		t.Fatalf("remove: %v", err)
	}

	left, listErr := cs.CoreV1().Pods(nodeshell.DefaultNamespace).List(context.Background(), metav1.ListOptions{})
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(left.Items) != 0 {
		t.Fatalf("pods = %d, want the removal passed on", len(left.Items))
	}
}

func TestTheManagerHandsChartWorkToHelm(t *testing.T) {
	ctx := t.Context()
	const repoURL = "https://charts.example.com"
	runner := &recordingRunner{out: "replicaCount: 1\n"}
	mgr := NewManager(ctx, Deps{
		Dynamic:   newClient(t),
		Clientset: k8sfake.NewClientset(),
		Helm: helm.NewService(
			k8sfake.NewClientset(), nil, runner, nil,
			[]helm.RepoEntry{{Repo: charts.Repo{URL: repoURL}}},
			api.ContextRef{Name: "kind-spinoza"},
		),
		Limits: Limits{IdleGrace: time.Millisecond},
	})

	search, searchErr := mgr.HelmChartSearch(ctx, "podinfo")
	values, valuesErr := mgr.HelmChartValues(ctx, helm.ValuesRequest{
		Chart:   "podinfo",
		Version: "6.14.1",
		RepoURL: repoURL,
	})
	if searchErr != nil || valuesErr != nil {
		t.Fatalf("search: %v, values: %v", searchErr, valuesErr)
	}

	if !strings.Contains(search.Error, "not wired up") {
		t.Fatalf("search = %+v, want the service's own answer without a chart index", search)
	}
	if values.Values != "replicaCount: 1\n" {
		t.Fatalf("values = %q, want what helm printed", values.Values)
	}
	if len(runner.args) == 0 || runner.args[0][0] != "show" {
		t.Fatalf("helm was run with %v, want show values", runner.args)
	}

	_, installErr := mgr.HelmInstall(ctx, helm.InstallRequest{
		Namespace: "demo",
		Name:      "greeter",
		Chart:     "podinfo",
		Version:   "6.14.1",
		RepoURL:   repoURL,
	})
	if installErr != nil {
		t.Fatalf("install: %v", installErr)
	}
	if runner.args[1][0] != "install" {
		t.Fatalf("helm was run with %v, want install", runner.args[1])
	}
}

type recordingRunner struct {
	args [][]string
	out  string
}

func (r *recordingRunner) Run(_ context.Context, args, _ []string) (string, error) {
	r.args = append(r.args, args)
	return r.out, nil
}

func (r *recordingRunner) Available() error {
	return nil
}
