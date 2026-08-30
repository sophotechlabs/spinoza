package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/history"
)

const recordTimeout = 10 * time.Second

const (
	verbApply     = "apply"
	verbDelete    = "delete"
	verbExec      = "exec"
	verbDebug     = "debug"
	verbNodeShell = "node shell"
	verbInstall   = "install"
	verbUpgrade   = "upgrade"
	verbRollback  = "rollback"
	verbUninstall = "uninstall"
)

const (
	kindRelease = "Release"
	kindPod     = "Pod"
	kindNode    = "Node"
)

func podRef(namespace, pod string) api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "pods", Namespace: namespace, Name: pod}
}

func nodeRef(node string) api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "nodes", Name: node}
}

var errBadLimit = errors.New("limit must be a number that is not negative")

var errBadSource = errors.New("source must be all, action or change")

type History interface {
	For(cluster string) history.Recorder
	Timeline(cluster string) history.Noter
	Recent(ctx context.Context, query history.Query) (history.Page, error)
	Changed(ctx context.Context, query history.Query) (history.Changes, error)
	Prune(ctx context.Context, keep history.Retention, now time.Time) error
	Forget(ctx context.Context, cluster string) error
	Reason() string
}

func (s *Server) UseHistory(store History) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.past = store
}

type Tabs interface {
	All(ctx context.Context) ([]history.Tab, error)
	Remember(ctx context.Context, tab history.Tab) error
	Recolor(ctx context.Context, id string, color int) error
	Rename(ctx context.Context, id, label, grouping string) error
	Reopening(ctx context.Context, id string, reopen bool) error
	Recording(ctx context.Context, id, kinds string) error
	Forget(ctx context.Context, id string) error
}

func (s *Server) UseTabs(held Tabs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = held
}

func (s *Server) tabs() Tabs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

func (s *Server) recorder() History {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.past
}

func (s *Server) instant() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now()
}

type change struct {
	verb   string
	ref    api.ObjectRef
	kind   string
	detail string
	dryRun bool
	err    error
}

func (s *Server) record(r *http.Request, made change) {
	if made.dryRun {
		return
	}
	store := s.recorder()
	if store == nil {
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), recordTimeout)
	defer cancel()
	_, on := s.lookup(clusterOf(r))
	err := store.For(on).Record(kept, history.Entry{
		At:        s.instant(),
		Verb:      made.verb,
		Group:     made.ref.Group,
		Version:   made.ref.Version,
		Resource:  made.ref.Resource,
		Kind:      made.kind,
		Namespace: made.ref.Namespace,
		Name:      made.ref.Name,
		Detail:    made.detail,
		Outcome:   outcomeOf(made.err),
		Message:   messageOf(made.err),
	})
	if err != nil {
		slog.Warn("what spinoza just did was not recorded", "verb", made.verb, "name", made.ref.Name, "error", err)
	}
}

func outcomeOf(err error) string {
	if err == nil {
		return api.HistoryDone
	}
	if statusFor(err) < http.StatusInternalServerError {
		return api.HistoryRefused
	}
	return api.HistoryFailed
}

func messageOf(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// History is one view of two tables: what spinoza did, and what the cluster
// did. Which of them a reader wants is a filter, not a second view.
func (s *Server) readHistory(w http.ResponseWriter, r *http.Request) {
	store := s.recorder()
	if store == nil {
		writeJSON(w, api.History{Entries: []api.HistoryEntry{}, Reason: api.HistoryOff})
		return
	}
	source := r.URL.Query().Get("source")
	if source == "" {
		source = api.HistoryAll
	}
	if source != api.HistoryAll && source != api.HistoryAction && source != api.HistoryChange {
		writeError(w, http.StatusBadRequest, errBadSource.Error())
		return
	}
	limit, err := historyLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	found, ok := s.historyFrom(w, r, source, limit)
	if !ok {
		return
	}
	found.Reason = store.Reason()
	writeJSON(w, found)
}

func (s *Server) historyFrom(
	w http.ResponseWriter, r *http.Request, source string, limit int,
) (api.History, bool) {
	if source == api.HistoryAction {
		return s.readActions(w, r, limit)
	}
	changes, ok := s.readChanges(w, r, limit)
	if !ok || source == api.HistoryChange {
		return changes, ok
	}
	actions, actionsOk := s.readActions(w, r, limit)
	if !actionsOk {
		return api.History{}, false
	}
	return merged(actions, changes, history.Limit(limit)), true
}

func (s *Server) readActions(w http.ResponseWriter, r *http.Request, limit int) (api.History, bool) {
	_, on := s.lookup(clusterOf(r))
	page, err := s.recorder().Recent(r.Context(), history.Query{Cluster: on, Limit: limit})
	if err != nil {
		writeAPIError(w, err)
		return api.History{}, false
	}
	return api.History{Entries: entriesOf(page.Entries), More: page.More}, true
}

// The two halves are ordered the same way one of them is, and the cap holds
// across the merge, so asking for everything cannot return more than asking for
// one side.
func merged(actions, changes api.History, limit int) api.History {
	rows := make([]api.HistoryEntry, 0, len(actions.Entries)+len(changes.Entries))
	rows = append(rows, actions.Entries...)
	rows = append(rows, changes.Entries...)
	slices.SortStableFunc(rows, newestFirst)
	more := actions.More || changes.More
	if len(rows) > limit {
		rows = rows[:limit]
		more = true
	}
	return api.History{Entries: rows, More: more, Dropped: changes.Dropped}
}

func newestFirst(left, right api.HistoryEntry) int {
	if left.At != right.At {
		return strings.Compare(right.At, left.At)
	}
	if left.Source != right.Source {
		return strings.Compare(left.Source, right.Source)
	}
	if left.ID != right.ID {
		return int(right.ID - left.ID)
	}
	return 0
}

func historyLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errBadLimit
	}
	if limit < 0 {
		return 0, errBadLimit
	}
	return limit, nil
}

func entriesOf(held []history.Entry) []api.HistoryEntry {
	out := make([]api.HistoryEntry, 0, len(held))
	for _, one := range held {
		out = append(out, api.HistoryEntry{
			ID:        one.ID,
			Source:    api.HistoryAction,
			At:        one.At.UTC().Format(time.RFC3339),
			Verb:      one.Verb,
			Group:     one.Group,
			Version:   one.Version,
			Resource:  one.Resource,
			Kind:      one.Kind,
			Namespace: one.Namespace,
			Name:      one.Name,
			Detail:    one.Detail,
			Outcome:   one.Outcome,
			Message:   one.Message,
		})
	}
	return out
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	store := s.recorder()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, api.HistoryOff)
		return
	}
	_, on := s.lookup(clusterOf(r))
	if on == "" {
		writeAPIError(w, notOpen(""))
		return
	}
	err := store.Forget(r.Context(), on)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
