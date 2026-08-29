package server

import (
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func (s *Server) listContexts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) switchContext(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	name := query.Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	err := s.cluster.Use(api.ContextRef{Kubeconfig: query.Get("kubeconfig"), Name: name})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.forgetHealth()
	s.announceContext()
	s.dropSessions()
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) setProtection(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.Query().Get("protected")
	if wanted != queryTrue && wanted != "false" {
		writeError(w, http.StatusBadRequest, "protected must be true or false")
		return
	}
	_, on := s.lookup(clusterOf(r))
	err := s.cluster.Protect(on, wanted == queryTrue)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) unconfirmed(r *http.Request, name string) bool {
	_, on := s.lookup(clusterOf(r))
	if !s.cluster.Protected(on) {
		return false
	}
	return r.URL.Query().Get("confirm") != name
}

func refuseUnconfirmed(w http.ResponseWriter, name string) {
	writeError(w, http.StatusPreconditionFailed,
		"this cluster is protected; type "+strconv.Quote(name)+" to confirm")
}

func (s *Server) addKubeconfig(w http.ResponseWriter, r *http.Request) {
	s.changeKubeconfigs(w, r, s.cluster.AddKubeconfig)
}

func (s *Server) removeKubeconfig(w http.ResponseWriter, r *http.Request) {
	s.changeKubeconfigs(w, r, s.cluster.RemoveKubeconfig)
}

func (s *Server) changeKubeconfigs(w http.ResponseWriter, r *http.Request, change func(string) error) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	err := change(path)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) filePickerSupport(w http.ResponseWriter, r *http.Request) {
	if s.filePicker() == nil {
		writeJSON(w, api.FilePicker{Reason: noFilePicker})
		return
	}
	writeJSON(w, api.FilePicker{Available: true})
}

func (s *Server) pickFile(w http.ResponseWriter, r *http.Request) {
	picker := s.filePicker()
	if picker == nil {
		writeError(w, http.StatusNotImplemented, noFilePicker)
		return
	}
	path, err := picker(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, api.PickedFile{Path: path})
}
