package server

import (
	"log/slog"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/telemetry"
)

func (s *Server) handleSelfMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", telemetry.ContentType())
	err := telemetry.Default().Registry.Write(w)
	if err != nil {
		slog.Warn("the metrics page could not be written", "error", err)
	}
}
