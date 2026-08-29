package checks

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const rbacGroup = "rbac.authorization.k8s.io"

const anything = "*"

var (
	roleTarget        = target{group: rbacGroup, resource: "roles"}
	clusterRoleTarget = target{group: rbacGroup, resource: "clusterroles"}
	bindingTarget     = target{group: rbacGroup, resource: "rolebindings"}
	clusterBindTarget = target{group: rbacGroup, resource: "clusterrolebindings"}
)

var roleKinds = []struct {
	resource string
	kind     string
	key      target
}{
	{resource: "roles", kind: "Role", key: roleTarget},
	{resource: "clusterroles", kind: "ClusterRole", key: clusterRoleTarget},
}

var bindingKinds = []struct {
	resource string
	kind     string
	key      target
}{
	{resource: "rolebindings", kind: "RoleBinding", key: bindingTarget},
	{resource: "clusterrolebindings", kind: "ClusterRoleBinding", key: clusterBindTarget},
}

var openSubjects = []string{
	"system:anonymous",
	"system:unauthenticated",
	"system:authenticated",
	"system:masters",
}

var writeVerbs = []string{"create", "update", "patch", "delete", "deletecollection", anything}

var readVerbs = []string{"get", "list", "watch", anything}

type rule struct {
	verbs     []string
	resources []string
	groups    []string
	names     []string
}

func rbacChecks() []check {
	return []check{
		ruleCheck("rbac-wildcard-verbs", "Role grants every verb", severityHigh,
			"A rule with * verbs grants delete and patch alongside the read the author wanted.",
			"Name the verbs the subject needs.",
			func(one rule) string {
				if !slices.Contains(one.verbs, anything) {
					return ""
				}
				return "grants every verb on " + listed(one.resources)
			}),
		ruleCheck("rbac-wildcard-resources", "Role grants every resource", severityHigh,
			"A rule with * resources reaches kinds nobody had in mind when they wrote it, including ones installed later.",
			"Name the resources the subject needs.",
			func(one rule) string {
				if !slices.Contains(one.resources, anything) {
					return ""
				}
				return "grants " + listed(one.verbs) + " on every resource"
			}),
		ruleCheck("rbac-wildcard-api-groups", "Role reaches every API group", severityHigh,
			"A rule with * apiGroups covers every CRD in the cluster, present and future.",
			"Name the API groups the subject needs.",
			func(one rule) string {
				if !slices.Contains(one.groups, anything) {
					return ""
				}
				return "reaches every API group"
			}),
		ruleCheck("rbac-escalate-or-bind", "Role may grant itself more", severityHigh,
			"escalate and bind let the holder award permissions it does not have, which is a way around every other limit.",
			"Remove escalate and bind unless this is a controller that genuinely delegates.",
			func(one rule) string {
				return namesVerb(one, []string{"escalate", "bind"}, "may ")
			}),
		ruleCheck("rbac-impersonate", "Role may act as somebody else", severityHigh,
			"impersonate lets the holder make requests as any user, group or service account.",
			"Remove impersonate.",
			func(one rule) string {
				return namesVerb(one, []string{"impersonate"}, "may ")
			}),
		ruleCheck("rbac-read-secrets", "Role reads Secrets", severityHigh,
			"Reading Secrets across a namespace is reading every credential in it.",
			"Name the individual secrets with resourceNames, or drop the rule.",
			func(one rule) string {
				if len(one.names) > 0 || !covers(one.resources, "secrets") {
					return ""
				}
				if !overlaps(one.verbs, readVerbs) {
					return ""
				}
				return "reads every Secret it can reach"
			}),
		ruleCheck("rbac-pod-exec", "Role may exec into pods", severityHigh,
			"A shell in any pod is that pod's service account token and everything it can read.",
			"Remove pods/exec and pods/attach.",
			func(one rule) string {
				return namesResource(one, []string{"pods/exec", "pods/attach"}, writeVerbs)
			}),
		ruleCheck("rbac-pod-portforward", "Role may forward pod ports", severityMedium,
			"Port-forwarding reaches anything the pod can reach, around every NetworkPolicy.",
			"Remove pods/portforward.",
			func(one rule) string {
				return namesResource(one, []string{"pods/portforward"}, writeVerbs)
			}),
		ruleCheck("rbac-pod-logs", "Role reads pod logs", severityMedium,
			"Logs carry whatever the application printed, which is often more than the author meant.",
			"Remove pods/log where it is not needed.",
			func(one rule) string {
				return namesResource(one, []string{"pods/log"}, readVerbs)
			}),
		ruleCheck("rbac-create-pods", "Role may create pods", severityHigh,
			"Creating a pod means choosing its service account, so this is a way to become any account in the namespace.",
			"Remove create on pods, or restrict the namespaces this applies to.",
			func(one rule) string {
				if !covers(one.resources, "pods") || !overlaps(one.verbs, []string{"create", anything}) {
					return ""
				}
				return "may create pods, and so run as any account in the namespace"
			}),
		ruleCheck("rbac-write-bindings", "Role may grant permissions", severityHigh,
			"Writing role bindings is writing the permission model itself.",
			"Remove write access to roles, rolebindings and their cluster-scoped forms.",
			func(one rule) string {
				return namesResource(one,
					[]string{"rolebindings", "clusterrolebindings", "roles", "clusterroles"}, writeVerbs)
			}),
		ruleCheck("rbac-write-webhooks", "Role may change admission", severityHigh,
			"Writing webhook configurations lets the holder intercept or wave through every request the API server handles.",
			"Remove write access to validatingwebhookconfigurations and mutatingwebhookconfigurations.",
			func(one rule) string {
				return namesResource(one,
					[]string{"validatingwebhookconfigurations", "mutatingwebhookconfigurations"}, writeVerbs)
			}),
		ruleCheck("rbac-node-proxy", "Role reaches the kubelet directly", severityHigh,
			"nodes/proxy talks to the kubelet API, which serves every pod on the node without the API server's checks.",
			"Remove nodes/proxy.",
			func(one rule) string {
				return namesResource(one, []string{"nodes/proxy"}, readVerbs)
			}),
		{
			id:         "cluster-admin-bound",
			title:      "cluster-admin granted",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			needs:      []target{clusterBindTarget},
			wrong:      "cluster-admin is every verb on every resource in every namespace, with no exceptions.",
			remedy:     "Bind a role that names what the subject actually needs.",
			find:       overCorpus(clusterAdminBound),
		},
		{
			id:         "bound-to-everyone",
			title:      "Permissions granted to everyone",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			needs:      []target{bindingTarget, clusterBindTarget},
			wrong:      "The subject is a group every request already belongs to, so this grants the permission to the whole cluster.",
			remedy:     "Bind the role to the specific service account or user that needs it.",
			find:       overCorpus(boundToEveryone),
		},
	}
}

func ruleCheck(id, title, severity, wrong, remedy string, judge func(rule) string) check {
	return check{
		id:         id,
		title:      title,
		category:   categorySecurity,
		severity:   severity,
		frameworks: []string{nsaCisa},
		needs:      []target{roleTarget, clusterRoleTarget},
		wrong:      wrong,
		remedy:     remedy,
		find:       overCorpus(overRoles(judge)),
	}
}

func overRoles(judge func(rule) string) func(scan) []found {
	return func(sc scan) []found {
		out := []found{}
		for _, kind := range roleKinds {
			for _, obj := range sc.held.of(rbacGroup, kind.resource) {
				detail := firstRuleDetail(obj, judge)
				if detail == "" {
					continue
				}
				out = append(out, corpusFinding(obj, kind.key, kind.kind, detail))
			}
		}
		return out
	}
}

func firstRuleDetail(obj *unstructured.Unstructured, judge func(rule) string) string {
	for _, one := range rulesOf(obj) {
		if detail := judge(one); detail != "" {
			return detail
		}
	}
	return ""
}

func rulesOf(obj *unstructured.Unstructured) []rule {
	out := []rule{}
	for _, raw := range unstr.Slice(obj, "rules") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, rule{
			verbs:     stringsAt(entry, "verbs"),
			resources: stringsAt(entry, "resources"),
			groups:    stringsAt(entry, "apiGroups"),
			names:     stringsAt(entry, "resourceNames"),
		})
	}
	return out
}

func stringsAt(entry map[string]any, field string) []string {
	listed, ok := entry[field].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(listed))
	for _, item := range listed {
		text, isString := item.(string)
		if !isString {
			continue
		}
		out = append(out, text)
	}
	return out
}

func covers(held []string, wanted string) bool {
	return slices.Contains(held, wanted) || slices.Contains(held, anything)
}

func overlaps(held, wanted []string) bool {
	for _, one := range held {
		if slices.Contains(wanted, one) {
			return true
		}
	}
	return false
}

func namesVerb(one rule, wanted []string, prefix string) string {
	for _, verb := range wanted {
		if slices.Contains(one.verbs, verb) {
			return prefix + verb
		}
	}
	return ""
}

func namesResource(one rule, wanted, verbs []string) string {
	if !overlaps(one.verbs, verbs) {
		return ""
	}
	for _, name := range wanted {
		if slices.Contains(one.resources, name) {
			return listed(one.verbs) + " on " + name
		}
	}
	return ""
}

func listed(names []string) string {
	if len(names) == 0 {
		return "nothing"
	}
	if len(names) > 3 {
		return strings.Join(names[:3], ", ") + " and more"
	}
	return strings.Join(names, ", ")
}

func roleRefName(obj *unstructured.Unstructured) string {
	return stringAt(specAt(obj, "roleRef"), "name")
}

func clusterAdminBound(sc scan) []found {
	out := []found{}
	for _, obj := range sc.held.of(rbacGroup, "clusterrolebindings") {
		if roleRefName(obj) != "cluster-admin" {
			continue
		}
		named := bindingSubjects(obj)
		if len(named) == 0 {
			continue
		}
		out = append(out, corpusFinding(obj, clusterBindTarget, "ClusterRoleBinding",
			"grants cluster-admin to "+listed(named)))
	}
	return out
}

func boundToEveryone(sc scan) []found {
	out := []found{}
	for _, kind := range bindingKinds {
		for _, obj := range sc.held.of(rbacGroup, kind.resource) {
			open := openBinding(obj)
			if open == "" {
				continue
			}
			out = append(out, corpusFinding(obj, kind.key, kind.kind,
				"binds "+roleRefName(obj)+" to "+open))
		}
	}
	return out
}

func openBinding(obj *unstructured.Unstructured) string {
	for _, name := range bindingSubjects(obj) {
		if slices.Contains(openSubjects, name) {
			return name
		}
	}
	return ""
}

func bindingSubjects(obj *unstructured.Unstructured) []string {
	out := []string{}
	for _, raw := range unstr.Slice(obj, "subjects") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name := stringAt(entry, "name"); name != "" {
			out = append(out, name)
		}
	}
	return out
}
