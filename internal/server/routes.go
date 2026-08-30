package server

import (
	"net/http"
	"net/http/pprof"

	"github.com/sophotechlabs/spinoza/internal/api"
)

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
		{http.MethodGet, "/api/update", s.handleUpdate, true},
		{http.MethodPost, "/api/update", s.handleInstallUpdate, true},
		{http.MethodGet, "/api/contexts", s.listContexts, true},
		{http.MethodPost, "/api/protection", s.setProtection, true},
		{http.MethodPost, "/api/kubeconfigs", s.addKubeconfig, true},
		{http.MethodDelete, "/api/kubeconfigs", s.removeKubeconfig, true},
		{http.MethodGet, "/api/kubeconfigs/picker", s.filePickerSupport, true},
		{http.MethodPost, "/api/kubeconfigs/picker", s.pickFile, true},
		{http.MethodGet, "/api/resources/counts", s.handleCounts, false},
		{http.MethodGet, "/api/resources/fleet", s.fleetInventory, false},
		{http.MethodGet, "/api/images/fleet", s.fleetImages, false},
		{http.MethodGet, "/api/search/fleet", s.fleetSearch, false},
		{http.MethodGet, "/api/gitops/fleet", s.fleetGitops, false},
		{http.MethodGet, "/api/search", s.handleSearch, false},
		{http.MethodGet, "/api/namespaces", s.handleNamespaces, false},
		{http.MethodGet, "/api/resources", s.listResources, false},
		{http.MethodPost, "/api/resources", s.refreshResources, false},
		{http.MethodGet, "/api/overview/fleet", s.fleetOverview, false},
		{http.MethodGet, "/api/overview", s.handleOverview, false},
		{http.MethodGet, "/api/capabilities", s.handleCapabilities, true},
		{http.MethodGet, "/api/issues", s.handleIssues, false},
		{http.MethodGet, "/api/issues/fleet", s.fleetIssues, true},
		{http.MethodGet, "/api/helm/access", s.helmAccess, false},
		{http.MethodGet, "/api/helm/release", s.handleHelmRelease, false},
		{http.MethodGet, "/api/helm/versions", s.handleHelmVersions, false},
		{http.MethodGet, "/api/helm/charts", s.handleHelmCharts, false},
		{http.MethodGet, "/api/helm/values", s.handleHelmChartValues, false},
		{http.MethodPost, "/api/helm/action", s.handleHelmAction, false},
		{http.MethodPost, "/api/helm/upgrade", s.handleHelmUpgrade, false},
		{http.MethodPost, "/api/helm/install", s.handleHelmInstall, false},
		{http.MethodGet, "/api/helm/fleet", s.fleetHelm, false},
		{http.MethodGet, "/api/helm", s.handleHelm, false},
		{http.MethodGet, "/api/checks/findings", s.handleCheckPage, false},
		{http.MethodPost, "/api/checks/baseline", s.takeBaseline, false},
		{http.MethodDelete, "/api/checks/baseline", s.clearBaseline, false},
		{http.MethodGet, "/api/checks/baseline/file", s.saveBaselineFile, false},
		{http.MethodPut, "/api/checks/baseline/file", s.loadBaselineFile, false},
		{http.MethodPost, "/api/checks/rules/faults", s.checkRules, false},
		{http.MethodGet, "/api/checks/export", s.exportChecks, false},
		{http.MethodGet, "/api/checks/mutes", s.readMutes, false},
		{http.MethodPost, "/api/checks/mutes", s.muteFinding, false},
		{http.MethodDelete, "/api/checks/mutes", s.unmuteFinding, false},
		{http.MethodGet, "/api/checks/fleet", s.fleetChecks, false},
		{http.MethodGet, "/api/checks", s.handleChecks, false},
		{http.MethodGet, "/api/gitops/graph", s.handleGraph, false},
		{http.MethodGet, "/api/topology", s.handleTopology, false},
		{http.MethodGet, "/api/gitops/app", withRef(s.gitopsApp), false},
		{http.MethodGet, "/api/gitops/app/graph", withRef(s.gitopsAppGraph), false},
		{http.MethodGet, "/api/flux", s.handleFlux, false},
		{http.MethodGet, "/api/flux/overview", s.handleFluxOverview, false},
		{http.MethodGet, "/api/argocd", s.handleArgo, false},
		{http.MethodPost, "/api/flux/action", withRef(s.fluxAction), false},
		{http.MethodPost, "/api/argocd/action", withRef(s.argoAction), false},
		{http.MethodPost, "/api/action", s.handleAction, false},
		{http.MethodGet, "/api/traffic", s.handleTraffic, false},
		{http.MethodGet, "/api/metrics/history", s.handleMetricHistory, false},
		{http.MethodGet, "/api/metrics", s.handleMetrics, false},
		{http.MethodGet, "/api/compare", withRef(s.compare), false},
		{http.MethodGet, "/api/compare/kind", s.compareKind, false},
		{http.MethodGet, "/api/rbac/who", s.handleRBACWho, false},
		{http.MethodGet, "/api/rbac", s.handleRBAC, false},
		{http.MethodGet, "/api/access", withRef(s.objectAccess), false},
		{http.MethodPost, "/api/access", s.bulkAccess, false},
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
		{http.MethodGet, "/api/nodeshell/support", s.handleNodeShellSupport, false},
		{http.MethodGet, "/api/nodeshell", s.handleNodeShell, false},
		{http.MethodGet, "/api/clusters", s.listClusters, true},
		{http.MethodPost, "/api/clusters", s.openCluster, true},
		{http.MethodDelete, "/api/clusters", s.closeCluster, true},
		{http.MethodPost, "/api/clusters/active", s.activateCluster, true},
		{http.MethodPost, "/api/clusters/color", s.recolorCluster, true},
		{http.MethodPost, "/api/clusters/name", s.renameCluster, true},
		{http.MethodPost, "/api/clusters/reopen", s.reopenCluster, true},
		{http.MethodPost, "/api/clusters/timeline", s.recordCluster, true},
		{http.MethodGet, "/api/history", s.readHistory, true},
		{http.MethodDelete, "/api/history", s.clearHistory, true},
		{http.MethodGet, "/api/view", s.readView, true},
		{http.MethodPost, "/api/view/browser", s.toBrowser, true},
		{http.MethodPost, "/api/view/desktop", s.toDesktop, true},
		{http.MethodGet, "/api/memory", handleMemory, true},
		{http.MethodGet, "/api/settings", s.readSettings, true},
		{http.MethodPut, "/api/settings", s.writeSettings, true},
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
	if s.wantsProfiler() {
		mountProfiler(mux)
	}
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
		if s.managerFor(r) == nil {
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
