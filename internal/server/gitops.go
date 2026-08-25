package server

import (
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

func (s *Server) fluxAction(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	result, err := s.manager().FluxAction(r.Context(), ref, flux.Action(r.URL.Query().Get("action")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) argoAction(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	action := argocd.Action(r.URL.Query().Get("action"))
	if action == argocd.Sync && s.unconfirmed(r, ref.Name) {
		refuseUnconfirmed(w, ref.Name)
		return
	}
	result, err := s.manager().ArgoAction(r.Context(), ref, action)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}
