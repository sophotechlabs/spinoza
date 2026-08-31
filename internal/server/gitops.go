package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

func (s *Server) gitopsApp(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	app, err := s.managerFor(r).GitopsApp(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, app)
}

func (s *Server) gitopsAppGraph(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	graph, err := s.managerFor(r).GitopsAppGraph(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, graph)
}

func argoDetail(req argocd.Request) string {
	if req.Action == argocd.Rollback {
		return "to revision " + strconv.FormatInt(req.Revision, 10)
	}
	if len(req.Resources) > 0 {
		return strconv.Itoa(len(req.Resources)) + " selected resources"
	}
	if req.Prune {
		return "with prune"
	}
	return ""
}

func (s *Server) fluxAction(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	action := flux.Action(r.URL.Query().Get("action"))
	if patchesTheCluster(action) && s.unconfirmed(r, ref.Name) {
		refuseUnconfirmed(w, ref.Name)
		return
	}
	result, err := s.managerFor(r).FluxAction(r.Context(), ref, action)
	s.record(r, change{verb: string(action), ref: ref, err: err})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

type argoResourceBody struct {
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type argoActionBody struct {
	Prune      bool               `json:"prune"`
	DryRun     bool               `json:"dryRun"`
	Force      bool               `json:"force"`
	Replace    bool               `json:"replace"`
	ServerSide bool               `json:"serverSide"`
	ApplyOnly  bool               `json:"applyOnly"`
	Revision   int64              `json:"revision"`
	Resources  []argoResourceBody `json:"resources"`
}

func (s *Server) argoAction(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var dto argoActionBody
	if len(raw) > 0 {
		unmarshalErr := json.Unmarshal(raw, &dto)
		if unmarshalErr != nil {
			writeError(w, http.StatusBadRequest, "the action options must be json: "+unmarshalErr.Error())
			return
		}
	}
	req := argoRequest(r.URL.Query().Get("action"), dto)
	if changesTheCluster(req.Action) && s.unconfirmed(r, ref.Name) {
		refuseUnconfirmed(w, ref.Name)
		return
	}
	result, actionErr := s.managerFor(r).ArgoAction(r.Context(), ref, req)
	s.record(r, change{
		verb:   string(req.Action),
		ref:    ref,
		detail: argoDetail(req),
		dryRun: dto.DryRun,
		err:    actionErr,
	})
	if actionErr != nil {
		writeAPIError(w, actionErr)
		return
	}
	writeJSON(w, result)
}

func patchesTheCluster(action flux.Action) bool {
	switch action {
	case flux.Reconcile, flux.ReconcileSource, flux.Suspend, flux.Resume:
		return true
	default:
		return false
	}
}

func changesTheCluster(action argocd.Action) bool {
	switch action {
	case argocd.Sync, argocd.Rollback, argocd.Suspend, argocd.Resume:
		return true
	default:
		return false
	}
}

func argoRequest(action string, dto argoActionBody) argocd.Request {
	return argocd.Request{
		Action:     argocd.Action(action),
		Prune:      dto.Prune,
		DryRun:     dto.DryRun,
		Force:      dto.Force,
		Replace:    dto.Replace,
		ServerSide: dto.ServerSide,
		ApplyOnly:  dto.ApplyOnly,
		Revision:   dto.Revision,
		Resources:  argoResources(dto.Resources),
	}
}

func argoResources(sent []argoResourceBody) []argocd.Resource {
	if len(sent) == 0 {
		return nil
	}
	out := make([]argocd.Resource, 0, len(sent))
	for _, one := range sent {
		out = append(out, argocd.Resource{
			Group:     one.Group,
			Kind:      one.Kind,
			Name:      one.Name,
			Namespace: one.Namespace,
		})
	}
	return out
}
