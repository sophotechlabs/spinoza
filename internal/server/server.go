package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const maxDocBytes = 4 << 20

type Cluster interface {
	Manager() *resources.Manager
	Contexts() api.ContextList
	Use(name string) error
}

type Server struct {
	cluster  Cluster
	assets   fs.FS
	mu       sync.Mutex
	sessions map[*wsSession]struct{}
}

func New(cluster Cluster, assets fs.FS) *Server {
	return &Server{cluster: cluster, assets: assets, sessions: map[*wsSession]struct{}{}}
}

func (s *Server) manager() *resources.Manager {
	return s.cluster.Manager()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", guard(healthz))
	mux.HandleFunc("/api/contexts", guard(s.handleContexts))
	mux.HandleFunc("/api/resources", guard(s.handleResources))
	mux.HandleFunc("/api/gitops/graph", guard(s.handleGraph))
	mux.HandleFunc("/api/flux", guard(s.handleFlux))
	mux.HandleFunc("/api/flux/action", guard(s.handleFluxAction))
	mux.HandleFunc("/api/action", guard(s.handleAction))
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
	writeJSONStatus(w, http.StatusOK, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
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

func cannotReachCluster(err error) bool {
	if errors.Is(err, prom.ErrUnavailable) {
		return true
	}
	return errors.Is(err, resources.ErrNotSynced)
}

func unreachable(err error) bool {
	switch {
	case apierrors.IsServiceUnavailable(err), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return true
	case apierrors.IsInternalError(err), apierrors.IsTooManyRequests(err):
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func writeAPIError(w http.ResponseWriter, err error) {
	writeError(w, statusFor(err), err.Error())
}

func statusFor(err error) int {
	switch {
	case cannotReachCluster(err):
		return http.StatusServiceUnavailable
	case apierrors.IsNotFound(err):
		return http.StatusNotFound
	case apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return http.StatusForbidden
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusUnprocessableEntity
	case unreachable(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func (s *Server) handleContexts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.cluster.Contexts())
	case http.MethodPost:
		s.switchContext(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) switchContext(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	err := s.cluster.Use(name)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	s.dropSessions()
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.manager().Resources())
	case http.MethodPost:
		writeJSON(w, s.manager().RefreshResources())
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Graph(r.Context()))
}

func (s *Server) handleFlux(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Flux(r.Context()))
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
	history, historyErr := s.manager().MetricHistory(r.Context(), namespace, pod, span)
	if historyErr != nil {
		writeAPIError(w, historyErr)
		return
	}
	writeJSON(w, history)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Metrics(r.Context()))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	events, err := s.manager().Events(r.Context(), query.Get("namespace"), query.Get("uid"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, events)
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
	result, err := s.manager().FluxAction(r.Context(), ref, flux.Action(r.URL.Query().Get("action")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	req, err := actionRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, actionErr := s.manager().Action(r.Context(), req)
	if actionErr != nil {
		writeAPIError(w, actionErr)
		return
	}
	writeJSON(w, result)
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
		Force:  query.Get("force") == "true",
		DryRun: query.Get("dryRun") == "true",
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
	version := query.Get("version")
	if kind == "" || version == "" {
		writeError(w, http.StatusBadRequest, "version and kind are required")
		return
	}
	doc, err := s.manager().Schema(jsonschema.GVK{Group: query.Get("group"), Version: version, Kind: kind})
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
		writeJSON(w, s.manager().Forwards())
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
	forward, startErr := s.manager().StartForward(r.Context(), target, int32(port))
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
	err := s.manager().StopForward(id)
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
	detail, err := s.manager().Object(r.Context(), ref)
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
	detail, err := s.manager().ApplyObject(r.Context(), ref, doc)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	err := s.manager().DeleteObject(r.Context(), ref)
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
