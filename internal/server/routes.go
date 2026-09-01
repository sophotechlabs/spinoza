package server

import (
	"net/http"
	"net/http/pprof"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

type endpoint struct {
	method  string
	path    string
	handler http.HandlerFunc
	offline bool
	writes  bool
}

func (s *Server) routes() []endpoint {
	return []endpoint{
		{http.MethodGet, "/healthz", s.handleHealth, true, false},
		{http.MethodGet, "/api/version", handleVersion, true, false},
		{http.MethodGet, "/api/update", s.handleUpdate, true, false},
		{http.MethodPost, "/api/update", s.handleInstallUpdate, true, false},
		{http.MethodGet, "/api/contexts", s.listContexts, true, false},
		{http.MethodPost, "/api/protection", s.setProtection, true, false},
		{http.MethodPost, "/api/kubeconfigs", s.addKubeconfig, true, false},
		{http.MethodDelete, "/api/kubeconfigs", s.removeKubeconfig, true, false},
		{http.MethodGet, "/api/kubeconfigs/picker", s.filePickerSupport, true, false},
		{http.MethodPost, "/api/kubeconfigs/picker", s.pickFile, true, false},
		{http.MethodGet, "/api/resources/counts", s.handleCounts, false, false},
		{http.MethodGet, "/api/resources/fleet", s.fleetInventory, false, false},
		{http.MethodGet, "/api/images/fleet", s.fleetImages, false, false},
		{http.MethodGet, "/api/search/fleet", s.fleetSearch, false, false},
		{http.MethodGet, "/api/gitops/fleet", s.fleetGitops, false, false},
		{http.MethodGet, "/api/search", s.handleSearch, false, false},
		{http.MethodGet, "/api/namespaces", s.handleNamespaces, false, false},
		{http.MethodGet, "/api/resources", s.listResources, false, false},
		{http.MethodPost, "/api/resources", s.refreshResources, false, false},
		{http.MethodGet, "/api/overview/fleet", s.fleetOverview, false, false},
		{http.MethodGet, "/api/overview", s.handleOverview, false, false},
		{http.MethodGet, "/api/capabilities", s.handleCapabilities, true, false},
		{http.MethodGet, "/api/issues", s.handleIssues, false, false},
		{http.MethodGet, "/api/issues/fleet", s.fleetIssues, true, false},
		{http.MethodGet, "/api/helm/access", s.helmAccess, false, false},
		{http.MethodGet, "/api/helm/release", s.handleHelmRelease, false, false},
		{http.MethodGet, "/api/helm/versions", s.handleHelmVersions, false, false},
		{http.MethodGet, "/api/helm/charts", s.handleHelmCharts, false, false},
		{http.MethodGet, "/api/helm/values", s.handleHelmChartValues, false, false},
		{http.MethodPost, "/api/helm/action", s.handleHelmAction, false, true},
		{http.MethodPost, "/api/helm/upgrade", s.handleHelmUpgrade, false, true},
		{http.MethodPost, "/api/helm/install", s.handleHelmInstall, false, true},
		{http.MethodGet, "/api/helm/fleet", s.fleetHelm, false, false},
		{http.MethodGet, "/api/helm", s.handleHelm, false, false},
		{http.MethodGet, "/api/checks/findings", s.handleCheckPage, false, false},
		{http.MethodGet, "/api/checks/findings/fleet", s.fleetCheckPage, false, false},
		{http.MethodPost, "/api/checks/baseline", s.takeBaseline, false, false},
		{http.MethodDelete, "/api/checks/baseline", s.clearBaseline, false, false},
		{http.MethodGet, "/api/checks/baseline/file", s.saveBaselineFile, false, false},
		{http.MethodPut, "/api/checks/baseline/file", s.loadBaselineFile, false, false},
		{http.MethodPost, "/api/checks/rules/faults", s.checkRules, false, false},
		{http.MethodGet, "/api/checks/export", s.exportChecks, false, false},
		{http.MethodGet, "/api/checks/mutes", s.readMutes, false, false},
		{http.MethodPost, "/api/checks/mutes", s.muteFinding, false, false},
		{http.MethodDelete, "/api/checks/mutes", s.unmuteFinding, false, false},
		{http.MethodGet, "/api/checks/fleet", s.fleetChecks, false, false},
		{http.MethodGet, "/api/checks", s.handleChecks, false, false},
		{http.MethodGet, "/api/gitops/graph", s.handleGraph, false, false},
		{http.MethodGet, "/api/topology", s.handleTopology, false, false},
		{http.MethodGet, "/api/gitops/app", withRef(s.gitopsApp), false, false},
		{http.MethodGet, "/api/gitops/app/graph", withRef(s.gitopsAppGraph), false, false},
		{http.MethodGet, "/api/flux", s.handleFlux, false, false},
		{http.MethodGet, "/api/flux/overview", s.handleFluxOverview, false, false},
		{http.MethodGet, "/api/argocd", s.handleArgo, false, false},
		{http.MethodPost, "/api/flux/action", withRef(s.fluxAction), false, true},
		{http.MethodPost, "/api/argocd/action", withRef(s.argoAction), false, true},
		{http.MethodPost, "/api/action", s.handleAction, false, true},
		{http.MethodGet, "/api/traffic", s.handleTraffic, false, false},
		{http.MethodGet, "/api/metrics/history", s.handleMetricHistory, false, false},
		{http.MethodGet, "/api/metrics", s.handleMetrics, false, false},
		{http.MethodGet, "/api/compare", withRef(s.compare), false, false},
		{http.MethodGet, "/api/compare/kind", s.compareKind, false, false},
		{http.MethodGet, "/api/rbac/who", s.handleRBACWho, false, false},
		{http.MethodGet, "/api/rbac", s.handleRBAC, false, false},
		{http.MethodGet, "/api/access", withRef(s.objectAccess), false, false},
		{http.MethodPost, "/api/access", s.bulkAccess, false, false},
		{http.MethodGet, "/api/object", withRef(s.getObject), false, false},
		{http.MethodPut, "/api/object", withRef(s.applyObject), false, true},
		{http.MethodDelete, "/api/object", withRef(s.deleteObject), false, true},
		{http.MethodGet, "/api/events", s.handleEvents, false, false},
		{http.MethodGet, "/api/schema", s.handleSchema, false, false},
		{http.MethodGet, "/api/portforward", s.listForwards, false, false},
		{http.MethodPost, "/api/portforward", s.startForward, false, false},
		{http.MethodDelete, "/api/portforward", s.stopForward, false, false},
		{http.MethodGet, "/api/exec/support", s.handleExecSupport, false, false},
		{http.MethodGet, "/api/debug/support", s.handleDebugSupport, false, false},
		{http.MethodPost, "/api/debug", s.handleDebug, false, true},
		{http.MethodGet, "/api/exec", s.handleExec, false, false},
		{http.MethodGet, "/api/nodeshell/support", s.handleNodeShellSupport, false, false},
		{http.MethodGet, "/api/nodeshell", s.handleNodeShell, false, true},
		{http.MethodGet, "/api/clusters", s.listClusters, true, false},
		{http.MethodPost, "/api/clusters", s.openCluster, true, false},
		{http.MethodDelete, "/api/clusters", s.closeCluster, true, false},
		{http.MethodPost, "/api/clusters/active", s.activateCluster, true, false},
		{http.MethodPost, "/api/clusters/color", s.recolorCluster, true, false},
		{http.MethodPost, "/api/clusters/name", s.renameCluster, true, false},
		{http.MethodPost, "/api/clusters/reopen", s.reopenCluster, true, false},
		{http.MethodPost, "/api/clusters/timeline", s.recordCluster, true, false},
		{http.MethodGet, "/api/history", s.readHistory, true, false},
		{http.MethodDelete, "/api/history", s.clearHistory, true, false},
		{http.MethodGet, "/api/view", s.readView, true, false},
		{http.MethodPost, "/api/view/browser", s.toBrowser, true, false},
		{http.MethodPost, "/api/view/desktop", s.toDesktop, true, false},
		{http.MethodGet, "/api/memory", handleMemory, true, false},
		{http.MethodGet, "/api/settings", s.readSettings, true, false},
		{http.MethodPut, "/api/settings", s.writeSettings, true, false},
		{http.MethodGet, "/api/shell", s.handleLocalShell, true, false},
		{http.MethodGet, "/ws", s.handleWS, false, false},
	}
}

func (s *Server) allRoutes() []endpoint {
	all := append(s.routes(), s.sessionRoute())
	if !s.inCluster() {
		return all
	}
	return append(all, s.signInRoutes()...)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	known := map[string]bool{}
	for _, entry := range s.allRoutes() {
		mux.HandleFunc(entry.method+" "+entry.path, s.permitted(entry))
		known[entry.path] = true
	}
	for path := range known {
		mux.HandleFunc(path, methodNotAllowed)
	}
	if s.wantsProfiler() {
		s.mountProfiler(mux)
	}
	mux.HandleFunc("/", s.handleAssets)
	return s.guard(mux.ServeHTTP)
}

func (s *Server) mountProfiler(mux *http.ServeMux) {
	for path, handler := range map[string]http.HandlerFunc{
		"GET /debug/pprof/":        pprof.Index,
		"GET /debug/pprof/cmdline": pprof.Cmdline,
		"GET /debug/pprof/profile": pprof.Profile,
		"GET /debug/pprof/symbol":  pprof.Symbol,
		"GET /debug/pprof/trace":   pprof.Trace,
	} {
		mux.HandleFunc(path, s.forAdmins(handler))
	}
}

func (s *Server) forAdmins(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.inCluster() && !s.holdsRole(r, auth.RoleAdmin) {
			refuseRole(w, r, auth.RoleAdmin)
			return
		}
		handler(w, r)
	}
}

func (s *Server) permitted(entry endpoint) http.HandlerFunc {
	inner := s.reachable(entry)
	need := roleFor(entry)
	only := onlyHere(entry)
	whole := wholeCluster(entry)
	fleet := wholeFleet(entry)
	if need == "" && !only && !whole {
		return inner
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.inCluster() {
			inner(w, r)
			return
		}
		if only {
			writeError(w, http.StatusForbidden, notInClusterMode)
			return
		}
		if need != "" && !s.holdsRole(r, need) {
			refuseRole(w, r, need)
			return
		}
		if whole {
			status, why := s.wholeClusterRefusal(r)
			if fleet {
				status, why = s.wholeFleetRefusal(r)
			}
			if why != "" {
				writeError(w, status, why)
				return
			}
		}
		inner(w, r)
	}
}

func (s *Server) wholeFleetRefusal(r *http.Request) (int, string) {
	for _, one := range s.cluster.Opened() {
		backend := s.managerOf(one.ID)
		if backend == nil {
			return http.StatusServiceUnavailable, nameOf(one) + " is not available"
		}
		seen := backend.Scope(r.Context())
		if seen.Everywhere {
			continue
		}
		if len(seen.Namespaces) == 0 && len(seen.Undecided) > 0 {
			return http.StatusServiceUnavailable, nameOf(one) + ": " + clusterWouldNotSay
		}
		return http.StatusForbidden, nameOf(one) + ": " + readsEverything
	}
	return 0, ""
}

func (s *Server) wholeClusterRefusal(r *http.Request) (int, string) {
	backend := s.managerFor(r)
	if backend == nil {
		return 0, ""
	}
	seen := backend.Scope(r.Context())
	if seen.Everywhere {
		return 0, ""
	}
	if len(seen.Namespaces) == 0 && len(seen.Undecided) > 0 {
		return http.StatusServiceUnavailable, clusterWouldNotSay
	}
	return http.StatusForbidden, readsEverything
}

func (s *Server) holdsRole(r *http.Request, need string) bool {
	who, ok := auth.IdentityFrom(r.Context())
	if !ok {
		return false
	}
	return auth.Allows(who.Role, need)
}

func refuseRole(w http.ResponseWriter, r *http.Request, need string) {
	writeError(w, http.StatusForbidden, "your role here is "+heldRole(r)+"; this needs "+need)
}

func heldRole(r *http.Request) string {
	who, ok := auth.IdentityFrom(r.Context())
	if !ok || who.Role == "" {
		return "unknown"
	}
	return who.Role
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
