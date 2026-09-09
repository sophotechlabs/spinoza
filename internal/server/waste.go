package server

import (
	"net/http"
	"time"

	"github.com/sophotechlabs/spinoza/internal/waste"
)

const wasteWindow = 30 * time.Minute

func (s *Server) handleWaste(w http.ResponseWriter, r *http.Request) {
	backend := s.managerFor(r)
	report, err := waste.Build(r.Context(), backend, waste.Request{
		Namespace: r.URL.Query().Get("namespace"),
		Meters: []waste.Meter{
			waste.OverAWindow(backend, wasteWindow),
			waste.RightNow(backend),
		},
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, report)
}
