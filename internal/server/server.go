package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const maxDocBytes = 4 << 20

type Server struct {
	mgr    *resources.Manager
	assets fs.FS
}

func New(mgr *resources.Manager, assets fs.FS) *Server {
	return &Server{mgr: mgr, assets: assets}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/api/resources", guard(s.handleResources))
	mux.HandleFunc("/api/gitops/graph", guard(s.handleGraph))
	mux.HandleFunc("/api/flux", guard(s.handleFlux))
	mux.HandleFunc("/api/flux/action", guard(s.handleFluxAction))
	mux.HandleFunc("/api/metrics/history", guard(s.handleMetricHistory))
	mux.HandleFunc("/api/metrics", guard(s.handleMetrics))
	mux.HandleFunc("/api/object", guard(s.handleObject))
	mux.HandleFunc("/api/events", guard(s.handleEvents))
	mux.HandleFunc("/api/schema", guard(s.handleSchema))
	mux.HandleFunc("/api/portforward", guard(s.handleForwards))
	mux.HandleFunc("/api/exec/support", guard(s.handleExecSupport))
	mux.HandleFunc("/api/debug/support", guard(s.handleDebugSupport))
	mux.HandleFunc("/api/debug", guard(s.handleDebug))
	mux.HandleFunc("/api/exec", guard(s.handleExec))
	mux.HandleFunc("/ws", guard(s.handleWS))
	mux.Handle("/", guard(http.FileServerFS(s.assets).ServeHTTP))
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(payload)
	if err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(map[string]string{"message": message})
	if err != nil {
		log.Printf("encode error response: %v", err)
	}
}

func writeAPIError(w http.ResponseWriter, err error) {
	writeError(w, statusFor(err), err.Error())
}

func statusFor(err error) int {
	switch {
	case apierrors.IsNotFound(err):
		return http.StatusNotFound
	case apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return http.StatusForbidden
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.mgr.Resources())
	case http.MethodPost:
		writeJSON(w, s.mgr.RefreshResources())
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mgr.Graph(r.Context()))
}

func (s *Server) handleFlux(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mgr.Flux(r.Context()))
}

func (s *Server) handleMetricHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	pod := query.Get("pod")
	if namespace == "" || pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	span, err := prom.ParseSpan(query.Get("range"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	history, historyErr := s.mgr.MetricHistory(r.Context(), namespace, pod, span)
	if historyErr != nil {
		writeAPIError(w, historyErr)
		return
	}
	writeJSON(w, history)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mgr.Metrics(r.Context()))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeJSON(w, s.mgr.Events(r.Context(), q.Get("namespace"), q.Get("uid")))
}

func (s *Server) handleFluxAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ref := refFrom(r)
	if ref.Version == "" || ref.Resource == "" || ref.Name == "" {
		writeError(w, http.StatusBadRequest, "version, resource and name are required")
		return
	}
	result, err := s.mgr.FluxAction(r.Context(), ref, flux.Action(r.URL.Query().Get("action")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind := query.Get("kind")
	version := query.Get("version")
	if kind == "" || version == "" {
		writeError(w, http.StatusBadRequest, "version and kind are required")
		return
	}
	doc, err := s.mgr.Schema(jsonschema.GVK{Group: query.Get("group"), Version: version, Kind: kind})
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
}

func (s *Server) handleForwards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.mgr.Forwards())
	case http.MethodPost:
		s.startForward(w, r)
	case http.MethodDelete:
		s.stopForward(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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
	forward, startErr := s.mgr.StartForward(r.Context(), target, int32(port))
	if startErr != nil {
		writeAPIError(w, startErr)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, forward)
}

func (s *Server) stopForward(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	err := s.mgr.StopForward(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleObject(w http.ResponseWriter, r *http.Request) {
	ref := refFrom(r)
	if ref.Version == "" || ref.Resource == "" || ref.Name == "" {
		writeError(w, http.StatusBadRequest, "version, resource and name are required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getObject(w, r, ref)
	case http.MethodPut:
		s.applyObject(w, r, ref)
	case http.MethodDelete:
		s.deleteObject(w, r, ref)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	detail, err := s.mgr.Object(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) applyObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	doc, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if readErr != nil {
		writeError(w, http.StatusBadRequest, readErr.Error())
		return
	}
	detail, err := s.mgr.ApplyObject(r.Context(), ref, doc)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	err := s.mgr.DeleteObject(r.Context(), ref)
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
