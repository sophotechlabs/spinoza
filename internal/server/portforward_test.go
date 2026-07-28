package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type fakeForwardRunner struct {
	local int32
	err   error
}

func (f *fakeForwardRunner) Run(_ context.Context, _ string, _ string, _ int32, ready chan<- int32, stop <-chan struct{}) error {
	if f.err != nil {
		return f.err
	}
	ready <- f.local
	<-stop
	return nil
}

type fakeForwardResolver struct {
	err error
}

func (f *fakeForwardResolver) Resolve(_ context.Context, target portforward.Target, port int32) (string, int32, error) {
	if f.err != nil {
		return "", 0, f.err
	}
	return target.Name, port, nil
}

const forwardQuery = "?kind=Pod&namespace=flux-system&name=web&port=8080"

func forwardServer(t *testing.T, runner portforward.Runner, resolver portforward.Resolver) *httptest.Server {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var registry *portforward.Registry
	if runner != nil {
		registry = portforward.NewRegistry(ctx, runner, resolver, nil)
		t.Cleanup(registry.StopAll)
	}
	mgr := resources.NewManager(ctx, dyn, k8sfake.NewClientset(), nil, registry, nil, nil, nil)
	ts := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func decodeForward(t *testing.T, body []byte) api.PortForward {
	t.Helper()
	var forward api.PortForward
	if err := json.Unmarshal(body, &forward); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return forward
}

func decodeForwards(t *testing.T, body []byte) []api.PortForward {
	t.Helper()
	var forwards []api.PortForward
	if err := json.Unmarshal(body, &forwards); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return forwards
}

func TestStartForwardReturnsTheAssignedLocalPort(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 45123}, &fakeForwardResolver{})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+forwardQuery, nil)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", resp.StatusCode, body)
	}
	forward := decodeForward(t, body)
	if forward.LocalPort != 45123 {
		t.Fatalf("localPort = %d", forward.LocalPort)
	}
	if forward.RemotePort != 8080 {
		t.Fatalf("remotePort = %d", forward.RemotePort)
	}
	if forward.State != portforward.StateRunning {
		t.Fatalf("state = %q", forward.State)
	}
}

func TestListForwards(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 45123}, &fakeForwardResolver{})
	doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+forwardQuery, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/portforward", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	forwards := decodeForwards(t, body)
	if len(forwards) != 1 {
		t.Fatalf("forwards = %v", forwards)
	}
	if forwards[0].Name != "web" {
		t.Fatalf("name = %q", forwards[0].Name)
	}
}

func TestStopForward(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 45123}, &fakeForwardResolver{})
	_, created := doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+forwardQuery, nil)
	forward := decodeForward(t, created)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/portforward?id="+forward.ID, nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	_, listed := doRequest(t, http.MethodGet, ts.URL+"/api/portforward", nil)
	if len(decodeForwards(t, listed)) != 0 {
		t.Fatalf("forward survived the delete")
	}
}

func TestStopForwardRequiresAnID(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{})

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/portforward", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestStopForwardUnknownID(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{})

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/portforward?id=pf-404", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestStartForwardRequiresATarget(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/portforward?kind=Pod&port=8080", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestStartForwardRejectsABadPort(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{})
	cases := []string{"", "0", "-1", "http"}

	for _, port := range cases {
		query := "?kind=Pod&namespace=flux-system&name=web&port=" + port
		resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("port %q gave status %d, want 400", port, resp.StatusCode)
		}
	}
}

func TestStartForwardSurfacesAResolverFailure(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{err: errors.New("no ready pod")})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+forwardQuery, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestForwardsRejectAnUnsupportedMethod(t *testing.T) {
	ts := forwardServer(t, &fakeForwardRunner{local: 1}, &fakeForwardResolver{})

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/portforward"+forwardQuery, nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestForwardsWithoutARegistry(t *testing.T) {
	ts := forwardServer(t, nil, nil)

	resp, listed := doRequest(t, http.MethodGet, ts.URL+"/api/portforward", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(decodeForwards(t, listed)) != 0 {
		t.Fatalf("expected an empty list")
	}

	started, _ := doRequest(t, http.MethodPost, ts.URL+"/api/portforward"+forwardQuery, nil)
	if started.StatusCode != http.StatusBadRequest {
		t.Fatalf("start status = %d, want 400", started.StatusCode)
	}

	stopped, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/portforward?id=pf-1", nil)
	if stopped.StatusCode != http.StatusNotFound {
		t.Fatalf("stop status = %d, want 404", stopped.StatusCode)
	}
}
