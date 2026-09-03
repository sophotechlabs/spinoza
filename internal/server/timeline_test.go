package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/store"
)

type taped struct {
	notStubbed

	mu      sync.Mutex
	kinds   []resources.Kind
	into    resources.Timeline
	stopped int
}

type stalledTape struct {
	taped

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newStalledTape() *stalledTape {
	return &stalledTape{entered: make(chan struct{}), release: make(chan struct{})}
}

func (r *stalledTape) Record(ctx context.Context, into resources.Timeline, kinds []resources.Kind) {
	r.once.Do(func() {
		close(r.entered)
	})
	select {
	case <-ctx.Done():
		return
	case <-r.release:
	}
	r.taped.Record(ctx, into, kinds)
}

func (r *stalledTape) resume() {
	r.once.Do(func() {})
	select {
	case <-r.release:
	default:
		close(r.release)
	}
}

func (r *taped) Record(_ context.Context, into resources.Timeline, kinds []resources.Kind) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.into = into
	r.kinds = kinds
}

func (r *taped) StopRecording() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped++
	r.into = nil
}

func (r *taped) sink() resources.Timeline {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.into
}

func (r *taped) watching() []resources.Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]resources.Kind{}, r.kinds...)
}

func (r *taped) stops() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func tapingServer(t *testing.T, backend Backend) (*Server, *heldHistory, *heldTabs) {
	t.Helper()
	open := &fleet{
		held:     []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active:   mk1,
		backends: map[string]Backend{mk1: backend},
	}
	srv := New(open, testAssets(), testToken)
	held := &heldHistory{}
	tabs := &heldTabs{tabs: []store.Tab{{ID: mk1, Context: "p-mk1"}}}
	srv.UseHistory(t.Context(), held)
	srv.UseTabs(tabs)
	return srv, held, tabs
}

func awaitNoted(t *testing.T, held *heldHistory, want int) []store.Change {
	t.Helper()
	for range 200 {
		found := held.noted()
		if len(found) >= want {
			return found
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("wanted %d changes written, got %d", want, len(held.noted()))
	return nil
}

func TestTurningTheTimelineOnWatchesTheKindsItNamed(t *testing.T) {
	backend := &taped{}
	srv, _, tabs := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	defer srv.stopRecording(mk1)

	if len(backend.watching()) != len(workloadKinds) {
		t.Fatalf("it is watching %d kinds, wanted %d", len(backend.watching()), len(workloadKinds))
	}
	_ = tabs
}

func TestTheWideSetIsMoreThanTheWorkloadOne(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, timelineWide)
	defer srv.stopRecording(mk1)

	if len(backend.watching()) <= len(workloadKinds) {
		t.Fatalf("the wide set watches %d kinds", len(backend.watching()))
	}
}

func TestAChangeReachesTheStoreWithoutHoldingUpTheInformer(t *testing.T) {
	backend := &taped{}
	srv, held, _ := tapingServer(t, backend)
	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	defer srv.stopRecording(mk1)

	backend.sink().Note(resources.Note{
		At: time.Now(), Verb: resources.Changed, Resource: "pods", Kind: "Pod",
		Namespace: "web", Name: "api-1", UID: "uid-1", Cells: []string{"1/1", "Running"},
	})

	found := awaitNoted(t, held, 1)
	if found[0].Name != "api-1" || found[0].Cluster != mk1 {
		t.Fatalf("the change was written as %+v", found[0])
	}
}

func TestChangesArriveInOneBatchRatherThanOneWriteEach(t *testing.T) {
	backend := &taped{}
	srv, held, _ := tapingServer(t, backend)
	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	defer srv.stopRecording(mk1)

	into := backend.sink()
	for at := range 20 {
		into.Note(resources.Note{At: time.Now(), Verb: resources.Added, Name: "pod", UID: string(rune('a' + at))})
	}

	awaitNoted(t, held, 20)
}

func TestARejectedTimelineWriteIsNotCountedAsStored(t *testing.T) {
	held := &heldHistory{recordErr: errors.New("the timeline is read-only")}
	pruned := 0
	recording := &recording{
		into: held.Timeline(mk1),
		prune: func(context.Context) {
			pruned++
		},
		written: pruneEvery - 1,
	}

	recording.write(t.Context(), []store.Change{{Name: "api-1"}})

	if len(held.noted()) != 0 {
		t.Fatalf("stored = %+v, want the rejected change left out", held.noted())
	}
	if recording.written != pruneEvery-1 {
		t.Fatalf("written = %d, want failed writes left uncounted", recording.written)
	}
	if pruned != 0 {
		t.Fatalf("pruned = %d, want no retention pass after a failed write", pruned)
	}
}

func TestEnoughStoredChangesTriggerAnotherRetentionPass(t *testing.T) {
	held := &heldHistory{}
	pruned := 0
	recording := &recording{
		into: held.Timeline(mk1),
		prune: func(context.Context) {
			pruned++
		},
		written: pruneEvery - 1,
	}

	recording.write(t.Context(), []store.Change{{Name: "api-1"}})

	if len(held.noted()) != 1 {
		t.Fatalf("stored = %+v, want the change written", held.noted())
	}
	if recording.written != 0 {
		t.Fatalf("written = %d, want the retention counter reset", recording.written)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want one retention pass", pruned)
	}
}

func TestTheTimelineIsTrimmedAsItIsWrittenTo(t *testing.T) {
	backend := &taped{}
	srv, held, _ := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	defer srv.stopRecording(mk1)

	for range 200 {
		if len(held.trims()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	trims := held.trims()
	if len(trims) == 0 {
		t.Fatal("nothing was trimmed when recording started")
	}
	if trims[0].Days != timelineDays || trims[0].Rows != timelineRows {
		t.Fatalf("it trimmed to %+v", trims[0])
	}
}

func TestTheAuditIsBoundedByTheSamePassThatTrimsTheTimeline(t *testing.T) {
	backend := &taped{}
	srv, taped, _ := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	defer srv.stopRecording(mk1)

	for range 200 {
		if len(taped.auditTrims()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	trims := taped.auditTrims()
	if len(trims) == 0 {
		t.Fatal("the audit was never trimmed, so it grows without bound")
	}
	if trims[0].Days != auditDays || trims[0].Rows != auditRows {
		t.Fatalf("it trimmed the audit to %+v", trims[0])
	}
}

func TestTimelineAndAuditPruneFailuresDoNotStopEachOther(t *testing.T) {
	srv, held, _ := tapingServer(t, &taped{})
	held.pruneErr = errors.New("timeline table is locked")
	held.auditErr = errors.New("audit table is locked")
	auditBefore := len(held.auditTrims())

	srv.pruneTimeline(t.Context())

	if len(held.trims()) != 1 {
		t.Fatalf("timeline prune calls = %d, want 1", len(held.trims()))
	}
	if len(held.auditTrims()) != auditBefore+1 {
		t.Fatalf("audit prune calls = %d, want %d", len(held.auditTrims()), auditBefore+1)
	}
}

func TestHowLongToKeepTheTimelineComesFromTheSettings(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	err := srv.stored().Merge(map[string]string{timelineDaysKey: "30"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if srv.keepDays() != 30 {
		t.Fatalf("it keeps %d days", srv.keepDays())
	}
}

func TestAnAbsurdRetentionIsCappedRatherThanObeyed(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	err := srv.stored().Merge(map[string]string{timelineDaysKey: "4000"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if srv.keepDays() != timelineMaxDays {
		t.Fatalf("it keeps %d days", srv.keepDays())
	}
}

func TestNonsenseRetentionFallsBackToTheDefault(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	err := srv.stored().Merge(map[string]string{timelineDaysKey: "soon"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if srv.keepDays() != timelineDays {
		t.Fatalf("it keeps %d days", srv.keepDays())
	}
}

func TestTurningTheTimelineOffStopsWatching(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)
	srv.startRecording(t.Context(), mk1, timelineWorkloads)

	srv.stopRecording(mk1)

	if backend.stops() != 1 {
		t.Fatalf("the backend was told to stop %d times", backend.stops())
	}
	if srv.recordingOn(mk1) != nil {
		t.Fatal("the recording is still held")
	}
}

func TestClosingTheServerStopsAndFlushesRecordings(t *testing.T) {
	backend := &taped{}
	srv, held, _ := tapingServer(t, backend)
	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	backend.sink().Note(resources.Note{Name: "waiting"})

	srv.Close()

	if backend.stops() != 1 {
		t.Fatalf("the backend was told to stop %d times", backend.stops())
	}
	if srv.recordingOn(mk1) != nil {
		t.Fatal("the recording is still held after shutdown")
	}
	found := held.noted()
	if len(found) != 1 || found[0].Name != "waiting" {
		t.Fatalf("shutdown kept changes %+v", found)
	}
}

func TestClosingTheServerCancelsARecordingThatIsStillStarting(t *testing.T) {
	backend := newStalledTape()
	srv, _, _ := tapingServer(t, backend)
	started := make(chan struct{})
	closed := make(chan struct{})
	var closeOnce sync.Once
	closeServer := func() {
		closeOnce.Do(func() {
			go func() {
				srv.Close()
				close(closed)
			}()
		})
	}
	t.Cleanup(func() {
		backend.resume()
		closeServer()
		<-started
		<-closed
	})
	go func() {
		srv.startRecording(t.Context(), mk1, timelineWorkloads)
		close(started)
	}()
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("recording startup never reached the backend")
	}
	closeServer()

	select {
	case <-closed:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("recording startup blocked server shutdown")
	}
}

func TestARecordingCannotStartAfterServerShutdown(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)
	srv.Close()

	srv.startRecording(t.Context(), mk1, timelineWorkloads)

	if backend.sink() != nil {
		t.Fatal("a recording started after shutdown")
	}
	if srv.recordingOn(mk1) != nil {
		t.Fatal("the server retained a recording after shutdown")
	}
}

func TestFlushingARecordingDrainsEveryQueuedChange(t *testing.T) {
	held := &heldHistory{}
	recording := &recording{
		into:  held.Timeline(mk1),
		queue: make(chan store.Change, timelineQueue),
	}
	for at := range timelineBatch + 1 {
		recording.queue <- store.Change{Name: fmt.Sprintf("pod-%d", at)}
	}

	recording.flush(t.Context())

	if got := len(held.noted()); got != timelineBatch+1 {
		t.Fatalf("flush wrote %d changes", got)
	}
}

func TestStoppingSomethingThatWasNotRecordingIsQuiet(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)

	srv.stopRecording(mk1)

	if backend.stops() != 0 {
		t.Fatalf("a backend that was not recording was told to stop %d times", backend.stops())
	}
}

func TestTurningItOnTwiceLeavesOneRecording(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, timelineWorkloads)
	first := srv.recordingOn(mk1)
	srv.startRecording(t.Context(), mk1, timelineWide)
	defer srv.stopRecording(mk1)

	if srv.recordingOn(mk1) == first {
		t.Fatal("the second start reused the first recording")
	}
}

func TestAKindSetNobodyKnowsRecordsNothing(t *testing.T) {
	backend := &taped{}
	srv, _, _ := tapingServer(t, backend)

	srv.startRecording(t.Context(), mk1, "everything and the kitchen sink")

	if srv.recordingOn(mk1) != nil {
		t.Fatal("an unknown kind set started a recording")
	}
}

func TestRecordingNeedsSomewhereToWriteAndSomethingToWatch(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	srv.UseHistory(t.Context(), nil)

	srv.startRecording(t.Context(), mk1, timelineWorkloads)

	if srv.recordingOn(mk1) != nil {
		t.Fatal("it started recording with nowhere to write")
	}
}

func TestATabThatWasRecordingIsRecordingAgainNextTime(t *testing.T) {
	backend := &taped{}
	srv, _, tabs := tapingServer(t, backend)
	tabs.tabs[0].Timeline = timelineWorkloads

	srv.StartRecordings(t.Context())
	defer srv.stopRecording(mk1)

	if srv.recordingOn(mk1) == nil {
		t.Fatal("the tab did not come back recording")
	}
}

func TestRestoringTabsInstallsThemBeforeRecordingsResume(t *testing.T) {
	backend := &taped{}
	srv, _, tabs := tapingServer(t, backend)
	tabs.tabs[0].Timeline = timelineWorkloads
	srv.UseTabs(nil)

	srv.RestoreTabs(t.Context(), tabs)
	defer srv.stopRecording(mk1)

	if srv.recordingOn(mk1) == nil {
		t.Fatal("the restored tab did not resume recording")
	}
}

func TestATabThatWasNotRecordingStaysQuiet(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})

	srv.StartRecordings(t.Context())

	if srv.recordingOn(mk1) != nil {
		t.Fatal("a tab that was not recording started on its own")
	}
}

func TestATabWhoseClusterIsNotOpenIsSkipped(t *testing.T) {
	srv, _, tabs := tapingServer(t, &taped{})
	tabs.tabs = append(tabs.tabs, store.Tab{ID: mk2, Context: "p-mk2", Timeline: timelineWorkloads})

	srv.StartRecordings(t.Context())

	if srv.recordingOn(mk2) != nil {
		t.Fatal("a cluster nobody opened started recording")
	}
}

func recordRoute(t *testing.T, srv *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/clusters/timeline?"+query, http.NoBody)
	rec := httptest.NewRecorder()
	srv.recordCluster(rec, req)
	return rec
}

func TestAskingATabToRecordRemembersItAndStarts(t *testing.T) {
	backend := &taped{}
	srv, _, tabs := tapingServer(t, backend)

	rec := recordRoute(t, srv, "cluster="+mk1+"&kinds="+timelineWorkloads)
	defer srv.stopRecording(mk1)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if tabs.tabs[0].Timeline != timelineWorkloads {
		t.Fatalf("the tab remembers %q", tabs.tabs[0].Timeline)
	}
	if srv.recordingOn(mk1) == nil {
		t.Fatal("nothing is recording")
	}
}

func TestAskingATabToStopRemembersThatToo(t *testing.T) {
	backend := &taped{}
	srv, _, tabs := tapingServer(t, backend)
	recordRoute(t, srv, "cluster="+mk1+"&kinds="+timelineWorkloads)

	rec := recordRoute(t, srv, "cluster="+mk1+"&kinds=")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if tabs.tabs[0].Timeline != timelineOff {
		t.Fatalf("the tab remembers %q", tabs.tabs[0].Timeline)
	}
	if srv.recordingOn(mk1) != nil {
		t.Fatal("it is still recording")
	}
}

func TestAKindSetTheServerDoesNotKnowIsRefused(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})

	rec := recordRoute(t, srv, "cluster="+mk1+"&kinds=everything")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRecordingWithNoClusterNamedIsRefused(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	held, ok := srv.cluster.(*fleet)
	if !ok {
		t.Fatal("the test server is not holding a fleet")
	}
	held.active = ""

	rec := recordRoute(t, srv, "kinds="+timelineWorkloads)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRecordingWithNowhereToRememberItIsRefused(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	srv.UseTabs(nil)

	rec := recordRoute(t, srv, "cluster="+mk1+"&kinds="+timelineWorkloads)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestARecordingThatCannotBeRememberedIsNotStarted(t *testing.T) {
	srv, _, tabs := tapingServer(t, &taped{})
	tabs.setErr = errBadColor

	rec := recordRoute(t, srv, "cluster="+mk1+"&kinds="+timelineWorkloads)

	if rec.Code == http.StatusOK {
		t.Fatal("a setting that was not stored was acted on")
	}
	if srv.recordingOn(mk1) != nil {
		t.Fatal("it started recording anyway")
	}
}

func TestChangesTheTimelineCouldNotKeepUpWithAreCounted(t *testing.T) {
	held := &recording{queue: make(chan store.Change, 1)}

	held.Note(resources.Note{Name: "first"})
	held.Note(resources.Note{Name: "second"})

	if held.dropped.Load() != 1 {
		t.Fatalf("it dropped %d", held.dropped.Load())
	}
}

func TestDroppedChangesAreReportedForTheClusterRecordingThem(t *testing.T) {
	srv, _, _ := tapingServer(t, &taped{})
	held := &recording{}
	held.dropped.Store(7)
	srv.holdRecording(mk1, held)
	t.Cleanup(func() {
		srv.holdRecording(mk1, nil)
	})

	if got := srv.droppedOn(mk1); got != 7 {
		t.Fatalf("dropped = %d, want 7", got)
	}
	if got := srv.droppedOn(mk2); got != 0 {
		t.Fatalf("other cluster dropped = %d, want 0", got)
	}
}

func mergingServer(t *testing.T, held *heldHistory) *httptest.Server {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.UseHistory(t.Context(), held)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func bothKinds() *heldHistory {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return &heldHistory{
		page: store.Page{Entries: []store.Entry{
			{ID: 1, At: at.Add(2 * time.Minute), Verb: "scale", Name: "web", Outcome: api.HistoryDone},
		}},
		changePage: store.Changes{Rows: []store.Change{
			{ID: 1, At: at.Add(time.Minute), Verb: store.Changed, Name: "web-1", Cells: []string{"1/1", "Running"}},
			{ID: 2, At: at, Verb: store.Added, Name: "web-2"},
		}},
	}
}

func askHistory(t *testing.T, ts *httptest.Server, query string) api.History {
	t.Helper()
	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history"+query, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	return readBack(t, body)
}

func TestHistoryHoldsWhatSpinozaDidAndWhatTheClusterDid(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "")

	if len(got.Entries) != 3 {
		t.Fatalf("entries = %d, want all three", len(got.Entries))
	}
	if got.Entries[0].Source != api.HistoryAction || got.Entries[0].Name != "web" {
		t.Fatalf("the newest row was %+v", got.Entries[0])
	}
	if got.Entries[1].Source != api.HistoryChange || got.Entries[1].Name != "web-1" {
		t.Fatalf("the second row was %+v", got.Entries[1])
	}
}

func TestAskingForOnlyWhatSpinozaDidLeavesTheClusterOut(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "?source=action")

	if len(got.Entries) != 1 || got.Entries[0].Source != api.HistoryAction {
		t.Fatalf("entries = %+v", got.Entries)
	}
}

func TestAskingForOnlyWhatTheClusterDidLeavesSpinozaOut(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "?source=change")

	if len(got.Entries) != 2 {
		t.Fatalf("entries = %+v", got.Entries)
	}
	for _, one := range got.Entries {
		if one.Source != api.HistoryChange {
			t.Fatalf("a row from the wrong table: %+v", one)
		}
	}
}

func TestAChangeRowShowsWhatTheTableShows(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "?source=change")

	if got.Entries[0].Detail != "1/1 · Running" {
		t.Fatalf("the row read %q", got.Entries[0].Detail)
	}
}

func TestASourceNobodyKnowsIsRefused(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history?source=hearsay", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestTheMergedPageHoldsTheSameCapOneSideDoes(t *testing.T) {
	held := bothKinds()
	ts := mergingServer(t, held)

	got := askHistory(t, ts, "?limit=2")

	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want the limit kept", len(got.Entries))
	}
	if !got.More {
		t.Fatal("the merge dropped a row and did not say so")
	}
}

func TestTheMergeSaysWhenEitherSideHasMore(t *testing.T) {
	held := bothKinds()
	held.page.More = true
	ts := mergingServer(t, held)

	got := askHistory(t, ts, "")

	if !got.More {
		t.Fatal("one side had more and the merge did not say so")
	}
}

func TestTheChangeReadFailingIsReported(t *testing.T) {
	held := bothKinds()
	held.changeErr = errBadColor
	ts := mergingServer(t, held)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history?source=change", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d, want the failure reported", resp.StatusCode)
	}
}

func TestTheActionReadFailingIsReportedEvenWhenChangesRead(t *testing.T) {
	held := bothKinds()
	held.readErr = errBadColor
	ts := mergingServer(t, held)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d, want the failure reported", resp.StatusCode)
	}
}

func TestTwoRowsStampedTheSameInstantStillHaveAnOrder(t *testing.T) {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	held := &heldHistory{
		page: store.Page{Entries: []store.Entry{{ID: 1, At: at, Name: "did"}}},
		changePage: store.Changes{Rows: []store.Change{
			{ID: 2, At: at, Name: "changed-late"},
			{ID: 1, At: at, Name: "changed-early"},
		}},
	}
	ts := mergingServer(t, held)

	got := askHistory(t, ts, "")

	if got.Entries[0].Name != "did" {
		t.Fatalf("the order came back as %+v", got.Entries)
	}
	if got.Entries[1].Name != "changed-late" {
		t.Fatalf("two changes at one instant came back as %+v", got.Entries[1:])
	}
}

func TestAChangeRowSaysWhatItMovedFrom(t *testing.T) {
	held := bothKinds()
	held.changePage.Rows[0].Was = []string{"2/2", "Running"}
	ts := mergingServer(t, held)

	got := askHistory(t, ts, "?source=change")

	if got.Entries[0].Was != "2/2 · Running" {
		t.Fatalf("was = %q", got.Entries[0].Was)
	}
}

func TestEveryHistoryRowSaysWhichClusterItIsOn(t *testing.T) {
	held := bothKinds()
	held.page.Entries[0].Cluster = mk1
	held.changePage.Rows[0].Cluster = mk2
	ts := mergingServer(t, held)

	got := askHistory(t, ts, "")

	on := map[string]bool{}
	for _, one := range got.Entries {
		on[one.Cluster] = true
	}
	if !on[mk1] || !on[mk2] {
		t.Fatalf("clusters = %+v", on)
	}
}

func TestTheFleetRollupAsksTheStoreForEveryCluster(t *testing.T) {
	held := bothKinds()
	ts := mergingServer(t, held)

	askHistory(t, ts, "?fleet=true&source=change")

	if held.asked().Cluster != "" {
		t.Fatalf("it asked for %q, want every cluster", held.asked().Cluster)
	}
}

func TestOneClustersHistoryStillAsksForThatCluster(t *testing.T) {
	held := bothKinds()
	ts := mergingServer(t, held)

	askHistory(t, ts, "?source=change")

	if held.asked().Cluster == "" {
		t.Fatalf("it asked for every cluster when one was meant")
	}
}

func TestAPageOfChangesCanBeContinued(t *testing.T) {
	held := bothKinds()
	ts := mergingServer(t, held)

	askHistory(t, ts, "?source=change&after=40")

	if held.asked().After != 40 {
		t.Fatalf("after = %d", held.asked().After)
	}
}

func TestAContinuedPageSaysWhereItEnded(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "?source=change")

	if got.Next != 2 {
		t.Fatalf("next = %d, want the oldest change on the page", got.Next)
	}
}

func TestAContinuedPageStillReachesTheActions(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	got := askHistory(t, ts, "?after=1")

	actions := 0
	for _, one := range got.Entries {
		if one.Source == api.HistoryAction {
			actions++
		}
	}
	if actions == 0 {
		t.Fatal("a continued page dropped the audit half, which is how a row becomes unreachable")
	}
}

func inOrder() *heldHistory {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return &heldHistory{
		page: store.Page{Entries: []store.Entry{
			{ID: 2, At: at.Add(3 * time.Minute), Verb: "scale", Name: "web", Outcome: api.HistoryDone},
			{ID: 1, At: at.Add(time.Minute), Verb: "scale", Name: "api", Outcome: api.HistoryDone},
		}},
		changePage: store.Changes{Rows: []store.Change{
			{ID: 2, At: at.Add(2 * time.Minute), Verb: store.Changed, Name: "web-2"},
			{ID: 1, At: at, Verb: store.Added, Name: "web-1"},
		}},
	}
}

func TestEachHalfIsBoundedByItsOwnCursor(t *testing.T) {
	ts := mergingServer(t, inOrder())

	first := askHistory(t, ts, "?limit=2")
	if first.Next == 0 || first.NextAction == 0 {
		t.Fatalf("a merged page offered next=%d nextAction=%d", first.Next, first.NextAction)
	}

	rest := askHistory(t, ts, fmt.Sprintf("?after=%d&afterAction=%d", first.Next, first.NextAction))

	for _, one := range rest.Entries {
		for _, seen := range first.Entries {
			if one.Source == seen.Source && one.ID == seen.ID {
				t.Fatalf("%s %d came back on both pages", one.Source, one.ID)
			}
		}
	}
}

func TestAHalfWithNothingOnThePageKeepsTheCursorItCameWith(t *testing.T) {
	ts := mergingServer(t, inOrder())

	got := askHistory(t, ts, "?afterAction=1")

	for _, one := range got.Entries {
		if one.Source == api.HistoryAction {
			t.Fatalf("an action came back below the cursor: %+v", one)
		}
	}
	if got.NextAction != 1 {
		t.Fatalf("nextAction = %d, want the cursor it came in with when no action was shown",
			got.NextAction)
	}
}

func TestACursorThatIsNotANumberIsRefused(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history?after=soon", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestACursorBelowZeroIsRefused(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history?after=-4", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestAnActionCursorThatIsInvalidIsRefused(t *testing.T) {
	ts := mergingServer(t, bothKinds())

	for _, cursor := range []string{"soon", "-4"} {
		resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/history?afterAction="+cursor, nil)

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("cursor %q status = %d", cursor, resp.StatusCode)
		}
	}
}
