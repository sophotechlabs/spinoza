package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type nodeShellBackend struct {
	notStubbed

	calls int
}

func (b *nodeShellBackend) StartNodeShell(context.Context, string) (api.NodeShellSession, error) {
	b.calls++
	return api.NodeShellSession{}, nil
}

func TestANodeShellOnAProtectedClusterNeedsTheNodeNameTyped(t *testing.T) {
	backend := &nodeShellBackend{}
	ts := protectedServer(t, backend)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/nodeshell?node=p-mk2", http.NoBody)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; a node shell puts a privileged pod on the cluster", resp.StatusCode)
	}
	if !strings.Contains(string(body), "protected") || !strings.Contains(string(body), "p-mk2") {
		t.Fatalf("body = %s, want the rule and the name to type", body)
	}
	if backend.calls != 0 {
		t.Fatal("the shell pod was created anyway")
	}
}

func TestTheNodeShellRefusalArrivesBeforeTheSocketIsUpgraded(t *testing.T) {
	ts := protectedServer(t, &nodeShellBackend{})

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/nodeshell?node=p-mk2", http.NoBody)

	if resp.Header.Get("Upgrade") != "" {
		t.Fatalf("Upgrade = %q, want the refusal to be a plain response a browser can read",
			resp.Header.Get("Upgrade"))
	}
}

type deleteBackend struct {
	notStubbed

	started chan struct{}
	release chan struct{}
	seen    chan error
}

func newDeleteBackend() *deleteBackend {
	return &deleteBackend{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		seen:    make(chan error, 1),
	}
}

func (b *deleteBackend) DeleteObject(ctx context.Context, _ api.ObjectRef) error {
	b.started <- struct{}{}
	<-b.release
	b.seen <- ctx.Err()
	return nil
}

func TestAnAbandonedDeleteStillFinishesTheMutation(t *testing.T) {
	backend := newDeleteBackend()
	handler := authed(New(&stubBackendCluster{backend: backend}, testAssets(), testToken).Handler())

	req := httptest.NewRequest(http.MethodDelete,
		"/api/object?group=&version=v1&resource=configmaps&namespace=demo&name=old", http.NoBody)
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
		t.Fatalf("the delete ran on a canceled context (%v); a browser that gives up mid-request "+
			"leaves the object half-removed and the audit row saying it failed", err)
	}
	<-served
}
