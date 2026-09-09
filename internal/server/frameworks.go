package server

import (
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/checks"
)

func (s *Server) handleFrameworks(w http.ResponseWriter, r *http.Request) {
	report := s.managerFor(r).CheckExport(r.Context(), s.checkFilter(r))
	writeJSON(w, checks.Posture(report, r.URL.Query().Get("framework")))
}
