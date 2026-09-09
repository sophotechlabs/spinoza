package checks

import (
	"slices"
	"strings"

	"k8s.io/pod-security-admission/policy"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	controlCovered    = "covered"
	controlUncovered  = "uncovered"
	controlOutOfScope = "out of scope"
)

const (
	beyondNodeFiles = "the file and its permissions sit on a host's disk, " +
		"and the API server does not serve them"
	beyondProcessFlags = "these are flags on a control-plane process, " +
		"which a running cluster does not report through its API"
	beyondKubelet = "the kubelet reads this from its own configuration on the node, " +
		"which is not an object anything can list"
	beyondEtcd = "etcd runs beside the API server rather than behind it, " +
		"so nothing about it comes back from a cluster read"
	beyondPlugin = "whether the CNI enforces a NetworkPolicy is a property of the plugin, " +
		"not of any object the cluster holds"
	beyondJudgement = "this asks where secrets ought to live, which is a decision " +
		"rather than a state a cluster read can report"
)

type control struct {
	framework  string
	id         string
	title      string
	checks     []string
	outOfScope string
}

func (c control) state() string {
	if c.outOfScope != "" {
		return controlOutOfScope
	}
	if len(c.checks) == 0 {
		return controlUncovered
	}
	return controlCovered
}

func frameworkNames() []string {
	return []string{pssBaseline, pssRestricted, nsaCisa, cisBenchmark}
}

func catalog() []control {
	return append(cisControls(), pssControls()...)
}

func cisControl(id, title string, named ...string) control {
	return control{framework: cisBenchmark, id: id, title: title, checks: named}
}

func cisBeyondReach(id, title, reason string) control {
	return control{framework: cisBenchmark, id: id, title: title, outOfScope: reason}
}

func cisControls() []control {
	out := cisBeyondSection5()
	return append(out, cisSection5()...)
}

func cisBeyondSection5() []control {
	return []control{
		cisBeyondReach("1.1", "Control Plane Node Configuration Files", beyondNodeFiles),
		cisBeyondReach("1.2", "API Server", beyondProcessFlags),
		cisBeyondReach("1.3", "Controller Manager", beyondProcessFlags),
		cisBeyondReach("1.4", "Scheduler", beyondProcessFlags),
		cisBeyondReach("2", "Etcd Node Configuration", beyondEtcd),
		cisBeyondReach("3.1", "Authentication and Authorization", beyondProcessFlags),
		cisBeyondReach("3.2", "Logging", beyondProcessFlags),
		cisBeyondReach("4.1", "Worker Node Configuration Files", beyondNodeFiles),
		cisBeyondReach("4.2", "Kubelet", beyondKubelet),
		cisBeyondReach("4.3", "kube-proxy", beyondKubelet),
	}
}

func cisSection5() []control {
	out := cisAccessControls()
	out = append(out, cisAdmissionControls()...)
	return append(out, cisPolicyControls()...)
}

func cisAccessControls() []control {
	return []control{
		cisControl("5.1.1", "Ensure that the cluster-admin role is only used where required",
			"cluster-admin-bound"),
		cisControl("5.1.2", "Minimize access to secrets",
			"rbac-read-secrets"),
		cisControl("5.1.3", "Minimize wildcard use in Roles and ClusterRoles",
			"rbac-wildcard-verbs", "rbac-wildcard-resources", "rbac-wildcard-api-groups"),
		cisControl("5.1.4", "Minimize access to create pods",
			"rbac-create-pods"),
		cisControl("5.1.5", "Ensure that default service accounts are not actively used",
			"default-service-account"),
		cisControl("5.1.6", "Ensure that Service Account Tokens are only mounted where necessary",
			"automount-token"),
		cisControl("5.1.7", "Avoid use of system:masters group",
			"bound-to-everyone"),
		cisControl("5.1.8", "Limit use of the Bind, Impersonate and Escalate permissions in the Kubernetes cluster",
			"rbac-escalate-or-bind", "rbac-impersonate"),
		cisControl("5.1.9", "Minimize access to create persistent volumes"),
		cisControl("5.1.10", "Minimize access to the proxy sub-resource of nodes",
			"rbac-node-proxy"),
		cisControl("5.1.11", "Minimize access to the approval sub-resource of certificatesigningrequests objects"),
		cisControl("5.1.12", "Minimize access to webhook configuration objects",
			"rbac-write-webhooks"),
		cisControl("5.1.13", "Minimize access to the service account token creation"),
	}
}

func cisAdmissionControls() []control {
	return []control{
		cisControl("5.2.1", "Ensure that the cluster has at least one active policy control mechanism in place"),
		cisControl("5.2.2", "Minimize the admission of privileged containers",
			"privileged-containers"),
		cisControl("5.2.3", "Minimize the admission of containers wishing to share the host process ID namespace",
			"host-namespaces"),
		cisControl("5.2.4", "Minimize the admission of containers wishing to share the host IPC namespace",
			"host-namespaces"),
		cisControl("5.2.5", "Minimize the admission of containers wishing to share the host network namespace",
			"host-namespaces"),
		cisControl("5.2.6", "Minimize the admission of containers with allowPrivilegeEscalation",
			"privilege-escalation"),
		cisControl("5.2.7", "Minimize the admission of root containers",
			"run-as-root", "run-as-user-zero"),
		cisControl("5.2.8", "Minimize the admission of containers with the NET_RAW capability",
			"net-raw-kept"),
		cisControl("5.2.9", "Minimize the admission of containers with capabilities assigned",
			"dangerous-capabilities", "capabilities-not-dropped"),
		cisControl("5.2.10", "Minimize the admission of Windows HostProcess containers",
			"host-process"),
		cisControl("5.2.11", "Minimize the admission of HostPath volumes",
			"host-path-volume", "sensitive-host-path", "writable-host-mount"),
		cisControl("5.2.12", "Minimize the admission of containers which use HostPorts",
			"host-ports"),
	}
}

func cisPolicyControls() []control {
	return []control{
		cisBeyondReach("5.3.1", "Ensure that the CNI in use supports NetworkPolicies", beyondPlugin),
		cisControl("5.3.2", "Ensure that all Namespaces have NetworkPolicies defined",
			"no-network-policy", "policy-allows-everything"),
		cisControl("5.4.1", "Prefer using Secrets as files over Secrets as environment variables",
			"env-from-secret-wholesale", "secret-in-env-literal"),
		cisBeyondReach("5.4.2", "Consider external secret storage", beyondJudgement),
		cisBeyondReach("5.5.1", "Configure Image Provenance using ImagePolicyWebhook admission controller",
			beyondProcessFlags),
		cisControl("5.6.1", "Create administrative boundaries between resources using namespaces",
			"default-namespace"),
		cisControl("5.6.2", "Ensure that the seccomp profile is set to docker/default in your Pod definitions",
			"seccomp-unset", "seccomp-unconfined"),
		cisControl("5.6.3", "Apply SecurityContext to your Pods and Containers",
			"run-as-root", "privilege-escalation", "writable-root-filesystem",
			"capabilities-not-dropped", "root-group"),
		cisControl("5.6.4", "The default namespace should not be used",
			"default-namespace"),
	}
}

func pssControls() []control {
	answered := map[policy.CheckID]check{}
	for _, entry := range registry() {
		if entry.upstream == "" {
			continue
		}
		answered[entry.upstream] = entry
	}
	shipped := policy.DefaultChecks()
	out := make([]control, 0, len(shipped))
	for _, one := range shipped {
		entry := control{framework: pssLabelOf(one.Level), id: string(one.ID), title: string(one.ID)}
		if rule, known := answered[one.ID]; known {
			entry.title, entry.checks = rule.title, []string{rule.id}
		}
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b control) int {
		return strings.Compare(a.id, b.id)
	})
	return out
}

type checkCount struct {
	found   int
	muted   int
	objects []int
	ran     bool
}

func countsOf(report api.CheckReport) map[string]checkCount {
	out := map[string]checkCount{}
	for _, group := range report.Groups {
		one := checkCount{found: group.Total, muted: group.Muted, ran: group.Skipped == ""}
		for _, finding := range group.Findings {
			one.objects = append(one.objects, finding.Ref)
		}
		out[group.ID] = one
	}
	return out
}

func (c control) posture(counted map[string]checkCount) (api.FrameworkControl, bool) {
	out := api.FrameworkControl{
		Framework: c.framework,
		Control:   c.id,
		Title:     c.title,
		Scope:     c.state(),
		Checks:    c.checks,
		Covered:   c.state() == controlCovered,
	}
	if c.outOfScope != "" {
		out.Reason = c.outOfScope
		return out, true
	}
	whole := true
	objects := map[int]bool{}
	for _, id := range c.checks {
		one, reported := counted[id]
		if !reported || !one.ran {
			whole = false
			continue
		}
		out.Failing += one.found
		out.Muted += one.muted
		for _, ref := range one.objects {
			objects[ref] = true
		}
	}
	out.Objects = len(objects)
	return out, whole
}

func Posture(report api.CheckReport, framework string) api.FrameworkPosture {
	known := frameworkNames()
	out := api.FrameworkPosture{Frameworks: known, Controls: []api.FrameworkControl{}}
	if framework != "" && !slices.Contains(known, framework) {
		out.Reason = "no framework goes by that name: " + framework
		return out
	}
	counted := countsOf(report)
	held := catalog()
	partial := []string{}
	empty := []string{}
	for _, name := range known {
		if framework != "" && name != framework {
			continue
		}
		before := len(out.Controls)
		for _, one := range held {
			if one.framework != name {
				continue
			}
			entry, whole := one.posture(counted)
			out.Controls = append(out.Controls, entry)
			if !whole {
				partial = append(partial, one.id)
			}
		}
		if len(out.Controls) == before {
			empty = append(empty, name)
		}
	}
	out.Reason = joined(withoutControls(empty), notEveryCheckRan(partial))
	return out
}

func withoutControls(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "no numbered controls are cataloged for " + strings.Join(names, ", ") +
		", which spinoza carries as a label on individual checks"
}

func notEveryCheckRan(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return "not every check behind these controls ran, so their counts are partial: " +
		strings.Join(ids, ", ")
}
