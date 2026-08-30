package rbac

import (
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func role(name, namespace string, rules ...any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"rules":      rules,
	}}
}

func clusterRole(name string, rules ...any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata":   map[string]any{"name": name},
		"rules":      rules,
	}}
}

func rule(verbs, groups, resources []string) map[string]any {
	return map[string]any{
		"verbs":     asAny(verbs),
		"apiGroups": asAny(groups),
		"resources": asAny(resources),
	}
}

func asAny(held []string) []any {
	out := make([]any, 0, len(held))
	for _, one := range held {
		out = append(out, one)
	}
	return out
}

func binding(name, namespace, roleKind, roleName string, subjects ...any) *unstructured.Unstructured {
	kind := "RoleBinding"
	meta := map[string]any{"name": name, "namespace": namespace}
	if namespace == "" {
		kind = "ClusterRoleBinding"
		meta = map[string]any{"name": name}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       kind,
		"metadata":   meta,
		"roleRef":    map[string]any{"kind": roleKind, "name": roleName},
		"subjects":   subjects,
	}}
}

func account(name, namespace string) map[string]any {
	return map[string]any{"kind": "ServiceAccount", "name": name, "namespace": namespace}
}

func user(name string) map[string]any {
	return map[string]any{"kind": "User", "name": name}
}

func holderFor(t *testing.T, index Index, label string) Holder {
	t.Helper()
	for _, one := range index.Holders {
		if one.Subject.Label() == label {
			return one
		}
	}
	t.Fatalf("no holder called %q in %+v", label, labels(index))
	return Holder{}
}

func labels(index Index) []string {
	out := make([]string, 0, len(index.Holders))
	for _, one := range index.Holders {
		out = append(out, one.Subject.Label())
	}
	return out
}

func TestABindingBecomesASubjectThatHoldsIt(t *testing.T) {
	index := Build(Held{
		Roles:    []*unstructured.Unstructured{role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		Bindings: []*unstructured.Unstructured{binding("read-web", "web", "Role", "reader", account("api", "web"))},
	})

	held := holderFor(t, index, "system:serviceaccount:web:api")

	if len(held.Grants) != 1 {
		t.Fatalf("grants = %+v", held.Grants)
	}
	if held.Grants[0].Role != "reader" || held.Grants[0].Namespace != "web" {
		t.Fatalf("the grant came back as %+v", held.Grants[0])
	}
}

func TestAServiceAccountWithNoNamespaceBelongsToTheBindings(t *testing.T) {
	index := Build(Held{
		Roles:    []*unstructured.Unstructured{role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		Bindings: []*unstructured.Unstructured{binding("read-web", "web", "Role", "reader", map[string]any{"kind": "ServiceAccount", "name": "api"})},
	})

	holderFor(t, index, "system:serviceaccount:web:api")
}

func TestAClusterRoleBindingReachesEveryNamespace(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"secrets"}))},
		ClusterBinds: []*unstructured.Unstructured{binding("read-all", "", "ClusterRole", "reader", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if !held.Grants[0].Everywhere() {
		t.Fatalf("the grant was confined to %q", held.Grants[0].Namespace)
	}
	if !held.Grants[0].Reaches(Ask{Verb: "get", Resource: "secrets", Namespace: "anywhere"}) {
		t.Fatal("a cluster-wide grant did not reach a namespace")
	}
}

// The corner people read the wrong way round: a RoleBinding to a ClusterRole
// takes the cluster role's rules but applies only where the binding is.
func TestARoleBindingToAClusterRoleStaysInItsNamespace(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"secrets"}))},
		Bindings:     []*unstructured.Unstructured{binding("read-web", "web", "ClusterRole", "reader", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if held.Grants[0].Everywhere() {
		t.Fatal("a role binding to a cluster role reached the whole cluster")
	}
	if !held.Grants[0].Reaches(Ask{Verb: "get", Resource: "secrets", Namespace: "web"}) {
		t.Fatal("it did not reach its own namespace")
	}
	if held.Grants[0].Reaches(Ask{Verb: "get", Resource: "secrets", Namespace: "other"}) {
		t.Fatal("it reached a namespace it was never bound in")
	}
}

func TestARoleIsLookedUpInItsOwnNamespace(t *testing.T) {
	index := Build(Held{
		Roles: []*unstructured.Unstructured{
			role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"})),
			role("reader", "db", rule([]string{"delete"}, []string{""}, []string{"pods"})),
		},
		Bindings: []*unstructured.Unstructured{binding("read-web", "web", "Role", "reader", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if held.Grants[0].Reaches(Ask{Verb: "delete", Resource: "pods", Namespace: "web"}) {
		t.Fatal("it picked up the other namespace's role of the same name")
	}
}

func TestABindingThatNamesARoleNobodyWroteIsReported(t *testing.T) {
	index := Build(Held{
		Bindings: []*unstructured.Unstructured{binding("read-web", "web", "Role", "gone", user("ana"))},
	})

	if len(index.Absent) != 1 {
		t.Fatalf("absent = %+v", index.Absent)
	}
	held := holderFor(t, index, "ana")
	if !held.Grants[0].Missing {
		t.Fatal("the grant did not say its role is missing")
	}
	if held.Grants[0].Reaches(Ask{Verb: "get", Resource: "pods", Namespace: "web"}) {
		t.Fatal("a missing role granted something")
	}
}

func TestAWildcardVerbCoversTheOneAsked(t *testing.T) {
	one := Rule{Verbs: []string{Anything}, Groups: []string{""}, Resources: []string{"pods"}}

	if !one.Allows(Ask{Verb: "delete", Resource: "pods"}) {
		t.Fatal("a wildcard verb did not cover delete")
	}
}

func TestAWildcardResourceCoversTheOneAsked(t *testing.T) {
	one := Rule{Verbs: []string{"get"}, Groups: []string{Anything}, Resources: []string{Anything}}

	if !one.Allows(Ask{Verb: "get", Group: "apps", Resource: "deployments"}) {
		t.Fatal("a wildcard resource did not cover deployments")
	}
}

func TestTheApiGroupHasToMatch(t *testing.T) {
	one := Rule{Verbs: []string{"get"}, Groups: []string{""}, Resources: []string{"deployments"}}

	if one.Allows(Ask{Verb: "get", Group: "apps", Resource: "deployments"}) {
		t.Fatal("a core-group rule answered for an apps-group question")
	}
}

// The distinction the whole feature exists for: holding pods is not holding
// pods/exec.
func TestPodsDoesNotCoverPodsExec(t *testing.T) {
	one := Rule{Verbs: []string{"create"}, Groups: []string{""}, Resources: []string{"pods"}}

	if one.Allows(Ask{Verb: "create", Resource: "pods/exec"}) {
		t.Fatal("a rule on pods answered for pods/exec")
	}
}

func TestASubresourceWildcardCoversIt(t *testing.T) {
	one := Rule{Verbs: []string{"create"}, Groups: []string{""}, Resources: []string{"pods/*"}}

	if !one.Allows(Ask{Verb: "create", Resource: "pods/exec"}) {
		t.Fatal("pods/* did not cover pods/exec")
	}
}

func TestARuleWithNoResourcesAllowsNothing(t *testing.T) {
	one := Rule{Verbs: []string{Anything}, Groups: []string{Anything}, URLs: []string{"/healthz"}}

	if one.Allows(Ask{Verb: "get", Resource: "pods"}) {
		t.Fatal("a non-resource rule answered a resource question")
	}
}

func TestWhoNamesEverySubjectThatMay(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{
			clusterRole("exec", rule([]string{"create"}, []string{""}, []string{"pods/exec"})),
			clusterRole("look", rule([]string{"get"}, []string{""}, []string{"pods"})),
		},
		ClusterBinds: []*unstructured.Unstructured{
			binding("can-exec", "", "ClusterRole", "exec", user("ana")),
			binding("can-look", "", "ClusterRole", "look", user("bo")),
		},
	})

	found := index.Who(Ask{Verb: "create", Resource: "pods/exec"})

	if len(found) != 1 || found[0].Subject.Name != "ana" {
		t.Fatalf("who = %+v", labelsOf(found))
	}
}

func TestWhoKeepsOnlyTheGrantsThatAnswer(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{
			clusterRole("exec", rule([]string{"create"}, []string{""}, []string{"pods/exec"})),
			clusterRole("look", rule([]string{"get"}, []string{""}, []string{"pods"})),
		},
		ClusterBinds: []*unstructured.Unstructured{
			binding("can-exec", "", "ClusterRole", "exec", user("ana")),
			binding("can-look", "", "ClusterRole", "look", user("ana")),
		},
	})

	found := index.Who(Ask{Verb: "create", Resource: "pods/exec"})

	if len(found[0].Grants) != 1 || found[0].Grants[0].Role != "exec" {
		t.Fatalf("grants = %+v", found[0].Grants)
	}
}

func TestWhoInOneNamespaceLeavesOutAnother(t *testing.T) {
	index := Build(Held{
		Roles: []*unstructured.Unstructured{
			role("reader", "web", rule([]string{"get"}, []string{""}, []string{"secrets"})),
			role("reader", "db", rule([]string{"get"}, []string{""}, []string{"secrets"})),
		},
		Bindings: []*unstructured.Unstructured{
			binding("read-web", "web", "Role", "reader", user("ana")),
			binding("read-db", "db", "Role", "reader", user("bo")),
		},
	})

	found := index.Who(Ask{Verb: "get", Resource: "secrets", Namespace: "web"})

	if len(found) != 1 || found[0].Subject.Name != "ana" {
		t.Fatalf("who = %+v", labelsOf(found))
	}
}

func labelsOf(held []Holder) []string {
	out := make([]string, 0, len(held))
	for _, one := range held {
		out = append(out, one.Subject.Label())
	}
	return out
}

func TestClusterAdminIsSaidOnItsOwn(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{
			clusterRole("cluster-admin", rule([]string{Anything}, []string{Anything}, []string{Anything})),
		},
		ClusterBinds: []*unstructured.Unstructured{binding("admins", "", "ClusterRole", "cluster-admin", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if !slices.Equal(held.Powers, []string{"cluster-admin"}) {
		t.Fatalf("powers = %+v, want cluster-admin alone", held.Powers)
	}
}

func TestThePowersAreNamedForWhatTheyReallyReach(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{
			clusterRole("ops",
				rule([]string{"get", "list"}, []string{""}, []string{"secrets"}),
				rule([]string{"create"}, []string{""}, []string{"pods/exec"})),
		},
		ClusterBinds: []*unstructured.Unstructured{binding("ops", "", "ClusterRole", "ops", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	for _, want := range []string{"reads secrets", "lists secrets", "execs into pods"} {
		if !slices.Contains(held.Powers, want) {
			t.Fatalf("powers = %+v, want %q", held.Powers, want)
		}
	}
}

func TestASubjectWithNothingWorthNamingSaysNothing(t *testing.T) {
	index := Build(Held{
		Roles:    []*unstructured.Unstructured{role("reader", "web", rule([]string{"get"}, []string{""}, []string{"configmaps"}))},
		Bindings: []*unstructured.Unstructured{binding("read-web", "web", "Role", "reader", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if len(held.Powers) != 0 {
		t.Fatalf("powers = %+v", held.Powers)
	}
}

func TestTheFurthestReachingSubjectComesFirst(t *testing.T) {
	index := Build(Held{
		Roles:        []*unstructured.Unstructured{role("quiet", "web", rule([]string{"get"}, []string{""}, []string{"configmaps"}))},
		ClusterRoles: []*unstructured.Unstructured{clusterRole("admin", rule([]string{Anything}, []string{Anything}, []string{Anything}))},
		Bindings:     []*unstructured.Unstructured{binding("quiet", "web", "Role", "quiet", user("bo"))},
		ClusterBinds: []*unstructured.Unstructured{binding("admin", "", "ClusterRole", "admin", user("ana"))},
	})

	if index.Holders[0].Subject.Name != "ana" {
		t.Fatalf("the order was %+v", labels(index))
	}
}

func TestASubjectSaysWhereItsGrantsApply(t *testing.T) {
	index := Build(Held{
		Roles: []*unstructured.Unstructured{
			role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"})),
			role("reader", "db", rule([]string{"get"}, []string{""}, []string{"pods"})),
		},
		Bindings: []*unstructured.Unstructured{
			binding("web", "web", "Role", "reader", user("ana")),
			binding("db", "db", "Role", "reader", user("ana")),
		},
	})

	held := holderFor(t, index, "ana")

	if !slices.Equal(held.Namespaces(), []string{"db", "web"}) {
		t.Fatalf("namespaces = %+v", held.Namespaces())
	}
}

func TestASubjectWithACurrentGrantIsNotConfinedToNamespaces(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		ClusterBinds: []*unstructured.Unstructured{binding("all", "", "ClusterRole", "reader", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if held.Namespaces() != nil {
		t.Fatalf("a cluster-wide subject was confined to %+v", held.Namespaces())
	}
}

func TestAnAggregatedRoleWithNoRulesYetSaysSo(t *testing.T) {
	role := clusterRole("view")
	role.Object["aggregationRule"] = map[string]any{"clusterRoleSelectors": []any{}}
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{role},
		ClusterBinds: []*unstructured.Unstructured{binding("view", "", "ClusterRole", "view", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if !held.Grants[0].Aggregated {
		t.Fatal("an aggregated role did not say it is filled in by a controller")
	}
	if held.Grants[0].RuleCount != 0 {
		t.Fatalf("rules = %d", held.Grants[0].RuleCount)
	}
}

func TestABindingWithNoRoleRefIsSkipped(t *testing.T) {
	odd := binding("odd", "web", "Role", "reader", user("ana"))
	delete(odd.Object, "roleRef")

	index := Build(Held{Bindings: []*unstructured.Unstructured{odd}})

	if len(index.Holders) != 0 {
		t.Fatalf("holders = %+v", labels(index))
	}
}

func TestASubjectWithNoNameIsSkipped(t *testing.T) {
	index := Build(Held{
		Roles:    []*unstructured.Unstructured{role("reader", "web", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		Bindings: []*unstructured.Unstructured{binding("read", "web", "Role", "reader", map[string]any{"kind": "User"})},
	})

	if len(index.Holders) != 0 {
		t.Fatalf("holders = %+v", labels(index))
	}
}

func TestAGroupKeepsItsOwnName(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		ClusterBinds: []*unstructured.Unstructured{
			binding("all", "", "ClusterRole", "reader", map[string]any{"kind": "Group", "name": "system:authenticated"}),
		},
	})

	held := holderFor(t, index, "system:authenticated")

	if held.Subject.Kind != KindGroup {
		t.Fatalf("the subject came back as %+v", held.Subject)
	}
}

func TestOneSubjectBoundTwiceKeepsBothGrants(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		ClusterBinds: []*unstructured.Unstructured{
			binding("one", "", "ClusterRole", "reader", user("ana")),
			binding("two", "", "ClusterRole", "reader", user("ana")),
		},
	})

	held := holderFor(t, index, "ana")

	if len(held.Grants) != 2 {
		t.Fatalf("grants = %+v", held.Grants)
	}
}

func TestAClusterBindingWithAMissingRoleIsNamedWithoutANamespace(t *testing.T) {
	index := Build(Held{
		ClusterBinds: []*unstructured.Unstructured{binding("gone", "", "ClusterRole", "nowhere", user("ana"))},
	})

	if len(index.Absent) != 1 || index.Absent[0] != "ClusterRoleBinding gone wants ClusterRole nowhere" {
		t.Fatalf("absent = %+v", index.Absent)
	}
}

func TestARuleThatIsNotAMapIsSkipped(t *testing.T) {
	odd := clusterRole("odd", "not a rule")
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{odd},
		ClusterBinds: []*unstructured.Unstructured{binding("odd", "", "ClusterRole", "odd", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if held.Grants[0].RuleCount != 0 {
		t.Fatalf("rules = %d", held.Grants[0].RuleCount)
	}
}

func TestASubjectThatIsNotAMapIsSkipped(t *testing.T) {
	odd := binding("odd", "", "ClusterRole", "reader", "not a subject", user("ana"))
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		ClusterBinds: []*unstructured.Unstructured{odd},
	})

	if len(index.Holders) != 1 {
		t.Fatalf("holders = %+v", labels(index))
	}
}

func TestAVerbListThatIsNotStringsIsIgnored(t *testing.T) {
	odd := clusterRole("odd", map[string]any{
		"verbs":     []any{true},
		"apiGroups": []any{""},
		"resources": []any{"pods"},
	})
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{odd},
		ClusterBinds: []*unstructured.Unstructured{binding("odd", "", "ClusterRole", "odd", user("ana"))},
	})

	held := holderFor(t, index, "ana")

	if held.Grants[0].Reaches(Ask{Verb: "get", Resource: "pods"}) {
		t.Fatal("a rule with no readable verbs granted something")
	}
}

func TestAServiceAccountWithNoNamespaceAnywhereKeepsItsBareName(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"pods"}))},
		ClusterBinds: []*unstructured.Unstructured{
			binding("all", "", "ClusterRole", "reader", map[string]any{"kind": "ServiceAccount", "name": "orphan"}),
		},
	})

	holderFor(t, index, "orphan")
}

func TestTwoSubjectsWithTheSamePowersAreOrderedByName(t *testing.T) {
	index := Build(Held{
		ClusterRoles: []*unstructured.Unstructured{clusterRole("reader", rule([]string{"get"}, []string{""}, []string{"configmaps"}))},
		ClusterBinds: []*unstructured.Unstructured{
			binding("one", "", "ClusterRole", "reader", user("zoe")),
			binding("two", "", "ClusterRole", "reader", user("ana")),
		},
	})

	if index.Holders[0].Subject.Name != "ana" {
		t.Fatalf("the order was %+v", labels(index))
	}
}
