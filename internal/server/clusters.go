package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
)

const defaultTerminalDrain = 20 * time.Second

const drainStep = 20 * time.Millisecond

var errNoClusterNamed = errors.New("cluster is required")

var errBadColor = errors.New("color must be a number between 1 and 8")

var errBadReopen = errors.New("reopen must be true or false")

var errNameTooLong = errors.New("a name may be 60 characters at most")

var errNowhereToKeepIt = errors.New("spinoza has nowhere to keep that")

func notOpen(id string) error {
	if id == "" {
		return fmt.Errorf("%w: spinoza has no cluster; pick a context that answers", api.ErrNotOpen)
	}
	return fmt.Errorf("%w: %s", api.ErrNotOpen, id)
}

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.clusterList(r.Context()))
}

func (s *Server) clusterList(ctx context.Context) api.ClusterList {
	opened := s.cluster.Opened()
	known := s.tabsByID(ctx)
	for i := range opened {
		health := s.healthOfCluster(opened[i].ID)
		opened[i].Reachable = health.Reachable
		opened[i].Wobbling = health.Wobbling
		opened[i].Reason = health.Reason
		opened[i].Color = known[opened[i].ID].Color
		opened[i].Label = known[opened[i].ID].Label
		opened[i].Grouping = known[opened[i].ID].Grouping
		opened[i].Reopen = known[opened[i].ID].Reopen
		opened[i].Timeline = known[opened[i].ID].Timeline
	}
	return api.ClusterList{Clusters: opened, Remembered: s.rememberedTabs(ctx)}
}

func (s *Server) rememberedTabs(ctx context.Context) []api.RememberedCluster {
	held := s.tabs()
	if held == nil {
		return []api.RememberedCluster{}
	}
	found, err := held.All(ctx)
	if err != nil {
		slog.Warn("the clusters that were open could not be read", "error", err)
		return []api.RememberedCluster{}
	}
	known := s.knownContexts()
	out := make([]api.RememberedCluster, 0, len(found))
	for _, one := range found {
		if !one.Reopen {
			continue
		}
		if len(known) > 0 && !known[one.Context] {
			slog.Info("a remembered cluster is not in your kubeconfigs and will not reopen", "context", one.Context)
			continue
		}
		out = append(out, api.RememberedCluster{
			ID:         one.ID,
			Context:    one.Context,
			Kubeconfig: one.Kubeconfig,
		})
	}
	return out
}

func (s *Server) knownContexts() map[string]bool {
	known := map[string]bool{}
	for _, source := range s.cluster.Contexts().Kubeconfigs {
		if source.Error != "" {
			continue
		}
		for _, one := range source.Contexts {
			known[one.Name] = true
		}
	}
	return known
}

const rememberTimeout = 10 * time.Second

func (s *Server) rememberTab(ctx context.Context, id string, ref api.ContextRef) {
	held := s.tabs()
	if held == nil {
		return
	}
	known := s.tabsByID(ctx)
	tab := store.Tab{
		ID:         id,
		Context:    ref.Name,
		Kubeconfig: ref.Kubeconfig,
		Seen:       s.instant(),
		Color:      colorFor(known, id, s.cluster.Opened()),
		Reopen:     true,
	}
	if was, seen := known[id]; seen {
		tab.Label = was.Label
		tab.Grouping = was.Grouping
		tab.Reopen = was.Reopen
		tab.Timeline = was.Timeline
	}
	err := held.Remember(ctx, tab)
	if err != nil {
		slog.Warn("this cluster will not come back next time", "context", ref.Name, "error", err)
	}
}

func (s *Server) RememberOpen(ctx context.Context) {
	known := s.tabsByID(ctx)
	for _, one := range s.cluster.Opened() {
		if _, seen := known[one.ID]; seen {
			continue
		}
		s.rememberTab(ctx, one.ID, api.ContextRef{Kubeconfig: one.Kubeconfig, Name: one.Context})
	}
}

func colorFor(known map[string]store.Tab, id string, open []api.OpenCluster) int {
	if held, seen := known[id]; seen && held.Color != 0 {
		return held.Color
	}
	taken := map[int]bool{}
	for _, one := range open {
		taken[known[one.ID].Color] = true
	}
	for color := 1; color <= api.ClusterColors; color++ {
		if !taken[color] {
			return color
		}
	}
	return 1
}

func (s *Server) tabsByID(ctx context.Context) map[string]store.Tab {
	known := map[string]store.Tab{}
	held := s.tabs()
	if held == nil {
		return known
	}
	found, err := held.All(ctx)
	if err != nil {
		slog.Warn("the clusters that were open could not be read", "error", err)
		return known
	}
	for _, one := range found {
		known[one.ID] = one
	}
	return known
}

const maxLabel = 60

func (s *Server) renameCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	query := r.URL.Query()
	label := strings.TrimSpace(query.Get("label"))
	grouping := strings.TrimSpace(query.Get("grouping"))
	if utf8.RuneCountInString(label) > maxLabel || utf8.RuneCountInString(grouping) > maxLabel {
		writeError(w, http.StatusBadRequest, errNameTooLong.Error())
		return
	}
	held := s.tabs()
	if held == nil {
		writeError(w, http.StatusServiceUnavailable, errNowhereToKeepIt.Error())
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rememberTimeout)
	defer cancel()
	err := held.Rename(kept, id, label, grouping)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, s.clusterList(kept))
}

func (s *Server) reopenCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	wanted := r.URL.Query().Get("reopen")
	if wanted != queryTrue && wanted != "false" {
		writeError(w, http.StatusBadRequest, errBadReopen.Error())
		return
	}
	held := s.tabs()
	if held == nil {
		writeError(w, http.StatusServiceUnavailable, errNowhereToKeepIt.Error())
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rememberTimeout)
	defer cancel()
	err := held.Reopening(kept, id, wanted == queryTrue)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, s.clusterList(kept))
}

func (s *Server) recolorCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	color, err := strconv.Atoi(r.URL.Query().Get("color"))
	if err != nil || color < 1 || color > api.ClusterColors {
		writeError(w, http.StatusBadRequest, errBadColor.Error())
		return
	}
	held := s.tabs()
	if held == nil {
		writeError(w, http.StatusServiceUnavailable, errNowhereToKeepIt.Error())
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rememberTimeout)
	defer cancel()
	setErr := held.Recolor(kept, id, color)
	if setErr != nil {
		writeAPIError(w, setErr)
		return
	}
	writeJSON(w, s.clusterList(kept))
}

func (s *Server) forgetTab(ctx context.Context, id string) {
	held := s.tabs()
	if held == nil {
		return
	}
	err := held.Forget(ctx, id)
	if err != nil {
		slog.Warn("this cluster will come back next time", "cluster", id, "error", err)
	}
}

func (s *Server) openCluster(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	name := query.Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	ref := api.ContextRef{Kubeconfig: query.Get("kubeconfig"), Name: name}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rememberTimeout)
	defer cancel()
	restoring := s.restoringTab(kept, ref)
	was := s.cluster.ID()
	id, err := s.cluster.Open(ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if restoring {
		s.keepActive(was, id)
	}
	s.rememberTab(kept, id, ref)
	s.announceContext()
	writeJSON(w, s.clusterList(kept))
}

func (s *Server) restoringTab(ctx context.Context, ref api.ContextRef) bool {
	for _, one := range s.rememberedTabs(ctx) {
		if one.Context != ref.Name {
			continue
		}
		if one.Kubeconfig != ref.Kubeconfig {
			continue
		}
		return !s.alreadyOpen(one.ID)
	}
	return false
}

func (s *Server) alreadyOpen(id string) bool {
	for _, one := range s.cluster.Opened() {
		if one.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) keepActive(was, opened string) {
	if was == "" {
		return
	}
	if was == opened {
		return
	}
	err := s.cluster.Activate(was)
	if err != nil {
		slog.Warn("the cluster spinoza started on could not be kept active", "cluster", was, "error", err)
	}
}

func (s *Server) activateCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	err := s.cluster.Activate(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.announceContext()
	writeJSON(w, s.clusterList(r.Context()))
}

func (s *Server) closeCluster(w http.ResponseWriter, r *http.Request) {
	id := clusterOf(r)
	if id == "" {
		writeError(w, http.StatusBadRequest, errNoClusterNamed.Error())
		return
	}
	s.stopRecording(id)
	s.drainTerminals(r.Context(), id)
	s.dropSubscriptionsOn(id)
	err := s.cluster.Close(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), rememberTimeout)
	defer cancel()
	s.forgetHealthOf(id)
	s.forgetTab(kept, id)
	s.announceContext()
	writeJSON(w, s.clusterList(kept))
}

func (s *Server) drainTerminals(ctx context.Context, id string) {
	open := s.terminalsOn(id)
	if len(open) == 0 {
		return
	}
	for _, conn := range open {
		_ = conn.CloseNow()
	}
	waiting, stop := context.WithTimeout(context.WithoutCancel(ctx), s.terminalDrain)
	defer stop()
	for len(s.terminalsOn(id)) > 0 {
		select {
		case <-waiting.Done():
			return
		case <-time.After(drainStep):
		}
	}
}
