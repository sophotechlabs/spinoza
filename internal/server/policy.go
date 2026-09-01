package server

import (
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

func routeKey(method, path string) string {
	return method + " " + path
}

var neededRole = map[string]string{
	routeKey(http.MethodPost, "/api/protection"):          auth.RoleAdmin,
	routeKey(http.MethodPost, "/api/helm/action"):         auth.RoleEditor,
	routeKey(http.MethodPost, "/api/helm/upgrade"):        auth.RoleEditor,
	routeKey(http.MethodPost, "/api/helm/install"):        auth.RoleEditor,
	routeKey(http.MethodPost, "/api/checks/baseline"):     auth.RoleEditor,
	routeKey(http.MethodDelete, "/api/checks/baseline"):   auth.RoleEditor,
	routeKey(http.MethodPut, "/api/checks/baseline/file"): auth.RoleEditor,
	routeKey(http.MethodPost, "/api/checks/mutes"):        auth.RoleEditor,
	routeKey(http.MethodDelete, "/api/checks/mutes"):      auth.RoleEditor,
	routeKey(http.MethodPost, "/api/flux/action"):         auth.RoleEditor,
	routeKey(http.MethodPost, "/api/argocd/action"):       auth.RoleEditor,
	routeKey(http.MethodPost, actionPath):                 auth.RoleEditor,
	routeKey(http.MethodPut, "/api/object"):               auth.RoleEditor,
	routeKey(http.MethodDelete, "/api/object"):            auth.RoleEditor,
	routeKey(http.MethodPost, "/api/debug"):               auth.RoleAdmin,
	routeKey(http.MethodGet, "/api/exec"):                 auth.RoleAdmin,
	routeKey(http.MethodGet, "/api/rbac"):                 auth.RoleAdmin,
	routeKey(http.MethodGet, "/api/rbac/who"):             auth.RoleAdmin,
	routeKey(http.MethodGet, "/api/nodeshell"):            auth.RoleAdmin,
	routeKey(http.MethodPost, "/api/clusters/timeline"):   auth.RoleAdmin,
	routeKey(http.MethodDelete, "/api/history"):           auth.RoleAdmin,
	routeKey(http.MethodPut, "/api/settings"):             auth.RoleAdmin,
}

var onlyWhenLocal = map[string]bool{
	routeKey(http.MethodGet, "/api/update"):              true,
	routeKey(http.MethodPost, "/api/update"):             true,
	routeKey(http.MethodPost, "/api/kubeconfigs"):        true,
	routeKey(http.MethodDelete, "/api/kubeconfigs"):      true,
	routeKey(http.MethodGet, "/api/kubeconfigs/picker"):  true,
	routeKey(http.MethodPost, "/api/kubeconfigs/picker"): true,
	routeKey(http.MethodPost, "/api/clusters"):           true,
	routeKey(http.MethodDelete, "/api/clusters"):         true,
	routeKey(http.MethodPost, "/api/clusters/active"):    true,
	routeKey(http.MethodPost, "/api/clusters/color"):     true,
	routeKey(http.MethodPost, "/api/clusters/name"):      true,
	routeKey(http.MethodPost, "/api/clusters/reopen"):    true,
	routeKey(http.MethodPost, "/api/view/browser"):       true,
	routeKey(http.MethodPost, "/api/view/desktop"):       true,
	routeKey(http.MethodGet, "/api/shell"):               true,
	routeKey(http.MethodPost, "/api/portforward"):        true,
	routeKey(http.MethodDelete, "/api/portforward"):      true,
}

const notInClusterMode = "spinoza is serving a cluster, so that only works when you run it yourself"

func roleFor(entry endpoint) string {
	return neededRole[routeKey(entry.method, entry.path)]
}

func onlyHere(entry endpoint) bool {
	return onlyWhenLocal[routeKey(entry.method, entry.path)]
}

var needsWholeCluster = map[string]bool{
	routeKey(http.MethodGet, "/api/resources/counts"):      true,
	routeKey(http.MethodGet, "/api/overview"):              true,
	routeKey(http.MethodGet, "/api/issues"):                true,
	routeKey(http.MethodGet, "/api/checks"):                true,
	routeKey(http.MethodGet, "/api/checks/findings"):       true,
	routeKey(http.MethodGet, "/api/checks/findings/fleet"): true,
	routeKey(http.MethodGet, "/api/checks/export"):         true,
	routeKey(http.MethodGet, "/api/checks/mutes"):          true,
	routeKey(http.MethodPost, "/api/checks/mutes"):         true,
	routeKey(http.MethodDelete, "/api/checks/mutes"):       true,
	routeKey(http.MethodGet, "/api/gitops/graph"):          true,
	routeKey(http.MethodGet, "/api/gitops/app"):            true,
	routeKey(http.MethodGet, "/api/gitops/app/graph"):      true,
	routeKey(http.MethodGet, "/api/topology"):              true,
	routeKey(http.MethodGet, "/api/flux"):                  true,
	routeKey(http.MethodGet, "/api/flux/overview"):         true,
	routeKey(http.MethodGet, "/api/argocd"):                true,
	routeKey(http.MethodGet, "/api/traffic"):               true,
	routeKey(http.MethodGet, "/api/metrics"):               true,
	routeKey(http.MethodGet, "/api/compare"):               true,
	routeKey(http.MethodGet, "/api/compare/kind"):          true,
	routeKey(http.MethodGet, "/api/resources/fleet"):       true,
	routeKey(http.MethodGet, "/api/images/fleet"):          true,
	routeKey(http.MethodGet, "/api/search/fleet"):          true,
	routeKey(http.MethodGet, "/api/gitops/fleet"):          true,
	routeKey(http.MethodGet, "/api/overview/fleet"):        true,
	routeKey(http.MethodGet, "/api/issues/fleet"):          true,
	routeKey(http.MethodGet, "/api/checks/fleet"):          true,
	routeKey(http.MethodGet, "/api/helm/fleet"):            true,
	routeKey(http.MethodPost, "/api/checks/baseline"):      true,
	routeKey(http.MethodGet, "/api/checks/baseline/file"):  true,
	routeKey(http.MethodGet, "/api/history"):               true,
	routeKey(http.MethodDelete, "/api/history"):            true,
}

const readsEverything = "this view reads the whole cluster, and your account reads named namespaces only"

const clusterWouldNotSay = "this view reads the whole cluster, and the cluster would not say whether your account may"

func wholeCluster(entry endpoint) bool {
	return needsWholeCluster[routeKey(entry.method, entry.path)]
}

const actionPath = "/api/action"

func actionRole(action actions.Action) string {
	if touchesNodes(action) {
		return auth.RoleAdmin
	}
	return neededRole[routeKey(http.MethodPost, actionPath)]
}

func touchesNodes(action actions.Action) bool {
	switch action {
	case actions.Cordon, actions.Uncordon, actions.Drain:
		return true
	default:
		return false
	}
}
