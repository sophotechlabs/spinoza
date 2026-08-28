package server

import (
	"context"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type Updates interface {
	Status(ctx context.Context) api.UpdateStatus
}

func (s *Server) UseUpdates(checker Updates) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = checker
}

func (s *Server) updateChecker() Updates {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	checker := s.updateChecker()
	if checker == nil {
		writeJSON(w, api.UpdateStatus{Reason: "this build does not check for releases"})
		return
	}
	writeJSON(w, checker.Status(r.Context()))
}
