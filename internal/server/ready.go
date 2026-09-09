package server

import (
	"net/http"
	"slices"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	readyNoCatalog   = "the resource catalog is still being read"
	readyNoInformers = "the cluster caches are still filling"
	readyNoStore     = "the history store is not open yet"
)

func (s *Server) Ready() api.Ready {
	state := s.readyNow()
	if !s.draining() {
		return state
	}
	state.Ready = false
	state.Waiting = append([]string{drainReason}, state.Waiting...)
	return state
}

func (s *Server) readyNow() api.Ready {
	if s.answeredReady() {
		return api.Ready{Ready: true, Discovery: true, Informers: true, Store: true}
	}
	state := s.readyState()
	if state.Ready {
		s.latchReady()
	}
	return state
}

func (s *Server) answeredReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wasReady
}

func (s *Server) latchReady() {
	s.mu.Lock()
	s.wasReady = true
	s.mu.Unlock()
}

func (s *Server) readyState() api.Ready {
	if !s.inCluster() {
		return api.Ready{Ready: true, Discovery: true, Informers: true, Store: true}
	}
	state := api.Ready{
		Discovery: s.readyCatalog(),
		Informers: s.readyCaches(),
		Store:     s.readyStore(),
	}
	state.Ready = state.Discovery && state.Informers && state.Store
	if !state.Discovery {
		state.Waiting = append(state.Waiting, readyNoCatalog)
	}
	if !state.Informers {
		state.Waiting = append(state.Waiting, readyNoInformers)
	}
	if !state.Store {
		state.Waiting = append(state.Waiting, readyNoStore)
	}
	return state
}

func (s *Server) readyCatalog() bool {
	backend, _ := s.lookup("")
	if backend == nil {
		return false
	}
	return len(backend.Resources().Categories) > 0
}

func (s *Server) readyCaches() bool {
	backend, _ := s.lookup("")
	if backend == nil {
		return false
	}
	if backend.Syncing() {
		return false
	}
	return !slices.ContainsFunc(s.openSessions(), readySyncing)
}

func readySyncing(sess *wsSession) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	for _, held := range sess.tables {
		if held.resource == nil {
			return true
		}
	}
	for _, held := range sess.logs {
		if held.resource == nil {
			return true
		}
	}
	return false
}

func (s *Server) readyStore() bool {
	past := s.recorder()
	if past == nil {
		return false
	}
	return past.Reason() == ""
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	state := s.Ready()
	if !state.Ready {
		writeJSONStatus(w, http.StatusServiceUnavailable, state)
		return
	}
	writeJSON(w, state)
}
