package server

import (
	"net/http"
)

func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Resources())
}

func (s *Server) refreshResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().RefreshResources())
}

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Namespaces(r.Context()))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Search(r.Context(), r.URL.Query().Get("q")))
}

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Counts(r.Context()))
}
