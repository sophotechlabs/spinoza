package resources

import (
	"context"
	"errors"
	"io"
	"maps"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	kubediscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

type stubImages struct {
	digest string
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

// what a manager says when a capability was never wired up

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

func TestMetricHistorySaysPrometheusIsUnavailable(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.MetricHistory(context.Background(), "prod", "web-0", time.Hour)

	if !errors.Is(err, prom.ErrUnavailable) {
		t.Fatalf("error = %v, want the unavailable error", err)
	}
}

// what a manager does once the capability is there

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
		Debugger:    debugcontainer.NewService(stubRunner{}, cs, "", api.ContextRef{}),
	})

	result := mgr.DebugSupport(ctx, "prod", "web-0")

	if result.Reason == debugcontainer.ErrUnavailable.Error() {
		t.Fatalf("support = %+v, want an answer from the service rather than the wiring guard", result)
	}
}

// actions, which the manager builds per call

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

// the dashboards the manager assembles for the views

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

// the cluster facts the flux overview leans on

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
		Debugger:    debugcontainer.NewService(stubRunner{}, cs, "", api.ContextRef{}),
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
