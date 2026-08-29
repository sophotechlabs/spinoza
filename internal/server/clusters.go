package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
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
	writeJSON(w, s.clusterList())
}

func (s *Server) clusterList() api.ClusterList {
	opened := s.cluster.Opened()
	for i := range opened {
		health := s.healthOfCluster(opened[i].ID)
		opened[i].Reachable = health.Reachable
		opened[i].Reason = health.Reason
	}
	return api.ClusterList{Clusters: opened}
}

func (s *Server) openCluster(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	name := query.Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	_, err := s.cluster.Open(api.ContextRef{Kubeconfig: query.Get("kubeconfig"), Name: name})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.announceContext()
	writeJSON(w, s.clusterList())
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
	writeJSON(w, s.clusterList())
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
	s.announceContext()
	writeJSON(w, s.clusterList())
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
