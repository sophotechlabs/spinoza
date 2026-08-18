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
	"strings"
	"sync"

	"github.com/coder/websocket"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const (
	maxDocBytes = 4 << 20
	queryTrue   = "true"
)

type Cluster interface {
	Manager() Backend
	Contexts() api.ContextList
	Use(ref api.ContextRef) error
	AddKubeconfig(path string) error
	RemoveKubeconfig(path string) error
	Protect(protected bool) error
	Protected() bool
}

type FilePicker func(ctx context.Context) (string, error)

const noFilePicker = "only the desktop window can open a file dialog; type the path instead"

type Server struct {
	cluster    Cluster
	assets     fs.FS
	files      http.Handler
	token      string
	mu         sync.Mutex
	picker     FilePicker
	localShell LocalShellOpener
	settings   Settings
	window     Window
	browser    BrowserOpener
	views      views
	sessions   map[*wsSession]struct{}
	terminals  map[*websocket.Conn]struct{}
}

func New(cluster Cluster, assets fs.FS, token string) *Server {
	return &Server{
		cluster:   cluster,
		assets:    assets,
		files:     http.FileServerFS(assets),
		token:     token,
		settings:  settings.Memory(),
		sessions:  map[*wsSession]struct{}{},
		terminals: map[*websocket.Conn]struct{}{},
		views:     views{grace: defaultIdleGrace, await: defaultBrowserAwait},
	}
}

func (s *Server) manager() Backend {
	return s.cluster.Manager()
}

func (s *Server) UseFilePicker(picker FilePicker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.picker = picker
}

func (s *Server) filePicker() FilePicker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.picker
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
		{http.MethodPost, "/api/protection", s.setProtection, true},
		{http.MethodPost, "/api/kubeconfigs", s.addKubeconfig, true},
		{http.MethodDelete, "/api/kubeconfigs", s.removeKubeconfig, true},
		{http.MethodGet, "/api/kubeconfigs/picker", s.filePickerSupport, true},
		{http.MethodPost, "/api/kubeconfigs/picker", s.pickFile, true},
		{http.MethodGet, "/api/resources/counts", s.handleCounts, false},
		{http.MethodGet, "/api/search", s.handleSearch, false},
		{http.MethodGet, "/api/namespaces", s.handleNamespaces, false},
		{http.MethodGet, "/api/resources", s.listResources, false},
		{http.MethodPost, "/api/resources", s.refreshResources, false},
		{http.MethodGet, "/api/overview", s.handleOverview, false},
		{http.MethodGet, "/api/helm/support", s.handleHelmSupport, true},
		{http.MethodGet, "/api/helm/release", s.handleHelmRelease, false},
		{http.MethodGet, "/api/helm/versions", s.handleHelmVersions, false},
		{http.MethodPost, "/api/helm/action", s.handleHelmAction, false},
		{http.MethodPost, "/api/helm/upgrade", s.handleHelmUpgrade, false},
		{http.MethodGet, "/api/helm", s.handleHelm, false},
		{http.MethodGet, "/api/gitops/graph", s.handleGraph, false},
		{http.MethodGet, "/api/flux", s.handleFlux, false},
		{http.MethodGet, "/api/flux/overview", s.handleFluxOverview, false},
		{http.MethodGet, "/api/argocd", s.handleArgo, false},
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
		{http.MethodGet, "/api/view", s.readView, true},
		{http.MethodPost, "/api/view/browser", s.toBrowser, true},
		{http.MethodPost, "/api/view/desktop", s.toDesktop, true},
		{http.MethodGet, "/api/settings", s.readSettings, true},
		{http.MethodPut, "/api/settings", s.writeSettings, true},
		{http.MethodGet, "/api/shell/support", s.handleLocalShellSupport, true},
		{http.MethodGet, "/api/shell", s.handleLocalShell, true},
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
		Context: s.cluster.Contexts().Current.Name,
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
	_, _ = w.Write(InjectHead(doc, s.IndexHead(ViewBrowser)))
}

func (s *Server) IndexHead(view string) string {
	return TokenScript(s.token) + SettingsScript(s.stored().All()) + ViewScript(view)
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
	case errors.Is(err, helm.ErrNoRelease):
		return http.StatusNotFound
	case errors.Is(err, helm.ErrFluxManaged):
		return http.StatusConflict
	case cannotReachCluster(err):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable
	default:
		return kubeStatusFor(err)
	}
}

func kubeStatusFor(err error) int {
	switch {
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
	s.dropSessions()
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) setProtection(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.Query().Get("protected")
	if wanted != queryTrue && wanted != "false" {
		writeError(w, http.StatusBadRequest, "protected must be true or false")
		return
	}
	err := s.cluster.Protect(wanted == queryTrue)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, s.cluster.Contexts())
}

func (s *Server) unconfirmed(r *http.Request, name string) bool {
	if !s.cluster.Protected() {
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

func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Resources())
}

func (s *Server) refreshResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().RefreshResources())
}

func (s *Server) handleHelmSupport(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.manager().HelmSupport())
}

func (s *Server) handleHelmRelease(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	name := query.Get("name")
	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}
	detail, err := s.manager().HelmRelease(r.Context(), namespace, name)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) handleHelmAction(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	name := query.Get("name")
	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}
	action := query.Get("action")
	if s.unconfirmed(r, name) {
		refuseUnconfirmed(w, name)
		return
	}
	if action == helm.ActionUninstal {
		removed, removeErr := s.manager().HelmUninstall(r.Context(), namespace, name)
		s.finishHelmAction(w, removed, removeErr)
		return
	}
	if action != helm.ActionRollback {
		writeError(w, http.StatusBadRequest, "action must be rollback or uninstall")
		return
	}
	revision, err := strconv.ParseInt(query.Get("revision"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "revision must be a number")
		return
	}
	rolled, rollErr := s.manager().HelmRollback(r.Context(), namespace, name, revision)
	s.finishHelmAction(w, rolled, rollErr)
}

func (s *Server) handleHelmVersions(w http.ResponseWriter, r *http.Request) {
	chart := r.URL.Query().Get("chart")
	if chart == "" {
		writeError(w, http.StatusBadRequest, "chart is required")
		return
	}
	found, err := s.manager().HelmVersions(r.Context(), chart)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

type helmUpgradeBody struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Chart     string `json:"chart"`
	Repo      string `json:"repo"`
	Version   string `json:"version"`
	Values    string `json:"values"`
}

func (s *Server) handleHelmUpgrade(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var dto helmUpgradeBody
	unmarshalErr := json.Unmarshal(body, &dto)
	if unmarshalErr != nil {
		writeError(w, http.StatusBadRequest, "the upgrade request must be json: "+unmarshalErr.Error())
		return
	}
	if dto.Namespace == "" || dto.Name == "" || dto.Chart == "" || dto.Repo == "" || dto.Version == "" {
		writeError(w, http.StatusBadRequest, "namespace, name, chart, repo and version are required")
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == queryTrue
	if !dryRun && s.unconfirmed(r, dto.Name) {
		refuseUnconfirmed(w, dto.Name)
		return
	}
	req := helm.UpgradeRequest{
		Namespace: dto.Namespace,
		Name:      dto.Name,
		Chart:     dto.Chart,
		Version:   dto.Version,
		RepoURL:   dto.Repo,
		OCI:       strings.HasPrefix(dto.Repo, "oci://"),
		Values:    dto.Values,
		DryRun:    dryRun,
	}
	result, upgradeErr := s.manager().HelmUpgrade(r.Context(), req)
	s.finishHelmAction(w, result, upgradeErr)
}

func (s *Server) finishHelmAction(w http.ResponseWriter, result api.HelmActionResult, err error) {
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
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

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Namespaces(r.Context()))
}

func (s *Server) handleFluxOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().FluxOverview(r.Context()))
}

func (s *Server) handleArgo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Argo(r.Context()))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Search(r.Context(), r.URL.Query().Get("q")))
}

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Counts(r.Context()))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Overview(r.Context()))
}

func (s *Server) handleHelm(w http.ResponseWriter, r *http.Request) {
	releases, err := s.manager().HelmReleases(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, releases)
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
	if guarded(req) && s.unconfirmed(r, req.Ref.Name) {
		refuseUnconfirmed(w, req.Ref.Name)
		return
	}
	result, actionErr := s.manager().Action(r.Context(), req)
	if actionErr != nil {
		writeAPIError(w, actionErr)
		return
	}
	writeJSON(w, result)
}

func guarded(req actions.Request) bool {
	if req.DryRun {
		return false
	}
	if req.Action == actions.Drain {
		return true
	}
	return req.Action == actions.Scale && req.Replicas == 0
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
	if s.unconfirmed(r, ref.Name) {
		refuseUnconfirmed(w, ref.Name)
		return
	}
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
