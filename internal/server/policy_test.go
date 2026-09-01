package server

import (
	"net/http"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

func everyRouteKey(t *testing.T) map[string]bool {
	t.Helper()
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	out := map[string]bool{}
	for _, entry := range srv.allRoutes() {
		out[routeKey(entry.method, entry.path)] = true
	}
	return out
}

func TestEveryPolicyLineNamesARouteThatExists(t *testing.T) {
	known := everyRouteKey(t)

	tables := map[string][]string{
		"neededRole":        keysOf(neededRole),
		"onlyWhenLocal":     keysOfBool(onlyWhenLocal),
		"needsWholeCluster": keysOfBool(needsWholeCluster),
	}
	for table, keys := range tables {
		for _, key := range keys {
			if !known[key] {
				t.Errorf("%s names %q, which is not a route; a typo here silently lets anyone in", table, key)
			}
		}
	}
}

func keysOf(held map[string]string) []string {
	out := make([]string, 0, len(held))
	for key := range held {
		out = append(out, key)
	}
	return out
}

func keysOfBool(held map[string]bool) []string {
	out := make([]string, 0, len(held))
	for key := range held {
		out = append(out, key)
	}
	return out
}

var readOnlyWrites = map[string]bool{
	routeKey(http.MethodPost, "/api/resources"):           true,
	routeKey(http.MethodPost, "/api/access"):              true,
	routeKey(http.MethodPost, "/api/checks/rules/faults"): true,
	routeKey(http.MethodPost, "/api/kubeconfigs"):         true,
	routeKey(http.MethodDelete, "/api/kubeconfigs"):       true,
	routeKey(http.MethodPost, "/api/kubeconfigs/picker"):  true,
	routeKey(http.MethodPost, "/api/clusters"):            true,
	routeKey(http.MethodDelete, "/api/clusters"):          true,
	routeKey(http.MethodPost, "/api/clusters/active"):     true,
	routeKey(http.MethodPost, "/api/clusters/color"):      true,
	routeKey(http.MethodPost, "/api/clusters/name"):       true,
	routeKey(http.MethodPost, "/api/clusters/reopen"):     true,
	routeKey(http.MethodPost, "/api/view/browser"):        true,
	routeKey(http.MethodPost, "/api/view/desktop"):        true,
	routeKey(http.MethodPost, "/api/update"):              true,
	routeKey(http.MethodPost, "/auth/backchannel-logout"): true,
}

func TestEveryRouteThatChangesSomethingNamesARoleOrIsListedAsHarmless(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})

	for _, entry := range srv.allRoutes() {
		if !mutating(entry.method) {
			continue
		}
		key := routeKey(entry.method, entry.path)
		if neededRole[key] != "" || onlyWhenLocal[key] || readOnlyWrites[key] {
			continue
		}
		t.Errorf("%s changes something and needs no role; add it to neededRole or say why it is harmless", key)
	}
}

func TestARouteWithNoPolicyIsHandedStraightThrough(t *testing.T) {
	entry := endpoint{method: http.MethodGet, path: "/api/version"}

	if roleFor(entry) != "" {
		t.Fatal("a plain read was given a role")
	}
	if onlyHere(entry) || wholeCluster(entry) {
		t.Fatal("a plain read was gated")
	}
}

func TestTheActionsThatTouchNodesAreNamed(t *testing.T) {
	for _, action := range []actions.Action{actions.Cordon, actions.Uncordon, actions.Drain} {
		if !touchesNodes(action) {
			t.Fatalf("%s changes a node and was not treated as one", action)
		}
	}
	for _, action := range []actions.Action{actions.Scale, actions.Restart, actions.Suspend} {
		if touchesNodes(action) {
			t.Fatalf("%s does not touch a node and was treated as one", action)
		}
	}
}

func TestExecAndPortForwardingAreAdminOnly(t *testing.T) {
	admin := []string{
		routeKey(http.MethodGet, "/api/rbac"),
		routeKey(http.MethodGet, "/api/rbac/who"),
		routeKey(http.MethodGet, "/api/exec"),
		routeKey(http.MethodGet, "/api/nodeshell"),
		routeKey(http.MethodPost, "/api/debug"),
		routeKey(http.MethodPut, "/api/settings"),
	}
	for _, key := range admin {
		if neededRole[key] != auth.RoleAdmin {
			t.Errorf("%s is %q, want %q", key, neededRole[key], auth.RoleAdmin)
		}
	}
}

func TestSharedHistoryAndMutesNeedTheWholeCluster(t *testing.T) {
	shared := []string{
		routeKey(http.MethodGet, "/api/history"),
		routeKey(http.MethodDelete, "/api/history"),
		routeKey(http.MethodGet, "/api/checks/mutes"),
		routeKey(http.MethodPost, "/api/checks/mutes"),
		routeKey(http.MethodDelete, "/api/checks/mutes"),
	}
	for _, key := range shared {
		if !needsWholeCluster[key] {
			t.Errorf("%s exposes shared cluster records without a whole-cluster scope check", key)
		}
	}
}
