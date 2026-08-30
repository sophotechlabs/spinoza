package server

import (
	"io"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

const maxRulesBytes = 1 << 20

// checkRules reads a rule list the way an audit would and answers with what it
// would refuse, so the editor can say which rule is wrong while it is being
// written rather than after the next audit has quietly dropped it.
func (s *Server) checkRules(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRulesBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "the rules could not be read")
		return
	}
	writeJSON(w, api.RuleFaults{Faults: checks.Faults(string(body))})
}
