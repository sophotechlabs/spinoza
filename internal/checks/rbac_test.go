package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func role(kind, name string, rules ...map[string]any) *unstructured.Unstructured {
	namespace := testNamespace
	if kind == "ClusterRole" {
		namespace = ""
	}
	obj := simple(kind, name, namespace, nil)
	listed := make([]any, 0, len(rules))
	for _, one := range rules {
		listed = append(listed, one)
	}
	obj.Object["rules"] = listed
	return obj
}

func grants(verbs, resources, groups []string) map[string]any {
	return map[string]any{
		"verbs":     anyList(verbs),
		"resources": anyList(resources),
		"apiGroups": anyList(groups),
	}
}

func anyList(names []string) []any {
	out := make([]any, 0, len(names))
	for _, name := range names {
		out = append(out, name)
	}
	return out
}

func binding(kind, name, roleName string, subjects ...string) *unstructured.Unstructured {
	namespace := testNamespace
	if kind == "ClusterRoleBinding" {
		namespace = ""
	}
	obj := simple(kind, name, namespace, nil)
	obj.Object["roleRef"] = map[string]any{"kind": "ClusterRole", "name": roleName}
	listed := make([]any, 0, len(subjects))
	for _, one := range subjects {
		listed = append(listed, map[string]any{"kind": "Group", "name": one})
	}
	obj.Object["subjects"] = listed
	return obj
}

func plainRole() *unstructured.Unstructured {
	return role("Role", "reader", grants([]string{"get"}, []string{"configmaps"}, []string{""}))
}

func TestEveryRbacCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	cases := []struct {
		id      string
		objects []*unstructured.Unstructured
	}{
		{
			id:      "rbac-wildcard-verbs",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{anything}, []string{"pods"}, []string{""}))},
		},
		{
			id:      "rbac-wildcard-resources",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"get"}, []string{anything}, []string{""}))},
		},
		{
			id:      "rbac-wildcard-api-groups",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"get"}, []string{"pods"}, []string{anything}))},
		},
		{
			id:      "rbac-escalate-or-bind",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"escalate"}, []string{"roles"}, []string{rbacGroup}))},
		},
		{
			id:      "rbac-impersonate",
			objects: []*unstructured.Unstructured{role("ClusterRole", "wide", grants([]string{"impersonate"}, []string{"users"}, []string{""}))},
		},
		{
			id:      "rbac-read-secrets",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"list"}, []string{"secrets"}, []string{""}))},
		},
		{
			id:      "rbac-pod-exec",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"create"}, []string{"pods/exec"}, []string{""}))},
		},
		{
			id:      "rbac-pod-portforward",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"create"}, []string{"pods/portforward"}, []string{""}))},
		},
		{
			id:      "rbac-pod-logs",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"get"}, []string{"pods/log"}, []string{""}))},
		},
		{
			id:      "rbac-create-pods",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"create"}, []string{"pods"}, []string{""}))},
		},
		{
			id:      "rbac-write-bindings",
			objects: []*unstructured.Unstructured{role("Role", "wide", grants([]string{"create"}, []string{"rolebindings"}, []string{rbacGroup}))},
		},
		{
			id: "rbac-write-webhooks",
			objects: []*unstructured.Unstructured{role("ClusterRole", "wide",
				grants([]string{"patch"}, []string{"validatingwebhookconfigurations"}, []string{"admissionregistration.k8s.io"}))},
		},
		{
			id:      "rbac-node-proxy",
			objects: []*unstructured.Unstructured{role("ClusterRole", "wide", grants([]string{"get"}, []string{"nodes/proxy"}, []string{""}))},
		},
		{
			id:      "cluster-admin-bound",
			objects: []*unstructured.Unstructured{binding("ClusterRoleBinding", "everything", "cluster-admin", "ops")},
		},
		{
			id:      "bound-to-everyone",
			objects: []*unstructured.Unstructured{binding("RoleBinding", "open", "reader", "system:authenticated")},
		},
	}

	registered := map[string]bool{}
	for _, entry := range rbacChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered RBAC checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered RBAC check", tc.id)
			}
			if findingCount(t, report(t, tc.objects...), tc.id) == 0 {
				t.Fatalf("%s did not fire on the role written to trip it", tc.id)
			}
			clean := report(t, plainRole(), binding("RoleBinding", "narrow", "reader", "api"))
			if findingCount(t, clean, tc.id) != 0 {
				t.Fatalf("%s fired on a role that only reads ConfigMaps", tc.id)
			}
		})
	}
}

func TestASecretRuleNamingItsSecretsIsNotFlagged(t *testing.T) {
	narrow := role("Role", "reader", map[string]any{
		"verbs":         anyList([]string{"get"}),
		"resources":     anyList([]string{"secrets"}),
		"apiGroups":     anyList([]string{""}),
		"resourceNames": anyList([]string{"api-token"}),
	})

	if findingCount(t, report(t, narrow), "rbac-read-secrets") != 0 {
		t.Fatal("a rule naming the one secret it reads was flagged")
	}
}

func TestWritingSecretsWithoutReadingThemIsNotTheReadCheck(t *testing.T) {
	writer := role("Role", "writer", grants([]string{"create"}, []string{"secrets"}, []string{""}))

	if findingCount(t, report(t, writer), "rbac-read-secrets") != 0 {
		t.Fatal("create on secrets was reported as reading them")
	}
}

func TestAWildcardResourceCoversTheNamedOnes(t *testing.T) {
	wide := role("ClusterRole", "wide", grants([]string{"create"}, []string{anything}, []string{""}))

	found := report(t, wide)

	if findingCount(t, found, "rbac-create-pods") != 1 {
		t.Fatal("a wildcard resource did not count as covering pods")
	}
}

func TestReadingPodsIsNotCreatingThem(t *testing.T) {
	reader := role("Role", "reader", grants([]string{"get", "list"}, []string{"pods"}, []string{""}))

	if findingCount(t, report(t, reader), "rbac-create-pods") != 0 {
		t.Fatal("read access to pods was reported as create")
	}
}

func TestABindingToANamedAccountIsNotABindingToEveryone(t *testing.T) {
	narrow := binding("RoleBinding", "narrow", "reader", "api")

	if findingCount(t, report(t, narrow), "bound-to-everyone") != 0 {
		t.Fatal("a binding to one account was reported as open")
	}
}

func TestABindingToSomethingOtherThanClusterAdminIsNotReported(t *testing.T) {
	narrow := binding("ClusterRoleBinding", "view", "view", "ops")

	if findingCount(t, report(t, narrow), "cluster-admin-bound") != 0 {
		t.Fatal("a binding to view was reported as cluster-admin")
	}
}

func TestAClusterAdminBindingWithNoSubjectsIsNotReported(t *testing.T) {
	empty := binding("ClusterRoleBinding", "orphan", "cluster-admin")

	if findingCount(t, report(t, empty), "cluster-admin-bound") != 0 {
		t.Fatal("a binding granting cluster-admin to nobody was reported")
	}
}

func TestTheDetailNamesWhatTheRuleReaches(t *testing.T) {
	wide := role("ClusterRole", "wide",
		grants([]string{anything}, []string{"pods", "services", "secrets", "configmaps"}, []string{""}))

	detail := onlyFinding(t, report(t, wide), "rbac-wildcard-verbs").Detail
	if !strings.Contains(detail, "and more") {
		t.Fatalf("detail was %q, want a bounded list", detail)
	}
}

func TestRulesAndSubjectsOfTheWrongShapeAreSkipped(t *testing.T) {
	odd := simple("Role", "odd", testNamespace, nil)
	odd.Object["rules"] = []any{"not-an-object", map[string]any{"verbs": "not-a-list"}}
	oddBinding := simple("RoleBinding", "odd", testNamespace, nil)
	oddBinding.Object["subjects"] = []any{"not-an-object", map[string]any{"kind": "Group"}}

	found := report(t, odd, oddBinding)

	for _, entry := range rbacChecks() {
		if findingCount(t, found, entry.id) != 0 {
			t.Fatalf("%s reported something from an object of the wrong shape", entry.id)
		}
	}
}

func TestAMalformedRuleValueDoesNotHideFollowingPermissions(t *testing.T) {
	wide := role("Role", "wide", map[string]any{
		"verbs":     []any{false, anything},
		"resources": anyList([]string{"pods"}),
		"apiGroups": anyList([]string{""}),
	})

	if findingCount(t, report(t, wide), "rbac-wildcard-verbs") != 1 {
		t.Fatal("a malformed value hid a later wildcard verb")
	}
}

func TestMalformedRuleEntriesDoNotHideLaterPermissions(t *testing.T) {
	wide := role("Role", "wide", map[string]any{
		"verbs":     []any{int64(7), "get"},
		"resources": []any{int64(7), "secrets"},
		"apiGroups": []any{int64(7), ""},
	})

	if findingCount(t, report(t, wide), "rbac-read-secrets") != 1 {
		t.Fatal("malformed rule entries hid a later sensitive permission")
	}
}

func TestARoleWithNoRulesAtAllSaysNothing(t *testing.T) {
	bare := simple("ClusterRole", "aggregator", "", nil)

	found := report(t, bare)

	for _, entry := range rbacChecks() {
		if findingCount(t, found, entry.id) != 0 {
			t.Fatalf("%s fired on a role holding no rules", entry.id)
		}
	}
}

func TestTheRbacChecksAreSkippedWithoutTheWiderCorpus(t *testing.T) {
	report := Run(t.Context(), newLister(), descriptors(), api.Metrics{}, Filter{}, 0)

	if group := groupNamed(t, report, "rbac-wildcard-verbs"); group.Skipped == "" {
		t.Fatal("an RBAC check ran on a workload-only audit")
	}
}
