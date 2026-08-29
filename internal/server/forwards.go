package server

import (
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/portforward"
)

func (s *Server) listForwards(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Forwards())
}

func (s *Server) startForward(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := portforward.Target{
		Kind:      query.Get("kind"),
		Namespace: query.Get("namespace"),
		Name:      query.Get("name"),
	}
	if target.Kind == "" || target.Namespace == "" || target.Name == "" {
		writeError(w, http.StatusBadRequest, "kind, namespace and name are required")
		return
	}
	port, err := strconv.ParseInt(query.Get("port"), 10, 32)
	if err != nil || port <= 0 {
		writeError(w, http.StatusBadRequest, "a positive port is required")
		return
	}
	forward, startErr := s.managerFor(r).StartForward(r.Context(), target, int32(port))
	if startErr != nil {
		writeAPIError(w, startErr)
		return
	}
	writeJSONStatus(w, http.StatusCreated, forward)
}

func (s *Server) stopForward(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	err := s.managerFor(r).StopForward(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
