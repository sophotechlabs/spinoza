package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type fleet struct {
	Cluster

	mu          sync.Mutex
	held        []api.OpenCluster
	active      string
	openErr     error
	activateErr error
	askedToOpen []api.ContextRef
	activated   []string
	closed      []string
	closeErr    error
}

func (f *fleet) Manager(string) Backend {
	return nil
}

func (f *fleet) ID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

func (f *fleet) Opened() []api.OpenCluster {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.OpenCluster{}, f.held...)
}

func (f *fleet) Open(ref api.ContextRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.askedToOpen = append(f.askedToOpen, ref)
	if f.openErr != nil {
		return "", f.openErr
	}
	id := "https://" + ref.Name + ":6443"
	f.held = append(f.held, api.OpenCluster{ID: id, Context: ref.Name, Kubeconfig: ref.Kubeconfig})
	f.active = id
	return id, nil
}

func (f *fleet) Close(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	if f.closeErr != nil {
		return f.closeErr
	}
	kept := make([]api.OpenCluster, 0, len(f.held))
	for _, one := range f.held {
		if one.ID != id {
			kept = append(kept, one)
		}
	}
	f.held = kept
	if f.active == id {
		f.active = ""
		if len(kept) > 0 {
			f.active = kept[0].ID
		}
	}
	return nil
}

func (f *fleet) Activate(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activated = append(f.activated, id)
	if f.activateErr != nil {
		return f.activateErr
	}
	f.active = id
	return nil
}

func (f *fleet) Contexts() api.ContextList {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := api.ContextRef{}
	for _, held := range f.held {
		if held.ID == f.active {
			current = api.ContextRef{Kubeconfig: held.Kubeconfig, Name: held.Context}
		}
	}
	return api.ContextList{Current: current, Kubeconfigs: []api.Kubeconfig{}}
}

func (f *fleet) asked() []api.ContextRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.ContextRef{}, f.askedToOpen...)
}

func fleetServer(t *testing.T, held *fleet) *httptest.Server {
	t.Helper()
	srv := New(held, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func clustersFrom(t *testing.T, body []byte) api.ClusterList {
	t.Helper()
	var got api.ClusterList
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	return got
}

func TestNothingIsOpenBeforeAnythingIsOpened(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(clustersFrom(t, body).Clusters) != 0 {
		t.Fatalf("clusters = %s, want none", body)
	}
}

func TestTheListIsReadableWithoutACluster(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the list served so the picker works with nothing connected", resp.StatusCode)
	}
}

func TestOpeningACluster(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters?kubeconfig=%2Fwork.yaml&name=p-mk1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	asked := held.asked()
	if len(asked) != 1 {
		t.Fatalf("opened %d times, want once", len(asked))
	}
	if asked[0].Name != "p-mk1" || asked[0].Kubeconfig != "/work.yaml" {
		t.Fatalf("opened %+v, want the context and the kubeconfig it came from", asked[0])
	}
	got := clustersFrom(t, body).Clusters
	if len(got) != 1 || got[0].Context != "p-mk1" {
		t.Fatalf("list = %s, want the cluster just opened", body)
	}
}

func TestOpeningWithoutANameIsRefused(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/clusters", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(held.asked()) != 0 {
		t.Fatal("a request with no context still reached the cluster")
	}
}

func TestAClusterThatWillNotOpenIsReported(t *testing.T) {
	ts := fleetServer(t, &fleet{openErr: errors.New("context \"gone\" did not answer within 30s")})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=gone", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("opening a cluster that never answered reported success")
	}
	if !strings.Contains(string(body), "did not answer") {
		t.Fatalf("body = %s, want it to say what went wrong", body)
	}
}

func TestActivatingATab(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk2", nil)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/active?cluster=https%3A%2F%2Fp-mk1%3A6443", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if held.active != "https://p-mk1:6443" {
		t.Fatalf("active = %q, want the tab that was clicked", held.active)
	}
}

func TestActivatingWithoutNamingAClusterIsRefused(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/active", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestActivatingAClusterNobodyOpenedIsNotFound(t *testing.T) {
	ts := fleetServer(t, &fleet{
		activateErr: fmt.Errorf("%w: no cluster https://ghost:6443 is open", api.ErrNotOpen),
	})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/active?cluster=https%3A%2F%2Fghost%3A6443", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, body)
	}
}

func TestTheListSaysWhichTabIsActiveAndWhetherItAnswers(t *testing.T) {
	held := &fleet{held: []api.OpenCluster{
		{ID: "https://p-mk1:6443", Context: "p-mk1", Active: true, Protection: api.ProtectionProtected},
		{ID: "https://p-mk2:6443", Context: "p-mk2", Protection: api.ProtectionOpen},
	}}
	ts := fleetServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	got := clustersFrom(t, body).Clusters
	if len(got) != 2 {
		t.Fatalf("clusters = %s, want both", body)
	}
	if !got[0].Active || got[1].Active {
		t.Fatalf("active flags = %v/%v, want only the first", got[0].Active, got[1].Active)
	}
	if got[0].Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want it carried through so the strip can mark it", got[0].Protection)
	}
	for _, one := range got {
		if !one.Reachable {
			t.Fatalf("%s came back unreachable before anything pinged it", one.ID)
		}
	}
}

func TestClosingATab(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk2", nil)

	resp, body := doRequest(t, http.MethodDelete,
		ts.URL+"/api/clusters?cluster=https%3A%2F%2Fp-mk1%3A6443", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	left := clustersFrom(t, body).Clusters
	if len(left) != 1 || left[0].Context != "p-mk2" {
		t.Fatalf("left open = %s, want only p-mk2", body)
	}
}

func TestClosingWithoutNamingAClusterIsRefused(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/clusters", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(held.closed) != 0 {
		t.Fatal("a request naming no cluster still closed one")
	}
}

func TestClosingAClusterNobodyOpenedIsNotFound(t *testing.T) {
	ts := fleetServer(t, &fleet{
		closeErr: fmt.Errorf("%w: no cluster https://ghost:6443 is open", api.ErrNotOpen),
	})

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster=https%3A%2F%2Fghost%3A6443", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestClosingTheLastTabLeavesNothingActive(t *testing.T) {
	held := &fleet{}
	ts := fleetServer(t, held)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=only", nil)

	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster=https%3A%2F%2Fonly%3A6443", nil)

	if held.ID() != "" {
		t.Fatalf("active = %q, want nothing once the last tab is gone", held.ID())
	}
}

func heldSocket(t *testing.T) *websocket.Conn {
	t.Helper()
	got := make(chan *websocket.Conn, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		got <- conn
		<-r.Context().Done()
	}))
	t.Cleanup(fixture.Close)
	client, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(fixture.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.CloseNow() })
	return <-got
}

func TestAShellOnAClosedClusterIsHungUpBeforeTheConnectionGoes(t *testing.T) {
	held := &fleet{}
	srv := New(held, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	shell := heldSocket(t)
	srv.trackExec(shell, "https://p-mk1:6443")
	srv.trackExec(heldSocket(t), "https://p-mk2:6443")
	go func() {
		time.Sleep(30 * time.Millisecond)
		srv.forgetExec(shell)
	}()

	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster=https%3A%2F%2Fp-mk1%3A6443", nil)

	if len(srv.terminalsOn("https://p-mk1:6443")) != 0 {
		t.Fatal("the connection was closed while a shell on it was still open")
	}
	if len(srv.terminalsOn("https://p-mk2:6443")) != 1 {
		t.Fatal("closing one cluster hung up a shell on another")
	}
}

func TestALocalShellSurvivesAClusterClosing(t *testing.T) {
	held := &fleet{}
	srv := New(held, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	srv.trackExec(heldSocket(t), localShellCluster)

	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster=https%3A%2F%2Fp-mk1%3A6443", nil)

	if len(srv.terminalsOn(localShellCluster)) != 1 {
		t.Fatal("closing a cluster hung up the shell on the laptop, which belongs to no cluster")
	}
}
