package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

type keptWriter struct {
	notStubbed

	started chan struct{}
	release chan struct{}
	seen    chan error
}

func newKeptWriter() *keptWriter {
	return &keptWriter{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		seen:    make(chan error, 1),
	}
}

func (b *keptWriter) hold(ctx context.Context) {
	b.started <- struct{}{}
	<-b.release
	b.seen <- ctx.Err()
}

func (b *keptWriter) Action(ctx context.Context, _ actions.Request) (api.ActionResult, error) {
	b.hold(ctx)
	return api.ActionResult{}, nil
}

func (b *keptWriter) ApplyObject(ctx context.Context, _ api.ObjectRef, _ []byte) (api.ObjectDetail, error) {
	b.hold(ctx)
	return api.ObjectDetail{}, nil
}

func (b *keptWriter) DeleteObject(ctx context.Context, _ api.ObjectRef) error {
	b.hold(ctx)
	return nil
}

func (b *keptWriter) FluxAction(
	ctx context.Context,
	_ api.ObjectRef,
	_ flux.Action,
) (api.FluxActionResult, error) {
	b.hold(ctx)
	return api.FluxActionResult{}, nil
}

func (b *keptWriter) ArgoAction(
	ctx context.Context,
	_ api.ObjectRef,
	_ argocd.Request,
) (api.ArgoActionResult, error) {
	b.hold(ctx)
	return api.ArgoActionResult{}, nil
}

func (b *keptWriter) HelmUninstall(ctx context.Context, _, _ string) (api.HelmActionResult, error) {
	b.hold(ctx)
	return api.HelmActionResult{}, nil
}

func (b *keptWriter) HelmUpgrade(ctx context.Context, _ helm.UpgradeRequest) (api.HelmActionResult, error) {
	b.hold(ctx)
	return api.HelmActionResult{}, nil
}

type heldInstaller struct {
	started chan struct{}
	release chan struct{}
	seen    chan error
}

func (i *heldInstaller) Install(ctx context.Context) error {
	i.started <- struct{}{}
	<-i.release
	i.seen <- ctx.Err()
	return nil
}

func abandonMidRequest(
	t *testing.T,
	handler http.Handler,
	method, target string,
	body io.Reader,
	started, release chan struct{},
	seen chan error,
) error {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.Host = "127.0.0.1:34131"
	ctx, abandon := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	served := make(chan struct{})
	go func() {
		defer close(served)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	<-started
	abandon()
	close(release)
	err := <-seen
	<-served
	return err
}

func TestAnAbandonedRequestStillFinishesTheMutation(t *testing.T) {
	cases := []struct {
		family string
		method string
		target string
		body   string
	}{
		{
			"action",
			http.MethodPost,
			"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=demo&name=web",
			noBody,
		},
		{"apply", http.MethodPut, "/api/object?version=v1&resource=configmaps&namespace=demo&name=old", "{}"},
		{"delete", http.MethodDelete, "/api/object?version=v1&resource=configmaps&namespace=demo&name=old", noBody},
		{
			"flux",
			http.MethodPost,
			"/api/flux/action?action=reconcile&group=kustomize.toolkit.fluxcd.io&version=v1" +
				"&resource=kustomizations&namespace=flux-system&name=apps",
			noBody,
		},
		{
			"argo",
			http.MethodPost,
			"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1" +
				"&resource=applications&namespace=argocd&name=shop",
			noBody,
		},
		{"helm", http.MethodPost, "/api/helm/action?action=uninstall&namespace=demo&name=podinfo", noBody},
	}
	for _, one := range cases {
		t.Run(one.family, func(t *testing.T) {
			backend := newKeptWriter()
			handler := authed(New(&stubBackendCluster{backend: backend}, testAssets(), testToken).Handler())

			err := abandonMidRequest(
				t, handler, one.method, one.target, strings.NewReader(one.body),
				backend.started, backend.release, backend.seen,
			)
			if err != nil {
				t.Fatalf("the %s ran on a canceled context (%v); a browser that gives up mid-request leaves "+
					"the cluster half-changed and the audit row saying it failed", one.family, err)
			}
		})
	}
}

func TestAnAbandonedUpgradeStillFinishesTheMutation(t *testing.T) {
	backend := newKeptWriter()
	handler := authed(New(&stubBackendCluster{backend: backend}, testAssets(), testToken).Handler())

	err := abandonMidRequest(
		t, handler, http.MethodPost, "/api/helm/upgrade", strings.NewReader(helmDoc),
		backend.started, backend.release, backend.seen,
	)
	if err != nil {
		t.Fatalf("the upgrade ran on a canceled context (%v); helm would be left holding a pending release", err)
	}
}

func TestAnAbandonedUpdateStillFinishesTheInstall(t *testing.T) {
	installer := &heldInstaller{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		seen:    make(chan error, 1),
	}
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseUpdates(newerRelease())
	srv.UseInstaller(installer)
	handler := authed(srv.Handler())

	err := abandonMidRequest(
		t, handler, http.MethodPost, "/api/update", http.NoBody,
		installer.started, installer.release, installer.seen,
	)
	if err != nil {
		t.Fatalf("the install ran on a canceled context (%v); the binary is replaced half way", err)
	}
}
