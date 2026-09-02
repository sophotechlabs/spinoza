package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/store"
)

const (
	timelineOff       = ""
	timelineWorkloads = "workloads"
	timelineWide      = "wide"
)

const (
	timelineQueue    = 2048
	timelineBatch    = 256
	timelineRows     = 200000
	timelineDays     = 7
	timelineMaxDays  = 90
	auditRows        = 50000
	auditDays        = 90
	pruneEvery       = 2000
	timelineWrite    = 15 * time.Second
	timelineDaysKey  = "timelineDays"
	timelineDropWarn = 1000
)

var errBadTimeline = errors.New("kinds must be workloads, wide, or empty to stop")

var workloadKinds = []resources.Kind{
	{Group: "", Resource: "pods"},
	{Group: "apps", Resource: "deployments"},
	{Group: "apps", Resource: "statefulsets"},
	{Group: "apps", Resource: "daemonsets"},
	{Group: "apps", Resource: "replicasets"},
	{Group: "batch", Resource: "jobs"},
	{Group: "batch", Resource: "cronjobs"},
}

var alsoWideKinds = []resources.Kind{
	{Group: "", Resource: "services"},
	{Group: "", Resource: "nodes"},
	{Group: "networking.k8s.io", Resource: "ingresses"},
	{Group: "kustomize.toolkit.fluxcd.io", Resource: "kustomizations"},
	{Group: "helm.toolkit.fluxcd.io", Resource: "helmreleases"},
	{Group: "source.toolkit.fluxcd.io", Resource: "gitrepositories"},
	{Group: "source.toolkit.fluxcd.io", Resource: "helmrepositories"},
	{Group: "source.toolkit.fluxcd.io", Resource: "ocirepositories"},
	{Group: "argoproj.io", Resource: "applications"},
}

func kindsNamed(name string) ([]resources.Kind, bool) {
	if name == timelineWorkloads {
		return workloadKinds, true
	}
	if name == timelineWide {
		return append(append([]resources.Kind{}, workloadKinds...), alsoWideKinds...), true
	}
	return nil, false
}

type recording struct {
	into    store.Noter
	backend Backend
	prune   func(ctx context.Context)
	queue   chan store.Change
	stop    chan struct{}
	stopped chan struct{}
	started chan struct{}
	cancel  context.CancelFunc
	once    sync.Once
	dropped atomic.Int64
	written int
}

func (t *recording) Note(note resources.Note) {
	select {
	case t.queue <- noteOf(note):
	default:
		count := t.dropped.Add(1)
		if count%timelineDropWarn == 0 {
			slog.Warn("the timeline is behind and is dropping changes", "dropped", count)
		}
	}
}

func noteOf(note resources.Note) store.Change {
	return store.Change{
		At:        note.At,
		Verb:      note.Verb,
		Group:     note.Group,
		Version:   note.Version,
		Resource:  note.Resource,
		Kind:      note.Kind,
		Namespace: note.Namespace,
		Name:      note.Name,
		UID:       note.UID,
		Cells:     note.Cells,
		Was:       note.Was,
	}
}

func (t *recording) run(ctx context.Context) {
	defer close(t.stopped)
	t.pruneWithin(ctx)
	for {
		select {
		case <-t.stop:
			t.flush(ctx)
			return
		case first := <-t.queue:
			t.write(ctx, t.fill(first))
		}
	}
}

func (t *recording) flush(ctx context.Context) {
	draining, cancel := context.WithTimeout(ctx, timelineWrite)
	defer cancel()
	for {
		select {
		case first := <-t.queue:
			t.write(draining, t.fill(first))
		default:
			return
		}
	}
}

func (t *recording) pruneWithin(ctx context.Context) {
	pruning, cancel := context.WithTimeout(ctx, timelineWrite)
	defer cancel()
	t.prune(pruning)
}

func (t *recording) fill(first store.Change) []store.Change {
	batch := make([]store.Change, 0, timelineBatch)
	batch = append(batch, first)
	for len(batch) < timelineBatch {
		select {
		case next := <-t.queue:
			batch = append(batch, next)
		default:
			return batch
		}
	}
	return batch
}

func (t *recording) write(ctx context.Context, batch []store.Change) {
	writing, cancel := context.WithTimeout(ctx, timelineWrite)
	defer cancel()
	err := t.into.Note(writing, batch)
	if err != nil {
		slog.Warn("what changed on the cluster was not recorded", "changes", len(batch), "error", err)
		return
	}
	t.written += len(batch)
	if t.written < pruneEvery {
		return
	}
	t.written = 0
	t.pruneWithin(ctx)
}

func (t *recording) close() {
	t.cancelStart()
	t.waitForStart()
	t.stopBackend()
	t.requestStop()
	t.waitForStop()
}

func (t *recording) cancelStart() {
	if t.cancel != nil {
		t.cancel()
	}
}

func (t *recording) waitForStart() {
	if t.started != nil {
		<-t.started
	}
}

func (t *recording) stopBackend() {
	if t.backend != nil {
		t.backend.StopRecording()
	}
}

func (t *recording) requestStop() {
	t.once.Do(func() { close(t.stop) })
}

func (t *recording) waitForStop() {
	<-t.stopped
}

func (s *Server) recordingOn(id string) *recording {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taping[id]
}

func (s *Server) holdRecording(id string, held *recording) *recording {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.taping[id]
	if held == nil {
		delete(s.taping, id)
		return was
	}
	s.taping[id] = held
	return was
}

func (s *Server) keepDays() int {
	days := timelineDays
	raw := s.stored().All()[timelineDaysKey]
	asked, err := strconv.Atoi(raw)
	if err == nil && asked > 0 {
		days = asked
	}
	if days > timelineMaxDays {
		days = timelineMaxDays
	}
	return days
}

func (s *Server) startRecording(ctx context.Context, id, name string) {
	kinds, ok := kindsNamed(name)
	if !ok {
		s.stopRecording(id)
		return
	}
	past := s.recorder()
	backend := s.managerOf(id)
	if past == nil || backend == nil {
		return
	}
	outlives := auth.AsServer(context.WithoutCancel(ctx))
	starting, cancel := context.WithCancel(outlives)
	held := &recording{
		into:    past.Timeline(id),
		backend: backend,
		queue:   make(chan store.Change, timelineQueue),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		started: make(chan struct{}),
		cancel:  cancel,
	}
	held.prune = func(pruning context.Context) {
		s.pruneTimeline(pruning)
	}
	s.tapeMu.Lock()
	if s.tapingClosed {
		s.tapeMu.Unlock()
		cancel()
		return
	}
	go held.run(outlives)
	was := s.holdRecording(id, held)
	s.tapeMu.Unlock()
	defer close(held.started)
	if was != nil {
		was.close()
	}
	backend.Record(starting, held, kinds)
}

func (s *Server) pruneTimeline(ctx context.Context) {
	past := s.recorder()
	if past == nil {
		return
	}
	err := past.Prune(ctx, store.Retention{Days: s.keepDays(), Rows: timelineRows}, s.instant())
	if err != nil {
		slog.Warn("the timeline could not be trimmed", "error", err)
	}
	auditErr := past.PruneAudit(ctx, store.Retention{Days: auditDays, Rows: auditRows}, s.instant())
	if auditErr != nil {
		slog.Warn("the audit could not be trimmed", "error", auditErr)
	}
}

func (s *Server) stopRecording(id string) {
	s.tapeMu.Lock()
	defer s.tapeMu.Unlock()
	s.stopRecordingLocked(id)
}

func (s *Server) stopRecordingLocked(id string) {
	was := s.holdRecording(id, nil)
	if was == nil {
		return
	}
	was.close()
}

func (s *Server) closeRecordings() {
	s.tapeMu.Lock()
	s.tapingClosed = true
	s.mu.Lock()
	held := s.taping
	s.taping = map[string]*recording{}
	s.mu.Unlock()
	s.tapeMu.Unlock()
	for _, recording := range held {
		recording.cancelStart()
	}
	for _, recording := range held {
		recording.waitForStart()
	}
	for _, recording := range held {
		recording.stopBackend()
	}
	for _, recording := range held {
		recording.requestStop()
	}
	for _, recording := range held {
		recording.waitForStop()
	}
}

func (s *Server) StartRecordings(ctx context.Context) {
	for id, tab := range s.tabsByID(ctx) {
		if tab.Timeline == timelineOff {
			continue
		}
		if s.managerOf(id) == nil {
			continue
		}
		s.startRecording(ctx, id, tab.Timeline)
	}
}

func (s *Server) RestoreTabs(ctx context.Context, held Tabs) {
	s.UseTabs(held)
	s.RememberOpen(ctx)
	s.StartRecordings(ctx)
}

func (s *Server) recordCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	name := r.URL.Query().Get("kinds")
	_, ok := kindsNamed(name)
	if !ok && name != timelineOff {
		writeError(w, http.StatusBadRequest, errBadTimeline.Error())
		return
	}
	held := s.tabs()
	if held == nil {
		writeError(w, http.StatusServiceUnavailable, errNowhereToKeepIt.Error())
		return
	}
	err := held.Recording(r.Context(), id, name)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if name == timelineOff {
		s.stopRecording(id)
	} else {
		s.startRecording(r.Context(), id, name)
	}
	writeJSON(w, s.clusterList(r.Context()))
}

func (s *Server) readChanges(
	w http.ResponseWriter, r *http.Request, limit int, after int64, on string,
) (api.History, bool) {
	page, readErr := s.recorder().Changed(
		r.Context(), store.Query{Cluster: on, Limit: limit, After: after},
	)
	if readErr != nil {
		writeAPIError(w, readErr)
		return api.History{}, false
	}
	rows := changesOf(page.Rows)
	return api.History{
		Entries: rows,
		More:    page.More,
		Dropped: s.droppedOn(on),
		Next:    lastOf(rows, api.HistoryChange, after),
	}, true
}

func (s *Server) droppedOn(id string) int {
	held := s.recordingOn(id)
	if held == nil {
		return 0
	}
	return int(held.dropped.Load())
}

func changesOf(held []store.Change) []api.HistoryEntry {
	out := make([]api.HistoryEntry, 0, len(held))
	for _, one := range held {
		out = append(out, api.HistoryEntry{
			ID:        one.ID,
			Source:    api.HistoryChange,
			Cluster:   one.Cluster,
			At:        one.At.UTC().Format(time.RFC3339),
			Verb:      one.Verb,
			Group:     one.Group,
			Version:   one.Version,
			Resource:  one.Resource,
			Kind:      one.Kind,
			Namespace: one.Namespace,
			Name:      one.Name,
			Detail:    strings.Join(one.Cells, " · "),
			Was:       strings.Join(one.Was, " · "),
			Outcome:   api.HistoryDone,
		})
	}
	return out
}
