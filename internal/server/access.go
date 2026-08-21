package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	maxAccessBytes = 1 << 20
	// maxAccessRefs bounds one question: a selection larger than this is asking
	// the apiserver more than it is worth asking on a click.
	maxAccessRefs = 500
	accessTimeout = 10 * time.Second
)

func (s *Server) objectAccess(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	writeJSON(w, s.manager().Access(r.Context(), ref))
}

// bulkAccess answers one capability across a selection of rows, which is what
// the bulk bar needs before it acts on them. A question that cannot be answered
// in time comes back refusing nothing, the same as everywhere else: spinoza does
// not stop a user over an answer it failed to get.
func (s *Server) bulkAccess(w http.ResponseWriter, r *http.Request) {
	var query api.AccessQuery
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAccessBytes)).Decode(&query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "an access query needs a capability and a list of objects")
		return
	}
	refuseErr := readable(query)
	if refuseErr != nil {
		writeError(w, http.StatusBadRequest, refuseErr.Error())
		return
	}
	bounded, cancel := context.WithTimeout(r.Context(), accessTimeout)
	defer cancel()
	writeJSON(w, s.manager().AccessEach(bounded, query.Capability, query.Refs))
}

// readable holds the answers exact. An object without a name would be asked
// about by kind alone, and a refusal for the kind is not a refusal for the row
// that was selected.
func readable(query api.AccessQuery) error {
	if query.Capability == "" {
		return errors.New("capability is required")
	}
	if len(query.Refs) > maxAccessRefs {
		return fmt.Errorf("at most %d objects can be asked about at once", maxAccessRefs)
	}
	for _, ref := range query.Refs {
		if ref.Version == "" || ref.Resource == "" || ref.Name == "" {
			return errors.New("every object needs a version, a resource and a name")
		}
	}
	return nil
}
