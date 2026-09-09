package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func sampleReport(fresh, cleared int) api.CheckReport {
	return api.CheckReport{
		Scanned: 42,
		Groups: []api.CheckGroup{
			{ID: "privileged-container", Severity: "high", Total: 3, NewCount: fresh},
			{ID: "requests-missing", Severity: "low", Total: 1, Fixed: cleared},
		},
	}
}

func TestARunCountsWhatTheReportFound(t *testing.T) {
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	run := runOf(sampleReport(2, 1), at)

	if run.Findings != 4 {
		t.Fatalf("findings = %d, want 4", run.Findings)
	}
	if run.Fresh != 2 {
		t.Fatalf("new = %d, want 2", run.Fresh)
	}
	if run.Cleared != 1 {
		t.Fatalf("cleared = %d, want 1", run.Cleared)
	}
	if run.Scanned != 42 {
		t.Fatalf("scanned = %d, want 42", run.Scanned)
	}
	if !run.At.Equal(at) {
		t.Fatalf("at = %s, want %s", run.At, at)
	}
}

func TestTheNoticeNamesWhatChangedAndCountsBySeverity(t *testing.T) {
	run := runOf(sampleReport(2, 1), time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC))

	notice := noticeOf("kind-spinoza", run, sampleReport(2, 1))

	if notice.Cluster != "kind-spinoza" {
		t.Fatalf("cluster = %q", notice.Cluster)
	}
	if notice.At != "2026-09-09T12:00:00Z" {
		t.Fatalf("at = %q", notice.At)
	}
	if notice.Severity["high"] != 3 || notice.Severity["low"] != 1 {
		t.Fatalf("severity = %v", notice.Severity)
	}
	if len(notice.Changed) != 2 {
		t.Fatalf("changed = %v, want both groups", notice.Changed)
	}
}

type countingReceiver struct {
	mu      sync.Mutex
	bodies  [][]byte
	refuse  int
	server  *httptest.Server
	refused int
}

func newReceiver(refuse int) *countingReceiver {
	got := &countingReceiver{refuse: refuse}
	got.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		got.mu.Lock()
		defer got.mu.Unlock()
		if got.refused < got.refuse {
			got.refused++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		got.bodies = append(got.bodies, body)
		w.WriteHeader(http.StatusOK)
	}))
	return got
}

func (c *countingReceiver) posts() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte{}, c.bodies...)
}

func TestTheNoticeGoesOutOncePerChangedRun(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	runner := &auditRunner{post: got.server.URL, client: got.server.Client(), retryAfter: time.Millisecond}
	run := runOf(sampleReport(1, 0), time.Now())

	runner.announce(t.Context(), "kind-spinoza", run, sampleReport(1, 0))

	posts := got.posts()
	if len(posts) != 1 {
		t.Fatalf("posts = %d, want exactly one", len(posts))
	}
	var read auditNotice
	if err := json.Unmarshal(posts[0], &read); err != nil {
		t.Fatalf("the notice did not parse: %v (%s)", err, posts[0])
	}
	if read.New != 1 {
		t.Fatalf("new = %d, want 1", read.New)
	}
}

func TestARefusedNoticeIsTriedOnceMore(t *testing.T) {
	got := newReceiver(1)
	t.Cleanup(got.server.Close)
	runner := &auditRunner{post: got.server.URL, client: got.server.Client(), retryAfter: time.Millisecond}
	run := runOf(sampleReport(1, 0), time.Now())

	runner.announce(t.Context(), "kind-spinoza", run, sampleReport(1, 0))

	if len(got.posts()) != 1 {
		t.Fatalf("posts = %d, want the retry to have landed", len(got.posts()))
	}
}

func TestNoWebhookMeansNoPost(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	runner := &auditRunner{post: "", client: got.server.Client(), retryAfter: time.Millisecond}

	runner.announce(t.Context(), "kind-spinoza", runOf(sampleReport(1, 0), time.Now()), sampleReport(1, 0))

	if len(got.posts()) != 0 {
		t.Fatalf("posts = %d, want none", len(got.posts()))
	}
}

func TestTheScheduleStaysOffWithoutAnInterval(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv.UseAuditSchedule(ctx, AuditSchedule{})
}

func TestTheRunsPageReadsBackWhatWasRecorded(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	held := &heldHistory{}
	held.runs = []store.Run{{ID: 7, At: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), Findings: 4, Fresh: 2, Scanned: 42}}
	srv.UseHistory(t.Context(), held)
	srv.UseAuditEvery(time.Hour)

	rec := httptest.NewRecorder()
	srv.handleAuditRuns(rec, httptest.NewRequest(http.MethodGet, "/api/checks/runs", http.NoBody))

	var page api.AuditRuns
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page did not parse: %v (%s)", err, rec.Body.String())
	}
	if len(page.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(page.Runs))
	}
	if page.Runs[0].ID != "7" || page.Runs[0].Findings != 4 || page.Runs[0].NewCount != 2 {
		t.Fatalf("run = %+v", page.Runs[0])
	}
	if page.Interval != 3600 {
		t.Fatalf("interval = %d, want 3600 seconds", page.Interval)
	}
}

func TestTheRunsPageSaysSoWithNoStore(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)

	rec := httptest.NewRecorder()
	srv.handleAuditRuns(rec, httptest.NewRequest(http.MethodGet, "/api/checks/runs", http.NoBody))

	var page api.AuditRuns
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page did not parse: %v", err)
	}
	if page.Reason == "" {
		t.Fatal("no store and no reason given")
	}
}

func TestARunIsOnlyAnnouncedWhenSomethingMovedSinceTheRunBefore(t *testing.T) {
	same := store.Run{Findings: 4, Fresh: 2, Cleared: 1, Scanned: 42}
	cases := []struct {
		name  string
		now   store.Run
		moved bool
	}{
		{name: "nothing moved", now: same},
		{name: "one more finding", now: store.Run{Findings: 5, Fresh: 2, Cleared: 1, Scanned: 42}, moved: true},
		{name: "something new appeared", now: store.Run{Findings: 4, Fresh: 3, Cleared: 1, Scanned: 42}, moved: true},
		{name: "something cleared", now: store.Run{Findings: 4, Fresh: 2, Cleared: 2, Scanned: 42}, moved: true},
		{name: "the cluster grew", now: store.Run{Findings: 4, Fresh: 2, Cleared: 1, Scanned: 43}, moved: true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := moved(same, one.now); got != one.moved {
				t.Fatalf("moved = %v, want %v", got, one.moved)
			}
		})
	}
}

func TestASecondRunThatFoundTheSameThingsSaysNothing(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	held := &heldHistory{}
	held.runs = []store.Run{{Findings: 4, Fresh: 2, Cleared: 1, Scanned: 42}}
	srv.UseHistory(t.Context(), held)
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	before, known := runner.previous(t.Context(), held, "kind-spinoza")

	if !known {
		t.Fatal("the run before was not read back")
	}
	if moved(before, store.Run{Findings: 4, Fresh: 2, Cleared: 1, Scanned: 42}) {
		t.Fatal("an identical run counted as movement")
	}
}

func TestWithNoRunBeforeItOnlyAnnouncesWhenThereIsSomethingToSay(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	runner := &auditRunner{server: srv, retryAfter: time.Millisecond}

	_, known := runner.previous(t.Context(), nil, "kind-spinoza")

	if known {
		t.Fatal("a missing store reported a run before")
	}
}
