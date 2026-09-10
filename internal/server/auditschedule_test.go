package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/safe"
	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
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
	tried   int
}

func newReceiver(refuse int) *countingReceiver {
	got := &countingReceiver{refuse: refuse}
	got.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		got.mu.Lock()
		defer got.mu.Unlock()
		got.tried++
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

func (c *countingReceiver) tries() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tried
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

type auditingBackend struct {
	*stubCatalog

	report api.CheckReport
}

func (a *auditingBackend) CheckExport(context.Context, checks.Filter) api.CheckReport {
	return a.report
}

func scheduledServer(t *testing.T) (*Server, *heldHistory) {
	t.Helper()
	backend := &auditingBackend{stubCatalog: &stubCatalog{}, report: sampleReport(1, 0)}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	srv.UseBaselines(newHeldBaselines())
	held := &heldHistory{}
	srv.UseHistory(t.Context(), held)
	return srv, held
}

func TestTheSelfMetricsPageCarriesTheExpositionFormat(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)

	rec := httptest.NewRecorder()
	srv.handleSelfMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, wanted := range []string{
		"# TYPE spinoza_http_requests_total counter",
		"spinoza_build_info{version=",
	} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("the page does not carry %q", wanted)
		}
	}
}

func TestAScheduledRunRecordsWhatItFoundAndAnnouncesTheChange(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	srv, held := scheduledServer(t)
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())

	if len(held.runs) != 1 {
		t.Fatalf("recorded %d runs, want 1", len(held.runs))
	}
	if held.runsOn[0] == "" {
		t.Fatal("the run was recorded against no cluster")
	}
	if len(got.posts()) != 1 {
		t.Fatalf("posts = %d, want the first run to be announced", len(got.posts()))
	}
}

func TestASecondRunThatFoundNothingNewSaysNothing(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	srv, held := scheduledServer(t)
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())
	runner.once(t.Context())

	if len(held.runs) != 2 {
		t.Fatalf("recorded %d runs, want both", len(held.runs))
	}
	if len(got.posts()) != 1 {
		t.Fatalf("posts = %d, want only the first", len(got.posts()))
	}
}

func TestARunAgainstAClusterThatIsNotThereIsSkipped(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	held := &heldHistory{}
	srv.UseHistory(t.Context(), held)
	runner := &auditRunner{server: srv, retryAfter: time.Millisecond}

	runner.runOn(t.Context(), "not-open")

	if len(held.runs) != 0 {
		t.Fatalf("recorded %d runs for a cluster that is not open", len(held.runs))
	}
}

func TestARunIsStillAnnouncedWhenTheStoreCannotKeepIt(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	srv, held := scheduledServer(t)
	held.runsErr = errBadLimit
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())

	if len(got.posts()) != 1 {
		t.Fatalf("posts = %d; a store that refused the run stopped the notice", len(got.posts()))
	}
}

func TestTheTimerStopsWhenTheProcessDoes(t *testing.T) {
	srv, held := scheduledServer(t)
	ctx, stop := context.WithCancel(t.Context())
	srv.UseAuditSchedule(ctx, AuditSchedule{Every: 5 * time.Millisecond})

	waitForRun(t, func() bool { return len(held.recordedRuns()) > 0 })
	stop()
	settled := len(held.recordedRuns())
	time.Sleep(30 * time.Millisecond)

	if grown := len(held.recordedRuns()); grown > settled+1 {
		t.Fatalf("the timer kept running after the process stopped: %d then %d", settled, grown)
	}
}

func waitForRun(t *testing.T, until func() bool) {
	t.Helper()
	for range 200 {
		if until() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the timer never ran")
}

func TestAFirstRunThatFoundNothingSaysNothing(t *testing.T) {
	got := newReceiver(0)
	t.Cleanup(got.server.Close)
	backend := &auditingBackend{stubCatalog: &stubCatalog{}, report: api.CheckReport{Scanned: 4}}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	srv.UseBaselines(newHeldBaselines())
	srv.UseHistory(t.Context(), &heldHistory{})
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())

	if len(got.posts()) != 0 {
		t.Fatalf("posts = %d, want none for a first clean run", len(got.posts()))
	}
}

func TestANoticeThatIsRefusedTwiceIsGivenUpOn(t *testing.T) {
	got := newReceiver(2)
	t.Cleanup(got.server.Close)
	srv, _ := scheduledServer(t)
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())

	if got.tries() != 2 {
		t.Fatalf("tries = %d, want the first and one retry", got.tries())
	}
	if len(got.posts()) != 0 {
		t.Fatalf("posts = %d, want none to have landed", len(got.posts()))
	}
}

func TestANoticeIsNotRetriedOnceTheProcessIsStopping(t *testing.T) {
	got := newReceiver(2)
	t.Cleanup(got.server.Close)
	srv, _ := scheduledServer(t)
	ctx, stop := context.WithCancel(t.Context())
	runner := &auditRunner{
		server:     srv,
		post:       got.server.URL,
		client:     got.server.Client(),
		retryAfter: time.Minute,
	}
	safe.Go("stopping the process while a notice is waiting to retry", func() {
		waitForPost(got)
		stop()
	})

	runner.runOn(ctx, "kind-spinoza")

	if got.tries() != 1 {
		t.Fatalf("tries = %d, want the retry abandoned", got.tries())
	}
}

func waitForPost(got *countingReceiver) {
	for range 400 {
		if got.tries() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestANoticeToAnAddressThatIsNotAURLIsNotSent(t *testing.T) {
	srv, _ := scheduledServer(t)
	runner := &auditRunner{
		server:     srv,
		post:       "http://[::1]:namedport/hook",
		client:     &http.Client{},
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())
}

func TestANoticeToAnAddressThatRefusesTheConnectionIsNotFatal(t *testing.T) {
	dead := newReceiver(0)
	url := dead.server.URL
	dead.server.Close()
	srv, _ := scheduledServer(t)
	runner := &auditRunner{
		server:     srv,
		post:       url,
		client:     &http.Client{Timeout: 200 * time.Millisecond},
		retryAfter: time.Millisecond,
	}

	runner.once(t.Context())
}

func TestTheRunsPageSaysWhyWhenTheStoreCannotBeRead(t *testing.T) {
	srv, held := scheduledServer(t)
	held.runsErr = errBadLimit

	rec := httptest.NewRecorder()
	srv.handleAuditRuns(rec, httptest.NewRequest(http.MethodGet, "/api/audit/runs", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
}
