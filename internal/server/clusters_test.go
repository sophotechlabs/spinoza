package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
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
	backends    map[string]Backend
	protected   map[string]bool
}

func (f *fleet) Protect(cluster string, protected bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.protected == nil {
		f.protected = map[string]bool{}
	}
	f.protected[cluster] = protected
	return nil
}

func (f *fleet) Protected(cluster string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.protected[cluster]
}

func (f *fleet) Manager(id string) Backend {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == "" {
		id = f.active
	}
	return f.backends[id]
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
	server, _ := heldPair(t)
	return server
}

func heldPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
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
	return <-got, client
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

type heldTabs struct {
	mu      sync.Mutex
	tabs    []store.Tab
	allErr  error
	setErr  error
	dropErr error
}

func (h *heldTabs) All(context.Context) ([]store.Tab, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.allErr != nil {
		return nil, h.allErr
	}
	return append([]store.Tab{}, h.tabs...), nil
}

func (h *heldTabs) Remember(_ context.Context, tab store.Tab) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.setErr != nil {
		return h.setErr
	}
	for at, held := range h.tabs {
		if held.ID == tab.ID {
			h.tabs[at] = tab
			return nil
		}
	}
	h.tabs = append(h.tabs, tab)
	return nil
}

func (h *heldTabs) Recolor(_ context.Context, id string, color int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.setErr != nil {
		return h.setErr
	}
	for at, held := range h.tabs {
		if held.ID == id {
			h.tabs[at].Color = color
		}
	}
	return nil
}

func (h *heldTabs) Rename(_ context.Context, id, label, grouping string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.setErr != nil {
		return h.setErr
	}
	for at, held := range h.tabs {
		if held.ID == id {
			h.tabs[at].Label = label
			h.tabs[at].Grouping = grouping
		}
	}
	return nil
}

func (h *heldTabs) Reopening(_ context.Context, id string, reopen bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.setErr != nil {
		return h.setErr
	}
	for at, held := range h.tabs {
		if held.ID == id {
			h.tabs[at].Reopen = reopen
		}
	}
	return nil
}

func (h *heldTabs) Recording(_ context.Context, id, kinds string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.setErr != nil {
		return h.setErr
	}
	for at, held := range h.tabs {
		if held.ID == id {
			h.tabs[at].Timeline = kinds
		}
	}
	return nil
}

func (h *heldTabs) Forget(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dropErr != nil {
		return h.dropErr
	}
	kept := []store.Tab{}
	for _, held := range h.tabs {
		if held.ID != id {
			kept = append(kept, held)
		}
	}
	h.tabs = kept
	return nil
}

func (h *heldTabs) remembered() []store.Tab {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Tab{}, h.tabs...)
}

var fleets = map[string]*fleet{}

var servers = map[string]*Server{}

func srvOf(ts *httptest.Server) *Server {
	return servers[ts.URL]
}

func heldFleet(ts *httptest.Server) *fleet {
	return fleets[ts.URL]
}

func listBody(t *testing.T, ts *httptest.Server) []byte {
	t.Helper()
	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)
	return body
}

func idOf(open *heldTabs, name string) string {
	for _, one := range open.remembered() {
		if one.Context == name {
			return one.ID
		}
	}
	return ""
}

func rememberingServer(t *testing.T, held *fleet) (*httptest.Server, *heldTabs) {
	t.Helper()
	open := &heldTabs{}
	srv := New(held, testAssets(), testToken)
	srv.UseTabs(open)
	ts := httptest.NewServer(authed(srv.Handler()))
	fleets[ts.URL] = held
	servers[ts.URL] = srv
	t.Cleanup(ts.Close)
	return ts, open
}

func TestOpeningAClusterRemembersItForNextTime(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})

	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?kubeconfig=%2Fwork.yaml&name=p-mk1", nil)

	held := open.remembered()
	if len(held) != 1 {
		t.Fatalf("remembered %d clusters, want the one that was opened", len(held))
	}
	if held[0].Context != "p-mk1" || held[0].Kubeconfig != "/work.yaml" {
		t.Fatalf("remembered %+v, want what it takes to open it again", held[0])
	}
}

func TestClosingAClusterStopsItReopeningNextTime(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	id := open.remembered()[0].ID

	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster="+urlValue(id), nil)

	if held := open.remembered(); len(held) != 0 {
		t.Fatalf("remembered %+v after it was closed", held)
	}
}

func TestTheListSaysWhichClustersWereOpenLastTime(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	open.tabs = []store.Tab{{ID: mk2, Context: "p-mk2", Kubeconfig: "/work.yaml", Reopen: true}}

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	said := clustersFrom(t, body).Remembered
	if len(said) != 1 || said[0].Context != "p-mk2" {
		t.Fatalf("remembered = %s, want the tab that was open last time", body)
	}
}

func TestAStoreThatCannotBeReadDoesNotStopTheList(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	open.allErr = errors.New("the file is gone")

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(clustersFrom(t, body).Remembered) != 0 {
		t.Fatalf("remembered = %s, want none when the held could not be read", body)
	}
}

func TestAStoreThatCannotBeWrittenDoesNotStopOpening(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	open.setErr = errors.New("read-only file system")
	open.dropErr = errors.New("read-only file system")

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s, want the cluster open even with nowhere to remember it", resp.StatusCode, body)
	}
	closed, _ := doRequest(t, http.MethodDelete,
		ts.URL+"/api/clusters?cluster="+urlValue(clustersFrom(t, body).Clusters[0].ID), nil)
	if closed.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the cluster closed even with nowhere to forget it", closed.StatusCode)
	}
}

func TestAServerWithNoStoreRemembersNothing(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	if len(clustersFrom(t, body).Remembered) != 0 {
		t.Fatalf("remembered = %s, want none without a held", body)
	}
}

func colorsOf(t *testing.T, body []byte) map[string]int {
	t.Helper()
	found := map[string]int{}
	for _, one := range clustersFrom(t, body).Clusters {
		found[one.Context] = one.Color
	}
	return found
}

func TestTheFirstClusterOpenedGetsTheFirstColor(t *testing.T) {
	ts, _ := rememberingServer(t, &fleet{})

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	if color := colorsOf(t, body)["p-mk1"]; color != 1 {
		t.Fatalf("color = %d, want the first one", color)
	}
}

func TestTwoOpenClustersNeverLookAlike(t *testing.T) {
	ts, _ := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk2", nil)

	found := colorsOf(t, body)
	if found["p-mk1"] == found["p-mk2"] {
		t.Fatalf("both clusters are color %d; two tabs would look the same", found["p-mk1"])
	}
}

func TestAClusterKeepsTheColorItWasGiven(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk2", nil)
	was := colorsOf(t, listBody(t, ts))["p-mk2"]
	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster="+urlValue(idOf(open, "p-mk2")), nil)
	open.tabs = append(open.tabs, store.Tab{ID: mk2, Context: "p-mk2", Color: was})

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk2", nil)

	if now := colorsOf(t, body)["p-mk2"]; now != was {
		t.Fatalf("color = %d, want the %d it had before it was closed", now, was)
	}
}

func TestTheColorsRunOutRatherThanGoingBlank(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	for color := 1; color <= api.ClusterColors; color++ {
		id := "https://c" + strconv.Itoa(color) + ":6443"
		open.tabs = append(open.tabs, store.Tab{ID: id, Color: color})
		heldFleet(ts).held = append(heldFleet(ts).held, api.OpenCluster{ID: id})
	}

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk9", nil)

	if color := colorsOf(t, body)["p-mk9"]; color < 1 || color > api.ClusterColors {
		t.Fatalf("color = %d, want one from the palette even with every color taken", color)
	}
}

func TestAColorCanBeChanged(t *testing.T) {
	ts, _ := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	id := clustersFrom(t, listBody(t, ts)).Clusters[0].ID

	_, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/color?cluster="+urlValue(id)+"&color=5", nil)

	if color := colorsOf(t, body)["p-mk1"]; color != 5 {
		t.Fatalf("color = %d, want the one that was asked for", color)
	}
}

func TestAColorOutsideThePaletteIsRefused(t *testing.T) {
	ts, _ := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	id := clustersFrom(t, listBody(t, ts)).Clusters[0].ID

	for _, asked := range []string{"0", "9", "blue", ""} {
		resp, _ := doRequest(t, http.MethodPost,
			ts.URL+"/api/clusters/color?cluster="+urlValue(id)+"&color="+asked, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("color=%q gave status %d, want it refused", asked, resp.StatusCode)
		}
	}
}

func TestRecoloringNeedsToKnowWhichCluster(t *testing.T) {
	ts, _ := rememberingServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/color?color=2", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want it refused without a cluster", resp.StatusCode)
	}
}

func TestRecoloringWithNowhereToKeepItSaysSo(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/color?cluster="+urlValue(mk1)+"&color=2", nil)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want it to say there is nowhere to keep it", resp.StatusCode)
	}
}

func TestAColorThatCannotBeWrittenIsReported(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	open.setErr = errors.New("read-only file system")

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/color?cluster="+urlValue(mk1)+"&color=2", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a color that was never written reported success")
	}
}

func TestTheClusterSpinozaStartsOnGetsATabToo(t *testing.T) {
	held := &fleet{
		held:   []api.OpenCluster{{ID: mk1, Context: "p-mk1", Kubeconfig: "/work.yaml", Active: true}},
		active: mk1,
	}
	ts, open := rememberingServer(t, held)

	srvOf(ts).RememberOpen(t.Context())

	remembered := open.remembered()
	if len(remembered) != 1 {
		t.Fatalf("remembered %d clusters, want the one spinoza started on", len(remembered))
	}
	if remembered[0].Color == 0 {
		t.Fatal("the startup cluster has no color, so its tab would look like an unknown one")
	}
	if remembered[0].Kubeconfig != "/work.yaml" {
		t.Fatalf("remembered %+v, want what it takes to open it again", remembered[0])
	}
}

func TestTheStartupClusterKeepsTheColorItAlreadyHad(t *testing.T) {
	held := &fleet{
		held:   []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active: mk1,
	}
	ts, open := rememberingServer(t, held)
	open.tabs = []store.Tab{{ID: mk1, Context: "p-mk1", Color: 6}}

	srvOf(ts).RememberOpen(t.Context())

	if color := open.remembered()[0].Color; color != 6 {
		t.Fatalf("color = %d, want the 6 it already had", color)
	}
}

func TestAServerWithNoStoreStartsAnyway(t *testing.T) {
	held := &fleet{held: []api.OpenCluster{{ID: mk1, Context: "p-mk1"}}}
	srv := New(held, testAssets(), testToken)

	srv.RememberOpen(t.Context())
}

func namesOf(t *testing.T, body []byte) map[string]api.OpenCluster {
	t.Helper()
	found := map[string]api.OpenCluster{}
	for _, one := range clustersFrom(t, body).Clusters {
		found[one.Context] = one
	}
	return found
}

func openedFleet(t *testing.T) (*httptest.Server, *heldTabs) {
	t.Helper()
	ts, open := rememberingServer(t, &fleet{})
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)
	return ts, open
}

func TestATabCanBeGivenAnotherName(t *testing.T) {
	ts, _ := openedFleet(t)

	_, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=client+a+prod&grouping=Client+A", nil)

	held := namesOf(t, body)["p-mk1"]
	if held.Label != "client a prod" {
		t.Fatalf("label = %q, want the one that was typed", held.Label)
	}
	if held.Grouping != "Client A" {
		t.Fatalf("grouping = %q, want the group it was put in", held.Grouping)
	}
}

func TestANameIsTrimmedRatherThanTakenAsTyped(t *testing.T) {
	ts, _ := openedFleet(t)

	_, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=++prod++", nil)

	if label := namesOf(t, body)["p-mk1"].Label; label != "prod" {
		t.Fatalf("label = %q, want it trimmed", label)
	}
}

func TestANameCanBeTakenBackOff(t *testing.T) {
	ts, _ := openedFleet(t)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=prod", nil)

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/name?cluster="+urlValue(mk1), nil)

	if label := namesOf(t, body)["p-mk1"].Label; label != "" {
		t.Fatalf("label = %q, want the context name back", label)
	}
}

func TestANameLongerThanTheStripCanShowIsRefused(t *testing.T) {
	ts, _ := openedFleet(t)

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label="+strings.Repeat("x", maxLabel+1), nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want a name that long refused", resp.StatusCode)
	}
}

func TestANameLimitCountsCharacters(t *testing.T) {
	ts, _ := openedFleet(t)
	label := strings.Repeat("ž", maxLabel)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label="+url.QueryEscape(label), nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want a %d-character name accepted", resp.StatusCode, maxLabel)
	}
	if got := namesOf(t, body)["p-mk1"].Label; got != label {
		t.Fatalf("label = %q, want %q", got, label)
	}
}

func TestRenamingNeedsToKnowWhichTab(t *testing.T) {
	ts, _ := openedFleet(t)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/name?label=prod", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want it refused without a cluster", resp.StatusCode)
	}
}

func TestRenamingWithNowhereToKeepItSaysSo(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=prod", nil)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want it to say there is nowhere to keep it", resp.StatusCode)
	}
}

func TestANameThatCannotBeWrittenIsReported(t *testing.T) {
	ts, open := openedFleet(t)
	open.setErr = errors.New("read-only file system")

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=prod", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a name that was never written reported success")
	}
}

func TestATabComesBackNextTimeUnlessItIsToldNotTo(t *testing.T) {
	ts, _ := openedFleet(t)

	if !namesOf(t, listBody(t, ts))["p-mk1"].Reopen {
		t.Fatal("a tab that was just opened would not come back")
	}

	_, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/reopen?cluster="+urlValue(mk1)+"&reopen=false", nil)

	if namesOf(t, body)["p-mk1"].Reopen {
		t.Fatal("the tab still says it comes back after being told not to")
	}
}

func TestATabToldNotToComeBackKeepsThatThroughAReopen(t *testing.T) {
	ts, open := openedFleet(t)
	doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/reopen?cluster="+urlValue(mk1)+"&reopen=false", nil)
	doRequest(t, http.MethodPost, ts.URL+"/api/clusters/name?cluster="+urlValue(mk1)+"&label=prod", nil)

	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	held := open.remembered()[0]
	if held.Reopen {
		t.Fatal("opening the cluster again turned reopen back on")
	}
	if held.Label != "prod" {
		t.Fatalf("label = %q, want the name kept through a reopen", held.Label)
	}
}

func TestReopenMustBeTrueOrFalse(t *testing.T) {
	ts, _ := openedFleet(t)

	for _, asked := range []string{"", "maybe", "1"} {
		resp, _ := doRequest(t, http.MethodPost,
			ts.URL+"/api/clusters/reopen?cluster="+urlValue(mk1)+"&reopen="+asked, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("reopen=%q gave status %d, want it refused", asked, resp.StatusCode)
		}
	}
}

func TestReopenNeedsToKnowWhichTab(t *testing.T) {
	ts, _ := openedFleet(t)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/clusters/reopen?reopen=false", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want it refused without a cluster", resp.StatusCode)
	}
}

func TestReopenWithNowhereToKeepItSaysSo(t *testing.T) {
	ts := fleetServer(t, &fleet{})

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/reopen?cluster="+urlValue(mk1)+"&reopen=false", nil)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want it to say there is nowhere to keep it", resp.StatusCode)
	}
}

func TestAReopenFlagThatCannotBeWrittenIsReported(t *testing.T) {
	ts, open := openedFleet(t)
	open.setErr = errors.New("read-only file system")

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/reopen?cluster="+urlValue(mk1)+"&reopen=false", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a flag that was never written reported success")
	}
}

func TestATabToldNotToComeBackIsNotOfferedForReopening(t *testing.T) {
	ts, open := rememberingServer(t, &fleet{})
	open.tabs = []store.Tab{
		{ID: mk1, Context: "p-mk1", Reopen: true},
		{ID: mk2, Context: "p-mk2", Reopen: false},
	}

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	offered := clustersFrom(t, body).Remembered
	if len(offered) != 1 || offered[0].Context != "p-mk1" {
		t.Fatalf("offered %+v, want only the tab that asked to come back", offered)
	}
}

func TestATabWriteOutlivesTheRequestThatAskedForIt(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})
	tabs := &heldTabs{}
	srv.UseTabs(tabs)

	gone, walkAway := context.WithCancel(context.Background())
	req, reqErr := http.NewRequestWithContext(gone, http.MethodPost, "http://spinoza.test/api/clusters?name=p-mk1", http.NoBody)
	if reqErr != nil {
		t.Fatalf("request: %v", reqErr)
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), rememberTimeout)
	defer cancel()
	walkAway()

	if kept.Err() != nil {
		t.Fatalf("the kept context died with the request: %v", kept.Err())
	}
	srv.rememberTab(kept, mk1, api.ContextRef{Name: "p-mk1"})

	if len(tabs.remembered()) == 0 {
		t.Fatal("the tab was not written, so the cluster will not come back next time")
	}
}

func TestTheKeptContextStillGivesUpEventually(t *testing.T) {
	req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://spinoza.test/api/clusters", http.NoBody)
	if reqErr != nil {
		t.Fatalf("request: %v", reqErr)
	}

	kept, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), rememberTimeout)
	defer cancel()

	deadline, ok := kept.Deadline()
	if !ok {
		t.Fatal("a kept context with no deadline would hold a write open forever")
	}
	if time.Until(deadline) > rememberTimeout {
		t.Fatalf("deadline is %s away, want no more than %s", time.Until(deadline), rememberTimeout)
	}
}
