package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

func writeCheckJSON(w http.ResponseWriter, report api.CheckReport) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-checks.json"`)
	writeJSON(w, report)
}

func writeCheckSARIF(w http.ResponseWriter, report api.CheckReport) {
	w.Header().Set("Content-Type", "application/sarif+json")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-checks.sarif"`)
	err := json.NewEncoder(w).Encode(checks.SARIF(report))
	if err != nil {
		slog.Warn("a sarif export could not be encoded", "error", err)
	}
}
