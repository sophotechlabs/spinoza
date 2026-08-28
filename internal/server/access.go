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
	maxAccessRefs  = 500
	accessTimeout  = 10 * time.Second
)

func (s *Server) objectAccess(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	writeJSON(w, s.manager().Access(r.Context(), ref))
}

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

// No release name means the question is about installing one.
func (s *Server) helmAccess(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	if namespace == "" {
		writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}
	bounded, cancel := context.WithTimeout(r.Context(), accessTimeout)
	defer cancel()
	writeJSON(w, s.manager().HelmAccess(bounded, namespace, query.Get("name")))
}

// An unnamed object would be asked about by kind, and a kind refusal is not a
// row refusal.
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
