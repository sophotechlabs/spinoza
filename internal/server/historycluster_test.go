package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/history"
)

func recordingFleet(t *testing.T) (*httptest.Server, *heldHistory) {
	t.Helper()
	srv, _ := twoClusters(t, &writingBackend{}, &writingBackend{})
	store := &heldHistory{}
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(store)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, store
}

func restart(t *testing.T, ts *httptest.Server, cluster string) {
	t.Helper()
	asked := ts.URL + "/api/action?action=restart&group=apps&version=v1&resource=deployments" +
		"&namespace=default&name=web&cluster=" + urlValue(cluster)
	resp, body := doRequest(t, http.MethodPost, asked, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}

func TestWhatWasDoneToAnotherClusterIsRecordedAgainstThatCluster(t *testing.T) {
	ts, store := recordingFleet(t)

	restart(t, ts, mk2)

	if held := store.only(t); held.Cluster != mk2 {
		t.Fatalf("recorded against %q, want the cluster the action was for", held.Cluster)
	}
}

func TestWhatWasDoneWithNoClusterNamedIsRecordedAgainstTheActiveOne(t *testing.T) {
	ts, store := recordingFleet(t)

	restart(t, ts, "")

	if held := store.only(t); held.Cluster != mk1 {
		t.Fatalf("recorded against %q, want the active cluster", held.Cluster)
	}
}

func TestHistoryIsReadForTheClusterTheTabIsShowing(t *testing.T) {
	ts, store := recordingFleet(t)

	doRequest(t, http.MethodGet, ts.URL+"/api/history?cluster="+urlValue(mk2), nil)

	if asked := store.asked(); asked.Cluster != mk2 {
		t.Fatalf("asked for %q, want the cluster the tab is showing", asked.Cluster)
	}
}

func TestClearingHistoryLeavesTheOtherClustersAlone(t *testing.T) {
	ts, store := recordingFleet(t)
	restart(t, ts, mk1)
	restart(t, ts, mk2)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/history?cluster="+urlValue(mk2), nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	held := store.recorded()
	if len(held) != 1 || held[0].Cluster != mk1 {
		t.Fatalf("kept %+v, want only the cluster that was not cleared", held)
	}
}

func TestClearingHistoryNamesTheClusterItClears(t *testing.T) {
	ts, store := recordingFleet(t)

	doRequest(t, http.MethodDelete, ts.URL+"/api/history?cluster="+urlValue(mk2), nil)

	if store.forgotCluster != mk2 {
		t.Fatalf("cleared %q, want the cluster asked for; an empty one clears every cluster", store.forgotCluster)
	}
}

func TestClearingHistoryWithNoClusterOpenIsRefused(t *testing.T) {
	srv := New(noCluster{}, testAssets(), testToken)
	store := &heldHistory{entries: []history.Entry{{Cluster: mk1, Name: "web"}}}
	srv.UseHistory(store)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode == http.StatusNoContent {
		t.Fatal("history was cleared with no cluster to clear it for; that wipes every cluster")
	}
	if len(store.recorded()) != 1 {
		t.Fatal("another cluster's history was cleared by a window with no cluster of its own")
	}
}

func urlValue(raw string) string {
	return url.QueryEscape(raw)
}
