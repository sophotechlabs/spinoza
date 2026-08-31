package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
)

type sourced struct {
	*fleet

	sources []api.Kubeconfig
}

func (s sourced) Contexts() api.ContextList {
	list := s.fleet.Contexts()
	list.Kubeconfigs = s.sources
	return list
}

func restoringServer(t *testing.T, held Cluster, tabs []store.Tab) *httptest.Server {
	t.Helper()
	open := &heldTabs{tabs: tabs}
	srv := New(held, testAssets(), testToken)
	srv.UseTabs(open)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func startedOn(id, context string) *fleet {
	return &fleet{
		held:   []api.OpenCluster{{ID: id, Context: context, Active: true}},
		active: id,
	}
}

func TestRestoringATabLeavesTheContextTheFlagChoseActive(t *testing.T) {
	held := startedOn(mk2, "p-mk2")
	ts := restoringServer(t, held, []store.Tab{
		{ID: mk2, Context: "p-mk2", Reopen: true},
		{ID: mk1, Context: "p-mk1", Reopen: true},
	})

	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	if held.ID() != mk2 {
		t.Fatalf("active = %q, want the cluster -context asked for", held.ID())
	}
	if len(held.Opened()) != 2 {
		t.Fatalf("%d clusters open, want the remembered tab to have opened too", len(held.Opened()))
	}
}

func TestOpeningAContextNobodyRememberedSwitchesToIt(t *testing.T) {
	held := startedOn(mk2, "p-mk2")
	ts := restoringServer(t, held, []store.Tab{{ID: mk2, Context: "p-mk2", Reopen: true}})

	doRequest(t, http.MethodPost, ts.URL+"/api/clusters?name=p-mk1", nil)

	if held.ID() != mk1 {
		t.Fatalf("active = %q, want the cluster that was just picked", held.ID())
	}
}

func TestARememberedClusterWhoseContextIsGoneIsNotOfferedForRestore(t *testing.T) {
	held := sourced{
		fleet: startedOn(mk2, "p-mk2"),
		sources: []api.Kubeconfig{{
			Contexts: []api.KubeContext{{Name: "p-mk2"}},
		}},
	}
	ts := restoringServer(t, held, []store.Tab{
		{ID: mk2, Context: "p-mk2", Reopen: true},
		{ID: mk1, Context: "kind-gone", Reopen: true},
	})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	said := clustersFrom(t, body).Remembered
	if len(said) != 1 {
		t.Fatalf("offered %+v for restore, want only the context that still exists", said)
	}
	if said[0].Context != "p-mk2" {
		t.Fatalf("offered %q, want the context that still exists", said[0].Context)
	}
}

func TestAnUnreadableKubeconfigDoesNotHideRememberedClusters(t *testing.T) {
	held := sourced{
		fleet:   startedOn(mk2, "p-mk2"),
		sources: []api.Kubeconfig{{Error: "permission denied"}},
	}
	ts := restoringServer(t, held, []store.Tab{{ID: mk1, Context: "p-mk1", Reopen: true}})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	if said := clustersFrom(t, body).Remembered; len(said) != 1 {
		t.Fatalf("offered %+v for restore, want the tab kept when nothing could be read", said)
	}
}
