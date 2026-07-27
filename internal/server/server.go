package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
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
	mux.HandleFunc("/api/resources", cors(s.handleResources))
	mux.HandleFunc("/api/gitops/graph", cors(s.handleGraph))
	mux.HandleFunc("/api/flux", cors(s.handleFlux))
	mux.HandleFunc("/api/flux/action", cors(s.handleFluxAction))
	mux.HandleFunc("/api/metrics", cors(s.handleMetrics))
	mux.HandleFunc("/api/object", cors(s.handleObject))
	mux.HandleFunc("/api/events", cors(s.handleEvents))
	mux.HandleFunc("/api/schema", cors(s.handleSchema))
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServerFS(s.assets))
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
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
	writeJSON(w, s.mgr.Resources())
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mgr.Graph(r.Context()))
}

func (s *Server) handleFlux(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mgr.Flux(r.Context()))
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
	err := s.mgr.FluxAction(r.Context(), ref, flux.Action(r.URL.Query().Get("action")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	version := q.Get("version")
	if kind == "" || version == "" {
		writeError(w, http.StatusBadRequest, "version and kind are required")
		return
	}
	doc, err := s.mgr.Schema(jsonschema.GVK{Group: q.Get("group"), Version: version, Kind: kind})
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
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
	q := r.URL.Query()
	return api.ObjectRef{
		Group:     q.Get("group"),
		Version:   q.Get("version"),
		Resource:  q.Get("resource"),
		Namespace: q.Get("namespace"),
		Name:      q.Get("name"),
	}
}
