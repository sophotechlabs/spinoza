package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/history"
)

const terminalDrain = 20 * time.Second

const drainStep = 20 * time.Millisecond

var errNoClusterNamed = errors.New("cluster is required")

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
	for i := range opened {
		health := s.healthOfCluster(opened[i].ID)
		opened[i].Reachable = health.Reachable
		opened[i].Reason = health.Reason
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
	out := make([]api.RememberedCluster, 0, len(found))
	for _, one := range found {
		out = append(out, api.RememberedCluster{
			ID:         one.ID,
			Context:    one.Context,
			Kubeconfig: one.Kubeconfig,
		})
	}
	return out
}

func (s *Server) rememberTab(ctx context.Context, id string, ref api.ContextRef) {
	held := s.tabs()
	if held == nil {
		return
	}
	err := held.Remember(ctx, history.Tab{
		ID:         id,
		Context:    ref.Name,
		Kubeconfig: ref.Kubeconfig,
		Seen:       s.instant(),
	})
	if err != nil {
		slog.Warn("this cluster will not come back next time", "context", ref.Name, "error", err)
	}
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
	id, err := s.cluster.Open(ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.rememberTab(r.Context(), id, ref)
	s.announceContext()
	writeJSON(w, s.clusterList(r.Context()))
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
	s.drainTerminals(r.Context(), id)
	s.dropSubscriptionsOn(id)
	err := s.cluster.Close(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.forgetHealthOf(id)
	s.forgetTab(r.Context(), id)
	s.announceContext()
	writeJSON(w, s.clusterList(r.Context()))
}

func (s *Server) drainTerminals(ctx context.Context, id string) {
	open := s.terminalsOn(id)
	if len(open) == 0 {
		return
	}
	for _, conn := range open {
		_ = conn.Close(websocket.StatusGoingAway, "that cluster was closed")
	}
	waiting, stop := context.WithTimeout(context.WithoutCancel(ctx), terminalDrain)
	defer stop()
	for len(s.terminalsOn(id)) > 0 {
		select {
		case <-waiting.Done():
			return
		case <-time.After(drainStep):
		}
	}
}
