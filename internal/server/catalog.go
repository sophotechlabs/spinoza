package server

import (
	"net/http"
)

func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Resources())
}

func (s *Server) refreshResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).RefreshResources())
}

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Namespaces(r.Context()))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Search(r.Context(), r.URL.Query().Get("q")))
}

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Counts(r.Context()))
}
