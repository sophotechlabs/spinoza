package server

import (
	"io"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

const maxRulesBytes = 1 << 20

func (s *Server) checkRules(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRulesBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "the rules could not be read")
		return
	}
	writeJSON(w, api.RuleFaults{Faults: checks.Faults(string(body))})
}
