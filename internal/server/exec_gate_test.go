package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type debugBackend struct {
	notStubbed

	started chan struct{}
	release chan struct{}
	seen    chan error
	calls   int
}

func newDebugBackend() *debugBackend {
	return &debugBackend{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		seen:    make(chan error, 1),
	}
}

func (b *debugBackend) StartDebug(ctx context.Context, _ debugcontainer.Request) (api.DebugSession, error) {
	b.calls++
	b.started <- struct{}{}
	<-b.release
	b.seen <- ctx.Err()
	return api.DebugSession{Container: "spinoza-debug-1", Created: true}, nil
}

func TestDebuggingAProtectedClusterNeedsThePodNameTyped(t *testing.T) {
	backend := newDebugBackend()
	ts := protectedServer(t, backend)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=demo&pod=probe&profile=general", http.NoBody)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; an ephemeral container cannot be taken back off a pod", resp.StatusCode)
	}
	if !strings.Contains(string(body), "protected") || !strings.Contains(string(body), "probe") {
		t.Fatalf("body = %s, want the rule and the name to type", body)
	}
	if backend.calls != 0 {
		t.Fatal("the request reached the cluster anyway")
	}
}

func TestDebuggingGoesAheadOnceThePodNameMatches(t *testing.T) {
	backend := newDebugBackend()
	close(backend.release)
	ts := protectedServer(t, backend)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=demo&pod=probe&confirm=probe", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the confirmed debug to go through: %s", resp.StatusCode, body)
	}
}

func TestAnAbandonedDebugRequestStillFinishesTheMutation(t *testing.T) {
	backend := newDebugBackend()
	handler := authed(New(&stubBackendCluster{backend: backend}, testAssets(), testToken).Handler())

	req := httptest.NewRequest(http.MethodPost, "/api/debug?namespace=demo&pod=probe&profile=general", http.NoBody)
	req.Host = "127.0.0.1:34131"
	ctx, abandon := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	served := make(chan struct{})
	go func() {
		defer close(served)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	<-backend.started
	abandon()
	close(backend.release)

	err := <-backend.seen
	if err != nil {
		t.Fatalf("the patch ran on a canceled context (%v); the container lands and the wait dies with the browser", err)
	}
	<-served
}

func drainableNodeShellServer(t *testing.T, cs *k8sfake.Clientset) (*httptest.Server, *Server) {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shell := newFakeShell()
	shell.greet = "/ # "
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:   dyn,
		Clientset: cs,
		Shells:    exec.NewService(shell, &fakeImages{digest: "sha256:shelled"}),
		NodeShells: nodeshell.NewService(
			cs,
			"busybox:1.37",
			nodeshell.DefaultNamespace,
			func() bool { return true },
			access.New(cs),
		),
	})
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, srv
}

func TestClosingTheServerTakesTheNodeShellPodWithIt(t *testing.T) {
	cs := shellCluster(t)
	deleting := make(chan struct{})
	hold := make(chan struct{})
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		close(deleting)
		<-hold
		return false, nil, nil
	})
	ts, srv := drainableNodeShellServer(t, cs)

	conn := dialNodeShell(t, ts, "?node=p-mk1")
	channel, payload := readFrame(t, conn)
	if channel != api.ExecChannelStdout || string(payload) != "/ # " {
		t.Fatalf("first frame = %d %q, want the shell greeting", channel, payload)
	}
	if shellPods(t, cs) != 1 {
		t.Fatalf("pods = %d, want the one the shell runs on", shellPods(t, cs))
	}

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		srv.Close()
	}()

	select {
	case <-deleting:
	case <-time.After(10 * time.Second):
		t.Fatal("closing the server never asked the cluster to remove the node shell pod")
	}
	if nodeShells.count() != 1 {
		t.Fatal("the removal in flight held nothing back; shutdown would race it and the pod would stay")
	}

	close(hold)
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close never returned")
	}
	if shellPods(t, cs) != 0 {
		t.Fatal("the process was about to exit with a privileged pod still running on the cluster")
	}
}

func TestTheShutdownDrainWaitsForARemovalStillInFlight(t *testing.T) {
	nodeShells.start()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		awaitNodeShells()
	}()

	select {
	case <-drained:
		t.Fatal("the drain returned while a node shell was still being taken down")
	case <-time.After(100 * time.Millisecond):
	}

	nodeShells.done()
	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("the drain never returned once the node shell was gone")
	}
}

func TestTheShutdownDrainGivesUpRatherThanHanging(t *testing.T) {
	was := nodeShellDrain
	nodeShellDrain = 50 * time.Millisecond
	t.Cleanup(func() { nodeShellDrain = was })

	nodeShells.start()
	t.Cleanup(nodeShells.done)

	done := make(chan struct{})
	go func() {
		defer close(done)
		awaitNodeShells()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a stuck removal wedged shutdown for good")
	}
}
