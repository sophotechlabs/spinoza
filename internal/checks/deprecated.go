package checks

import (
	"slices"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Facts are what the cluster says about itself, as opposed to what its objects
// say. The audit cannot work any of this out from the objects it lists.
type Facts struct {
	ServerVersion  string
	ServedVersions []string
	Warnings       []string
}

type removal struct {
	groupVersion string
	kinds        string
	minor        int
}

// Removed group-versions, newest first. A cluster that still serves one of
// these breaks on the release named, so the check is only interesting while
// the server is below it. Kubernetes publishes this list per release; it needs
// a look each cycle.
var removals = []removal{
	{groupVersion: "flowcontrol.apiserver.k8s.io/v1beta3", kinds: "FlowSchema, PriorityLevelConfiguration", minor: 32},
	{groupVersion: "flowcontrol.apiserver.k8s.io/v1beta2", kinds: "FlowSchema, PriorityLevelConfiguration", minor: 29},
	{groupVersion: "autoscaling/v2beta2", kinds: "HorizontalPodAutoscaler", minor: 26},
	{groupVersion: "autoscaling/v2beta1", kinds: "HorizontalPodAutoscaler", minor: 25},
	{groupVersion: "batch/v1beta1", kinds: cronKind, minor: 25},
	{groupVersion: "policy/v1beta1", kinds: "PodDisruptionBudget, PodSecurityPolicy", minor: 25},
	{groupVersion: "discovery.k8s.io/v1beta1", kinds: "EndpointSlice", minor: 25},
	{groupVersion: "events.k8s.io/v1beta1", kinds: "Event", minor: 25},
	{groupVersion: "node.k8s.io/v1beta1", kinds: "RuntimeClass", minor: 25},
	{groupVersion: "coordination.k8s.io/v1beta1", kinds: "Lease", minor: 25},
	{groupVersion: "storage.k8s.io/v1beta1", kinds: "CSIDriver, CSINode, StorageClass, VolumeAttachment", minor: 25},
	{groupVersion: "networking.k8s.io/v1beta1", kinds: "Ingress, IngressClass", minor: 22},
	{groupVersion: "rbac.authorization.k8s.io/v1beta1", kinds: "Role, RoleBinding, ClusterRole, ClusterRoleBinding", minor: 22},
	{groupVersion: "apiextensions.k8s.io/v1beta1", kinds: "CustomResourceDefinition", minor: 22},
	{groupVersion: "admissionregistration.k8s.io/v1beta1", kinds: "ValidatingWebhookConfiguration, MutatingWebhookConfiguration", minor: 22},
	{groupVersion: "apiregistration.k8s.io/v1beta1", kinds: "APIService", minor: 22},
	{groupVersion: "certificates.k8s.io/v1beta1", kinds: "CertificateSigningRequest", minor: 22},
	{groupVersion: "authentication.k8s.io/v1beta1", kinds: "TokenReview", minor: 22},
}

func deprecationChecks() []check {
	return []check{
		{
			id:       "serves-a-removed-api",
			title:    "Cluster still serves an API a release removes",
			category: categoryReliability,
			severity: severityHigh,
			wrong:    "Anything still writing to this version stops working the moment the cluster is upgraded past the release that removes it.",
			remedy:   "Move whatever uses it to the replacement version before that upgrade.",
			find:     overCorpus(servesARemovedAPI),
		},
		{
			id:       "apiserver-says-deprecated",
			title:    "The apiserver warned about something the audit asked for",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Your own cluster answered a request with a deprecation warning. This is what it has said since Spinoza connected, so a window open for an hour has seen more than one just started.",
			remedy:   "Move to what the warning names.",
			find:     overCorpus(apiserverSaysDeprecated),
		},
	}
}

func minorOf(version string) int {
	trimmed := strings.TrimPrefix(version, "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return 0
	}
	minor, err := strconv.Atoi(strings.TrimSuffix(parts[1], "+"))
	if err != nil {
		return 0
	}
	return minor
}

func servesARemovedAPI(sc scan) []found {
	running := minorOf(sc.facts.ServerVersion)
	if running == 0 {
		return nil
	}
	out := []found{}
	for _, one := range removals {
		if one.minor <= running {
			continue
		}
		if !slices.Contains(sc.facts.ServedVersions, one.groupVersion) {
			continue
		}
		out = append(out, clusterFinding("serves "+one.groupVersion,
			one.groupVersion+" carries "+one.kinds+" and is removed in 1."+
				strconv.Itoa(one.minor)+"; this cluster is 1."+strconv.Itoa(running)))
	}
	return out
}

func apiserverSaysDeprecated(sc scan) []found {
	out := make([]found, 0, len(sc.facts.Warnings))
	for _, text := range sc.facts.Warnings {
		out = append(out, clusterFinding(warningSubject(text), text))
	}
	return out
}

// The warning text opens with what it is about, so the first few words make a
// readable subject without parsing a format the apiserver never promised.
func warningSubject(text string) string {
	head, _, found := strings.Cut(text, " is deprecated")
	if !found {
		return "the apiserver"
	}
	return head
}

func clusterFinding(name, detail string) found {
	return found{
		subject: Subject{
			Ref:  api.ObjectRef{Version: "v1", Resource: "clusters", Name: name},
			Kind: "Cluster",
		},
		detail: detail,
	}
}
