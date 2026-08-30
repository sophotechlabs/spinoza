package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/issues"
)

type queueBackend struct {
	Backend

	queue api.IssueQueue
}

func (b *queueBackend) Issues(context.Context) api.IssueQueue {
	return b.queue
}

func issue(id, severity string) api.Issue {
	return api.Issue{
		ID:       id,
		Severity: severity,
		Title:    id,
		Object:   api.ObjectRef{Namespace: "default", Name: id},
		Since:    "2026-08-30T00:00:00Z",
	}
}

func queueOf(rows ...api.Issue) api.IssueQueue {
	return api.IssueQueue{Rows: rows}
}

func queueServer(t *testing.T, first, second api.IssueQueue) *httptest.Server {
	t.Helper()
	srv, _ := twoClusters(t, &queueBackend{queue: first}, &queueBackend{queue: second})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func fleetFrom(t *testing.T, body []byte) api.IssueQueue {
	t.Helper()
	var got api.IssueQueue
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	return got
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestTheFleetQueueHoldsEveryOpenClustersIssues(t *testing.T) {
	ts := queueServer(t,
		queueOf(issue("a", api.SeverityWarning)),
		queueOf(issue("b", api.SeverityWarning)))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	rows := fleetFrom(t, body).Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want one from each open cluster", len(rows))
	}
}

func TestEveryFleetRowSaysWhichClusterItIsOn(t *testing.T) {
	ts := queueServer(t,
		queueOf(issue("a", api.SeverityWarning)),
		queueOf(issue("b", api.SeverityWarning)))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	on := map[string]string{}
	for _, row := range fleetFrom(t, body).Rows {
		on[row.ID] = row.Cluster
	}
	if on["a"] != mk1 || on["b"] != mk2 {
		t.Fatalf("rows are on %v, want each named against the cluster it came from", on)
	}
}

func TestTheWorstInTheFleetIsAtTheTopWhicheverClusterItIsOn(t *testing.T) {
	ts := queueServer(t,
		queueOf(issue("mild", api.SeverityWarning)),
		queueOf(issue("broken", api.SeverityFatal)))

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	rows := fleetFrom(t, body).Rows
	if rows[0].ID != "broken" {
		t.Fatalf("first row = %q, want the worst thing in the fleet", rows[0].ID)
	}
}

func TestAClusterThatCouldNotAnswerIsNamedRatherThanMissed(t *testing.T) {
	ts := queueServer(t,
		queueOf(issue("a", api.SeverityWarning)),
		api.IssueQueue{Error: "the cluster is not answering"})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	said := fleetFrom(t, body).Error
	if !contains(said, "p-mk2") || !contains(said, "not answering") {
		t.Fatalf("error = %q, want the cluster that failed named", said)
	}
}

func TestWhatEachClusterHeldBackIsCountedTogether(t *testing.T) {
	ts := queueServer(t,
		api.IssueQueue{Rows: []api.Issue{issue("a", api.SeverityWarning)}, Dropped: 3},
		api.IssueQueue{Rows: []api.Issue{issue("b", api.SeverityWarning)}, Dropped: 4})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	if dropped := fleetFrom(t, body).Dropped; dropped != 7 {
		t.Fatalf("dropped = %d, want what both clusters held back", dropped)
	}
}

func TestTheMergedQueueStopsAtTheSameCapOneClusterDoes(t *testing.T) {
	first := api.IssueQueue{}
	second := api.IssueQueue{}
	for at := range issues.MaxRows {
		first.Rows = append(first.Rows, issue("a"+itoa(at), api.SeverityWarning))
		second.Rows = append(second.Rows, issue("b"+itoa(at), api.SeverityWarning))
	}
	ts := queueServer(t, first, second)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet?shown=1000", nil)

	merged := fleetFrom(t, body)
	if len(merged.Rows) != issues.MaxRows {
		t.Fatalf("rows = %d, want the queue capped at %d", len(merged.Rows), issues.MaxRows)
	}
	if merged.Dropped != issues.MaxRows {
		t.Fatalf("dropped = %d, want the rest counted", merged.Dropped)
	}
	if merged.Next != "" {
		t.Fatalf("a page holding the whole cap still offers %q", merged.Next)
	}
}

func TestTheFleetQueueHandsOutOnePageAtATime(t *testing.T) {
	first := api.IssueQueue{}
	second := api.IssueQueue{}
	for at := range issues.Shown {
		first.Rows = append(first.Rows, issue("a"+itoa(at), api.SeverityWarning))
		second.Rows = append(second.Rows, issue("b"+itoa(at), api.SeverityWarning))
	}
	ts := queueServer(t, first, second)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	merged := fleetFrom(t, body)
	if len(merged.Rows) != issues.Shown {
		t.Fatalf("rows = %d, want one page of %d", len(merged.Rows), issues.Shown)
	}
	if merged.Next == "" {
		t.Fatal("a queue twice a page long offered no cursor for the rest")
	}

	_, rest := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet?after="+merged.Next, nil)

	tail := fleetFrom(t, rest)
	if len(tail.Rows) != issues.Shown {
		t.Fatalf("the second page carries %d rows, want %d", len(tail.Rows), issues.Shown)
	}
	if tail.Next != "" {
		t.Fatalf("the last page still offers %q", tail.Next)
	}
	seen := map[string]bool{}
	for _, row := range append(merged.Rows, tail.Rows...) {
		if seen[row.Cluster+"/"+row.ID] {
			t.Fatalf("row %q came back on both pages", row.ID)
		}
		seen[row.Cluster+"/"+row.ID] = true
	}
	if len(seen) != 2*issues.Shown {
		t.Fatalf("the two pages carried %d distinct rows, want %d", len(seen), 2*issues.Shown)
	}
}

func TestAClusterWithNoBackendIsSkippedRatherThanFatal(t *testing.T) {
	srv, held := twoClusters(t, &queueBackend{queue: queueOf(issue("a", api.SeverityWarning))}, nil)
	held.backends[mk2] = nil
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(fleetFrom(t, body).Rows) != 1 {
		t.Fatalf("rows = %s, want the cluster that answered", body)
	}
}

func TestOneClustersQueueStillSaysWhichClusterItIsOn(t *testing.T) {
	ts := queueServer(t, queueOf(issue("a", api.SeverityWarning)), queueOf())

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues?cluster="+urlValue(mk1), nil)

	rows := fleetFrom(t, body).Rows
	if len(rows) != 1 || rows[0].Cluster != mk1 {
		t.Fatalf("rows = %s, want the row named against its cluster", body)
	}
}
