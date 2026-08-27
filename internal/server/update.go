package server

import (
	"context"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Updates is asked whether a newer spinoza has been published. The first call
// is the one that asks; the rest read what it found.
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

// handleUpdate answers whether there is a newer release. A run with no checker
// wired up says so rather than failing: not asking is a state, not a fault.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	checker := s.updateChecker()
	if checker == nil {
		writeJSON(w, api.UpdateStatus{Reason: "this build does not check for releases"})
		return
	}
	writeJSON(w, checker.Status(r.Context()))
}
