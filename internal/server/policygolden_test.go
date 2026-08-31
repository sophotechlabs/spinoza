package server

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
)

const policyGolden = "testdata/route-policy.txt"

func policyLines(t *testing.T) string {
	t.Helper()
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	routes := srv.allRoutes()
	lines := make([]string, 0, len(routes))
	for _, entry := range routes {
		lines = append(lines, fmt.Sprintf(
			"%s %s role=%s local=%s whole=%s",
			entry.method,
			entry.path,
			orDash(roleFor(entry)),
			yesNo(onlyHere(entry)),
			yesNo(wholeCluster(entry)),
		))
	}
	for _, action := range actions.Every() {
		lines = append(lines, fmt.Sprintf("action %s role=%s", action, actionRole(action)))
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

func orDash(role string) string {
	if role == "" {
		return "-"
	}
	return role
}

func yesNo(held bool) string {
	if held {
		return "yes"
	}
	return "no"
}

func TestEveryRouteHasBeenGivenAPolicy(t *testing.T) {
	got := policyLines(t)
	if os.Getenv("UPDATE_ROUTE_POLICY") == "1" {
		if err := os.WriteFile(policyGolden, []byte(got), 0o600); err != nil {
			t.Fatalf("writing %s: %v", policyGolden, err)
		}
		t.Fatalf("route policy updated; re-run to confirm, and check the new lines say what you meant")
	}
	want, err := os.ReadFile(policyGolden)
	if err != nil {
		t.Fatalf("read %s: %v", policyGolden, err)
	}
	if got == string(want) {
		return
	}
	t.Fatalf(`the routes or what they are gated on changed.

Cluster mode decides three things per route: the role it needs, whether it only
works when you run spinoza yourself, and whether it reads the whole cluster. A
route with none of them is open to every signed-in reader, which is the right
answer for most reads and the wrong one for anything that changes the cluster or
gathers it up. The action lines say the same for each thing POST /api/action can
do, because a new one that reaches a node needs admin rather than editor. Decide,
then refresh the golden with:

    UPDATE_ROUTE_POLICY=1 go test ./internal/server/

diff (want on the left, got on the right):

%s`, sideBySide(string(want), got))
}

func sideBySide(want, got string) string {
	wanted := strings.Split(strings.TrimSpace(want), "\n")
	found := strings.Split(strings.TrimSpace(got), "\n")
	out := []string{}
	for _, line := range found {
		if !slices.Contains(wanted, line) {
			out = append(out, "  added:   "+line)
		}
	}
	for _, line := range wanted {
		if !slices.Contains(found, line) {
			out = append(out, "  gone:    "+line)
		}
	}
	return strings.Join(out, "\n")
}
