package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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

type History interface {
	For(cluster string) history.Recorder
	Recent(ctx context.Context, query history.Query) (history.Page, error)
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

func (s *Server) readHistory(w http.ResponseWriter, r *http.Request) {
	store := s.recorder()
	if store == nil {
		writeJSON(w, api.History{Entries: []api.HistoryEntry{}, Reason: api.HistoryOff})
		return
	}
	limit, err := historyLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, on := s.lookup(clusterOf(r))
	page, readErr := store.Recent(r.Context(), history.Query{Cluster: on, Limit: limit})
	if readErr != nil {
		writeAPIError(w, readErr)
		return
	}
	writeJSON(w, api.History{
		Entries: entriesOf(page.Entries),
		More:    page.More,
		Reason:  store.Reason(),
	})
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
