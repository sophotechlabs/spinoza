package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync"

	"github.com/coder/websocket"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const maxDocBytes = 4 << 20

type Cluster interface {
	Manager() Backend
	Contexts() api.ContextList
	Use(name string) error
}

type Server struct {
	cluster   Cluster
	assets    fs.FS
	files     http.Handler
	token     string
	mu        sync.Mutex
	sessions  map[*wsSession]struct{}
	terminals map[*websocket.Conn]struct{}
}

func New(cluster Cluster, assets fs.FS, token string) *Server {
	return &Server{
		cluster:   cluster,
		assets:    assets,
		files:     http.FileServerFS(assets),
		token:     token,
		sessions:  map[*wsSession]struct{}{},
		terminals: map[*websocket.Conn]struct{}{},
	}
}

func (s *Server) manager() Backend {
	return s.cluster.Manager()
}

type endpoint struct {
	method  string
	path    string
	handler http.HandlerFunc
	offline bool
}

func (s *Server) routes() []endpoint {
	return []endpoint{
		{http.MethodGet, "/healthz", s.handleHealth, true},
		{http.MethodGet, "/api/version", handleVersion, true},
		{http.MethodGet, "/api/contexts", s.listContexts, true},
		{http.MethodPost, "/api/contexts", s.switchContext, true},
		{http.MethodGet, "/api/resources/counts", s.handleCounts, false},
		{http.MethodGet, "/api/resources", s.listResources, false},
		{http.MethodPost, "/api/resources", s.refreshResources, false},
		{http.MethodGet, "/api/gitops/graph", s.handleGraph, false},
		{http.MethodGet, "/api/flux", s.handleFlux, false},
		{http.MethodPost, "/api/flux/action", withRef(s.fluxAction), false},
		{http.MethodPost, "/api/action", s.handleAction, false},
		{http.MethodGet, "/api/metrics/history", s.handleMetricHistory, false},
		{http.MethodGet, "/api/metrics", s.handleMetrics, false},
		{http.MethodGet, "/api/object", withRef(s.getObject), false},
		{http.MethodPut, "/api/object", withRef(s.applyObject), false},
		{http.MethodDelete, "/api/object", withRef(s.deleteObject), false},
		{http.MethodGet, "/api/events", s.handleEvents, false},
		{http.MethodGet, "/api/schema", s.handleSchema, false},
		{http.MethodGet, "/api/portforward", s.listForwards, false},
		{http.MethodPost, "/api/portforward", s.startForward, false},
		{http.MethodDelete, "/api/portforward", s.stopForward, false},
		{http.MethodGet, "/api/exec/support", s.handleExecSupport, false},
		{http.MethodGet, "/api/debug/support", s.handleDebugSupport, false},
		{http.MethodPost, "/api/debug", s.handleDebug, false},
		{http.MethodGet, "/api/exec", s.handleExec, false},
		{http.MethodGet, "/ws", s.handleWS, false},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	known := map[string]bool{}
	for _, entry := range s.routes() {
		mux.HandleFunc(entry.method+" "+entry.path, s.reachable(entry))
		known[entry.path] = true
	}
	for path := range known {
		mux.HandleFunc(path, methodNotAllowed)
	}
	mountProfiler(mux)
	mux.HandleFunc("/", s.handleAssets)
	return s.guard(mux.ServeHTTP)
}

func mountProfiler(mux *http.ServeMux) {
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}

func (s *Server) reachable(entry endpoint) http.HandlerFunc {
	if entry.offline {
		return entry.handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if s.manager() == nil {
			writeError(w, http.StatusServiceUnavailable, "spinoza has no cluster; pick a context that answers")
			return
		}
		entry.handler(w, r)
	}
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func withRef(handler func(http.ResponseWriter, *http.Request, api.ObjectRef)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := refFrom(r)
		if ref.Version == "" || ref.Resource == "" || ref.Name == "" {
			writeError(w, http.StatusBadRequest, "version, resource and name are required")
			return
		}
		handler(w, r, ref)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Health{
		Status:  "ok",
		Version: version.String(),
		Context: s.cluster.Contexts().Current,
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Build{Version: version.String()})
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.serveIndex(w)
		return
	}
	s.files.ServeHTTP(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	doc, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "index.html is missing from the bundled assets")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(InjectHead(doc, TokenScript(s.token)))
}

func TokenScript(token string) string {
	return "<script>window.__SPINOZA_TOKEN__=" + strconv.Quote(token) + ";</script>"
}

func InjectHead(doc []byte, markup string) []byte {
	closing := []byte("</head>")
	return bytes.Replace(doc, closing, append([]byte(markup), closing...), 1)
}

func writeJSON(w http.ResponseWriter, payload any) {
	writeJSONStatus(w, http.StatusOK, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(payload)
	if err != nil {
		slog.Warn("a response could not be encoded", "error", err)
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(map[string]string{"message": message})
	if err != nil {
		slog.Warn("an error response could not be encoded", "error", err)
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

func oversized(err error) bool {
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, api.ErrInternal):
		return http.StatusInternalServerError
	case oversized(err):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, inspect.ErrInvalidUID):
		return http.StatusBadRequest
	case errors.Is(err, jsonschema.ErrNoSchema):
		return http.StatusNotFound
	case cannotReachCluster(err):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable
	case apierrors.IsNotFound(err):
		return http.StatusNotFound
	case apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized
	case apierrors.IsForbidden(err):
		return http.StatusForbidden
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusUnprocessableEntity
	case unreachable(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func (s *Server) listContexts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.cluster.Contexts())
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

func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Resources())
}

func (s *Server) refreshResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().RefreshResources())
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

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Counts(r.Context()))
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

func (s *Server) fluxAction(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	result, err := s.manager().FluxAction(r.Context(), ref, flux.Action(r.URL.Query().Get("action")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
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
	apiVersion := query.Get("version")
	if kind == "" || apiVersion == "" {
		writeError(w, http.StatusBadRequest, "version and kind are required")
		return
	}
	doc, err := s.manager().Schema(r.Context(), jsonschema.GVK{Group: query.Get("group"), Version: apiVersion, Kind: kind})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
}

func (s *Server) listForwards(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Forwards())
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
		writeAPIError(w, readErr)
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
