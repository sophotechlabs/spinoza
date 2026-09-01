package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
)

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	req, err := actionRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	need := actionRole(req.Action)
	if s.inCluster() && !s.holdsRole(r, need) {
		refuseRole(w, r, need)
		return
	}
	confirm := unguarded
	if guarded(req) {
		confirm = req.Ref.Name
	}
	writer, kept, stop, ok := s.writing(w, r, confirm)
	if !ok {
		return
	}
	defer stop()
	//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
	result, actionErr := writer.Action(kept, req)
	recordErr := actionErr
	if recordErr == nil {
		recordErr = incompleteAction(result)
	}
	s.record(r, change{
		verb:   string(req.Action),
		ref:    req.Ref,
		detail: actionDetail(req),
		dryRun: req.DryRun,
		err:    recordErr,
	})
	if actionErr != nil {
		writeAPIError(w, actionErr)
		return
	}
	writeJSON(w, result)
}

func incompleteAction(result api.ActionResult) error {
	for _, pod := range result.Pods {
		if pod.Outcome != api.OutcomeBlocked && pod.Outcome != api.OutcomeFailed {
			continue
		}
		if result.Message != "" {
			return fmt.Errorf("%w: %s", api.ErrInternal, result.Message)
		}
		return fmt.Errorf("%w: the action did not finish", api.ErrInternal)
	}
	return nil
}

func actionDetail(req actions.Request) string {
	if req.Action != actions.Scale {
		return ""
	}
	return "to " + strconv.FormatInt(req.Replicas, 10) + replicaWord(req.Replicas)
}

func replicaWord(replicas int64) string {
	if replicas == 1 {
		return " replica"
	}
	return " replicas"
}

func guarded(req actions.Request) bool {
	if req.DryRun {
		return false
	}
	switch req.Action {
	case actions.Scale, actions.Restart, actions.Cordon, actions.Uncordon, actions.Drain, actions.Suspend, actions.Resume, actions.Trigger:
		return true
	default:
		return false
	}
}

func actionRequest(r *http.Request) (actions.Request, error) {
	ref := refFrom(r)
	if ref.Version == "" || ref.Resource == "" || ref.Name == "" {
		return actions.Request{}, errors.New("version, resource and name are required")
	}
	query := r.URL.Query()
	req := actions.Request{
		Ref:    ref,
		Action: actions.Action(query.Get("action")),
		Force:  query.Get("force") == queryTrue,
		DryRun: query.Get("dryRun") == queryTrue,
	}
	replicas := query.Get("replicas")
	if req.Action != actions.Scale {
		return req, nil
	}
	if replicas == "" {
		return actions.Request{}, errors.New("replicas is required to scale")
	}
	count, err := strconv.ParseInt(replicas, 10, 32)
	if err != nil {
		return actions.Request{}, fmt.Errorf("replicas must be a number: %w", err)
	}
	req.Replicas = count
	return req, nil
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind := query.Get("kind")
	apiVersion := query.Get("version")
	if kind == "" || apiVersion == "" {
		writeError(w, http.StatusBadRequest, "version and kind are required")
		return
	}
	doc, err := s.managerFor(r).Schema(r.Context(), jsonschema.GVK{Group: query.Get("group"), Version: apiVersion, Kind: kind})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	detail, err := s.managerFor(r).Object(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) applyObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	writer, kept, stop, ok := s.writing(w, r, ref.Name)
	if !ok {
		return
	}
	defer stop()
	doc, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if readErr != nil {
		writeAPIError(w, readErr)
		return
	}
	detail, err := writer.ApplyObject(kept, ref, doc)
	s.record(r, change{verb: verbApply, ref: ref, kind: detail.Kind, err: err})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	writer, kept, stop, ok := s.writing(w, r, ref.Name)
	if !ok {
		return
	}
	defer stop()
	err := writer.DeleteObject(kept, ref)
	s.record(r, change{verb: verbDelete, ref: ref, err: err})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func refFrom(r *http.Request) api.ObjectRef {
	query := r.URL.Query()
	return api.ObjectRef{
		Group:     query.Get("group"),
		Version:   query.Get("version"),
		Resource:  query.Get("resource"),
		Namespace: query.Get("namespace"),
		Name:      query.Get("name"),
	}
}
