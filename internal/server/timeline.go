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
	"github.com/sophotechlabs/spinoza/internal/history"
	"github.com/sophotechlabs/spinoza/internal/resources"
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

// A recording holds the informer's callback away from the disk: the callback
// hands over a change and returns, and one goroutine writes them in batches.
type recording struct {
	into    history.Noter
	prune   func(ctx context.Context)
	queue   chan history.Change
	stop    chan struct{}
	stopped chan struct{}
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

func noteOf(note resources.Note) history.Change {
	return history.Change{
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
	}
}

func (t *recording) run(ctx context.Context) {
	defer close(t.stopped)
	t.prune(ctx)
	for {
		select {
		case <-t.stop:
			return
		case first := <-t.queue:
			t.write(ctx, t.fill(first))
		}
	}
}

func (t *recording) fill(first history.Change) []history.Change {
	batch := make([]history.Change, 0, timelineBatch)
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

func (t *recording) write(ctx context.Context, batch []history.Change) {
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
	t.prune(ctx)
}

func (t *recording) close() {
	t.once.Do(func() { close(t.stop) })
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

// startRecording is what turns a tab's timeline on: the kinds are warmed so the
// timeline holds the same thing whatever the tab is showing.
func (s *Server) startRecording(ctx context.Context, id, name string) {
	kinds, ok := kindsNamed(name)
	if !ok {
		s.stopRecording(id)
		return
	}
	store := s.recorder()
	backend := s.managerOf(id)
	if store == nil || backend == nil {
		return
	}
	held := &recording{
		into:    store.Timeline(id),
		queue:   make(chan history.Change, timelineQueue),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	held.prune = func(pruning context.Context) {
		s.pruneTimeline(pruning)
	}
	was := s.holdRecording(id, held)
	if was != nil {
		was.close()
	}
	kept := context.WithoutCancel(ctx)
	go held.run(kept)
	backend.Record(kept, held, kinds)
}

func (s *Server) pruneTimeline(ctx context.Context) {
	store := s.recorder()
	if store == nil {
		return
	}
	err := store.Prune(ctx, history.Retention{Days: s.keepDays(), Rows: timelineRows}, s.instant())
	if err != nil {
		slog.Warn("the timeline could not be trimmed", "error", err)
	}
}

func (s *Server) stopRecording(id string) {
	was := s.holdRecording(id, nil)
	if was == nil {
		return
	}
	backend := s.managerOf(id)
	if backend != nil {
		backend.StopRecording()
	}
	was.close()
}

// StartRecordings picks up where the last run left off: a tab that was
// recording when spinoza closed is recording again when it opens.
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

func (s *Server) readChanges(w http.ResponseWriter, r *http.Request, limit int) (api.History, bool) {
	_, on := s.lookup(clusterOf(r))
	page, readErr := s.recorder().Changed(r.Context(), history.Query{Cluster: on, Limit: limit})
	if readErr != nil {
		writeAPIError(w, readErr)
		return api.History{}, false
	}
	return api.History{
		Entries: changesOf(page.Rows),
		More:    page.More,
		Dropped: s.droppedOn(on),
	}, true
}

func (s *Server) droppedOn(id string) int {
	held := s.recordingOn(id)
	if held == nil {
		return 0
	}
	return int(held.dropped.Load())
}

func changesOf(held []history.Change) []api.HistoryEntry {
	out := make([]api.HistoryEntry, 0, len(held))
	for _, one := range held {
		out = append(out, api.HistoryEntry{
			ID:        one.ID,
			Source:    api.HistoryChange,
			At:        one.At.UTC().Format(time.RFC3339),
			Verb:      one.Verb,
			Group:     one.Group,
			Version:   one.Version,
			Resource:  one.Resource,
			Kind:      one.Kind,
			Namespace: one.Namespace,
			Name:      one.Name,
			Detail:    strings.Join(one.Cells, " · "),
			Outcome:   api.HistoryDone,
		})
	}
	return out
}
