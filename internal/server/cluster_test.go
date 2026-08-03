package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type stubCluster struct {
	mu       sync.Mutex
	mgr      *resources.Manager
	names    []string
	current  string
	useErr   error
	switched []string
}

func fixed(mgr *resources.Manager) *stubCluster {
	return &stubCluster{mgr: mgr, names: []string{"p-mk1", "p-mk2"}, current: "p-mk2"}
}

func (s *stubCluster) Manager() *resources.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr
}

func (s *stubCluster) Contexts() api.ContextList {
	s.mu.Lock()
	defer s.mu.Unlock()
	return api.ContextList{Contexts: s.names, Current: s.current}
}

func (s *stubCluster) Use(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.useErr != nil {
		return s.useErr
	}
	s.switched = append(s.switched, name)
	s.current = name
	return nil
}

func (s *stubCluster) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.switched...)
}

func contextServer(t *testing.T, cluster Cluster) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New(cluster, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestContextsAreListedWithTheCurrentOne(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	list := decodeContexts(t, body)
	if len(list.Contexts) != 2 {
		t.Fatalf("contexts = %v", list.Contexts)
	}
	if list.Current != "p-mk2" {
		t.Fatalf("current = %q", list.Current)
	}
}

func TestSwitchingContextsAsksTheCluster(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=p-mk1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(cluster.calls()) != 1 || cluster.calls()[0] != "p-mk1" {
		t.Fatalf("switched = %v", cluster.calls())
	}
	list := decodeContexts(t, body)
	if list.Current != "p-mk1" {
		t.Fatalf("current = %q, want the new context echoed back", list.Current)
	}
}

func TestSwitchingRequiresAName(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(cluster.calls()) != 0 {
		t.Fatalf("switched = %v on a bad request", cluster.calls())
	}
}

func TestAFailedSwitchKeepsTheOldContext(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.useErr = errors.New("context \"gone\" does not exist")
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=gone", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a failed switch reported success")
	}
	if !strings.Contains(string(body), "does not exist") {
		t.Fatalf("body = %s, want the reason", body)
	}
	if cluster.Contexts().Current != "p-mk2" {
		t.Fatalf("current = %q, want the old context kept", cluster.Contexts().Current)
	}
}

func TestContextsRejectsADelete(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func decodeContexts(t *testing.T, body []byte) api.ContextList {
	t.Helper()
	var list api.ContextList
	err := json.Unmarshal(body, &list)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return list
}

func TestSwitchingClosesOpenSessions(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	cluster := fixed(mgr)
	srv := New(cluster, testAssets())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	writeErr := wsjson.Write(ctx, conn, api.ClientMsg{
		Type: "subscribe", SubID: "main", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default",
	})
	if writeErr != nil {
		t.Fatalf("subscribe: %v", writeErr)
	}
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected a snapshot")
	}

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=p-mk1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switch: %d %s", resp.StatusCode, body)
	}

	_, _, readErr := conn.Read(ctx)
	if readErr == nil {
		t.Fatal("the session survived a context switch; it would stream the old cluster's objects")
	}
}
