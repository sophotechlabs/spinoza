package checks

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	openCIDR       = "0.0.0.0/0"
	snippetMarker  = "-snippet"
	deleteReclaim  = "Delete"
	loadBalancer   = "LoadBalancer"
	nodePort       = "NodePort"
	unhealthyField = "unhealthyPodEvictionPolicy"
)

func objectChecks() []check {
	return []check{
		corpusCheck("service-load-balancer", "Service published by a load balancer",
			categorySecurity, severityMedium, []target{serviceTarget}, []string{nsaCisa},
			"A cloud load balancer puts the Service on a public address, and every controller that reconciles it can move that address.",
			"Publish it through an Ingress, or use ClusterIP and reach it from inside.",
			overObjects("", "services", "Service", serviceTarget, publishedByLoadBalancer)),
		corpusCheck("service-node-port", "Service published on every node",
			categorySecurity, severityMedium, []target{serviceTarget}, []string{nsaCisa},
			"A NodePort opens the same high port on every node in the cluster, whatever the firewall in front of them says.",
			"Use ClusterIP with an Ingress, unless something outside genuinely dials the node.",
			overObjects("", "services", "Service", serviceTarget, publishedOnNodePort)),
		corpusCheck("service-external-ips", "Service claims an address it was given",
			categorySecurity, severityHigh, []target{serviceTarget}, []string{nsaCisa},
			"externalIPs makes the node answer for an address nothing checked it owns, which is how traffic for somebody else is intercepted.",
			"Remove spec.externalIPs and publish the Service properly.",
			overObjects("", "services", "Service", serviceTarget, claimsExternalIPs)),
		corpusCheck("ingress-no-tls", "Ingress serves plaintext",
			categorySecurity, severityMedium, []target{ingressTarget}, []string{nsaCisa},
			"Everything the route carries crosses the network in the clear, credentials included.",
			"Add a spec.tls block naming the secret that holds the certificate.",
			overObjects(networkGroup, "ingresses", "Ingress", ingressTarget, servesPlaintext)),
		corpusCheck("ingress-wildcard-host", "Ingress claims a wildcard host",
			categorySecurity, severityMedium, []target{ingressTarget}, []string{nsaCisa},
			"A wildcard host answers for every name under the domain, including ones another Ingress meant to own.",
			"Name the hosts this route serves.",
			overObjects(networkGroup, "ingresses", "Ingress", ingressTarget, claimsWildcardHost)),
		corpusCheck("ingress-snippet-annotation", "Ingress injects controller configuration",
			categorySecurity, severityHigh, []target{ingressTarget}, []string{nsaCisa},
			"A snippet annotation is raw configuration handed to the ingress controller, which is a way to reconfigure the proxy from a namespace that only owns a route.",
			"Move what the snippet does into the controller's own configuration.",
			overObjects(networkGroup, "ingresses", "Ingress", ingressTarget, injectsSnippet)),
		corpusCheck("policy-allows-everything", "NetworkPolicy that allows everything",
			categorySecurity, severityMedium, []target{policyTarget}, []string{nsaCisa},
			"An empty selector with an empty rule matches every pod and permits every source, which is the same as having no policy while looking like one.",
			"Name the pods it covers and the sources they may accept.",
			overObjects(networkGroup, "networkpolicies", "NetworkPolicy", policyTarget, allowsEverything)),
		corpusCheck("policy-open-ip-block", "NetworkPolicy opens the whole internet",
			categorySecurity, severityMedium, []target{policyTarget}, []string{nsaCisa},
			"An ipBlock of 0.0.0.0/0 permits every address there is, which undoes the point of writing the policy.",
			"Narrow the CIDR to what actually needs to reach it.",
			overObjects(networkGroup, "networkpolicies", "NetworkPolicy", policyTarget, opensEveryAddress)),
		corpusCheck("pdb-no-policy", "Disruption budget that says nothing",
			categoryReliability, severityMedium, []target{budgetTarget}, nil,
			"With neither minAvailable nor maxUnavailable the budget has no effect at all.",
			"Set minAvailable, or maxUnavailable.",
			overObjects("policy", "poddisruptionbudgets", "PodDisruptionBudget", budgetTarget, budgetSaysNothing)),
		corpusCheck("pdb-blocks-all", "Disruption budget that forbids every eviction",
			categoryReliability, severityHigh, []target{budgetTarget}, nil,
			"maxUnavailable of zero means no pod may ever be evicted, so a drain waits for ever.",
			"Raise maxUnavailable above zero, or use minAvailable below the replica count.",
			overObjects("policy", "poddisruptionbudgets", "PodDisruptionBudget", budgetTarget, budgetForbidsEviction)),
		corpusCheck("pdb-unhealthy-eviction", "Disruption budget can be held by an unhealthy pod",
			categoryReliability, severityLow, []target{budgetTarget}, nil,
			"Without unhealthyPodEvictionPolicy a pod that is running but never ready still counts against the budget, so a drain stalls on a pod that is already broken.",
			"Set unhealthyPodEvictionPolicy to AlwaysAllow.",
			overObjects("policy", "poddisruptionbudgets", "PodDisruptionBudget", budgetTarget, budgetHeldByUnhealthy)),
		corpusCheck("claim-no-storage-class", "Claim names no StorageClass",
			categoryReliability, severityLow, []target{claimTarget}, nil,
			"It gets whichever class is marked default, which is not the same class on another cluster.",
			"Name the class the workload needs.",
			overObjects("", "persistentvolumeclaims", "PersistentVolumeClaim", claimTarget, claimNamesNoClass)),
		corpusCheck("storage-class-deletes-data", "StorageClass deletes the volume with the claim",
			categoryReliability, severityMedium, []target{storageTarget}, nil,
			"reclaimPolicy Delete destroys the underlying volume the moment the claim goes, so deleting a namespace deletes the data in it.",
			"Set reclaimPolicy to Retain on the classes that hold data you would want back.",
			overObjects("storage.k8s.io", "storageclasses", "StorageClass", storageTarget, classDeletesData)),
		corpusCheck("hpa-floor-of-one", "Autoscaler may scale down to one",
			categoryReliability, severityLow, []target{scalerTarget}, nil,
			"At one replica every rollout, drain and eviction is downtime, and the autoscaler will take it there whenever load allows.",
			"Raise minReplicas to at least two.",
			overObjects("autoscaling", "horizontalpodautoscalers", "HorizontalPodAutoscaler", scalerTarget, scalerFloorOfOne)),
		corpusCheck("hpa-no-metrics", "Autoscaler with nothing to scale on",
			categoryReliability, severityHigh, []target{scalerTarget}, nil,
			"With no metric the autoscaler holds the replica count where it is and reports it cannot compute a target.",
			"Add a metric, or drop the autoscaler and set the replica count yourself.",
			overObjects("autoscaling", "horizontalpodautoscalers", "HorizontalPodAutoscaler", scalerTarget, scalerHasNoMetrics)),
	}
}

func corpusCheck(
	id, title, category, severity string,
	needs []target,
	frameworks []string,
	wrong, remedy string,
	find func(scan) []found,
) check {
	return check{
		id:         id,
		title:      title,
		category:   category,
		severity:   severity,
		frameworks: frameworks,
		needs:      needs,
		wrong:      wrong,
		remedy:     remedy,
		find:       overCorpus(find),
	}
}

func overObjects(
	group, resource, kind string,
	key target,
	judge func(*unstructured.Unstructured) string,
) func(scan) []found {
	return func(sc scan) []found {
		out := []found{}
		for _, obj := range sc.held.of(group, resource) {
			detail := judge(obj)
			if detail == "" {
				continue
			}
			out = append(out, corpusFinding(obj, key, kind, detail))
		}
		return out
	}
}

func publishedByLoadBalancer(obj *unstructured.Unstructured) string {
	if stringAt(specAt(obj, specField), "type") != loadBalancer {
		return ""
	}
	return "spec.type is LoadBalancer"
}

func publishedOnNodePort(obj *unstructured.Unstructured) string {
	if stringAt(specAt(obj, specField), "type") != nodePort {
		return ""
	}
	return "spec.type is NodePort, so the port is open on every node"
}

func claimsExternalIPs(obj *unstructured.Unstructured) string {
	listed := stringsAt(specAt(obj, specField), "externalIPs")
	if len(listed) == 0 {
		return ""
	}
	return "spec.externalIPs claims " + strings.Join(listed, ", ")
}

func servesPlaintext(obj *unstructured.Unstructured) string {
	if listedItems(specAt(obj, specField), "tls") > 0 {
		return ""
	}
	hosts := ingressHosts(obj)
	if len(hosts) == 0 {
		return "no spec.tls, so the route is served over plain HTTP"
	}
	return "no spec.tls for " + listed(hosts)
}

func claimsWildcardHost(obj *unstructured.Unstructured) string {
	for _, host := range ingressHosts(obj) {
		if strings.HasPrefix(host, "*.") {
			return "answers for " + host
		}
	}
	return ""
}

func ingressHosts(obj *unstructured.Unstructured) []string {
	out := []string{}
	for _, raw := range unstr.Slice(obj, specField, "rules") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if host := stringAt(entry, "host"); host != "" {
			out = append(out, host)
		}
	}
	return out
}

func injectsSnippet(obj *unstructured.Unstructured) string {
	for key := range obj.GetAnnotations() {
		if strings.HasSuffix(key, snippetMarker) {
			return "the " + key + " annotation configures the controller directly"
		}
	}
	return ""
}

func allowsEverything(obj *unstructured.Unstructured) string {
	spec := specAt(obj, specField)
	selector, ok := spec["podSelector"].(map[string]any)
	if !ok || len(selector) > 0 {
		return ""
	}
	for _, field := range []string{"ingress", "egress"} {
		for _, raw := range unstr.Slice(obj, specField, field) {
			entry, isMap := raw.(map[string]any)
			if !isMap {
				continue
			}
			if len(entry) == 0 {
				return "an empty podSelector with an empty " + field + " rule permits everything"
			}
		}
	}
	return ""
}

func opensEveryAddress(obj *unstructured.Unstructured) string {
	for _, field := range []string{"ingress", "egress"} {
		for _, raw := range unstr.Slice(obj, specField, field) {
			entry, isMap := raw.(map[string]any)
			if !isMap {
				continue
			}
			if peerOpensEverything(entry) {
				return "an ipBlock of " + openCIDR + " on the " + field + " side"
			}
		}
	}
	return ""
}

func peerOpensEverything(entry map[string]any) bool {
	for _, side := range []string{"from", "to"} {
		listed, ok := entry[side].([]any)
		if !ok {
			continue
		}
		for _, raw := range listed {
			peer, isMap := raw.(map[string]any)
			if !isMap {
				continue
			}
			block, hasBlock := peer["ipBlock"].(map[string]any)
			if hasBlock && stringAt(block, "cidr") == openCIDR {
				return true
			}
		}
	}
	return false
}

func budgetSaysNothing(obj *unstructured.Unstructured) string {
	spec := specAt(obj, specField)
	_, hasFloor := numberAt(spec, "minAvailable")
	_, hasCeiling := numberAt(spec, "maxUnavailable")
	if hasFloor || hasCeiling {
		return ""
	}
	return "neither minAvailable nor maxUnavailable is set"
}

func budgetForbidsEviction(obj *unstructured.Unstructured) string {
	ceiling, ok := numberAt(specAt(obj, specField), "maxUnavailable")
	if !ok || ceiling != 0 {
		return ""
	}
	return "maxUnavailable is 0, so nothing may ever be evicted"
}

func budgetHeldByUnhealthy(obj *unstructured.Unstructured) string {
	if stringAt(specAt(obj, specField), unhealthyField) != "" {
		return ""
	}
	return unhealthyField + " is unset, so a pod that never becomes ready still holds the budget"
}

func claimNamesNoClass(obj *unstructured.Unstructured) string {
	if stringAt(specAt(obj, specField), "storageClassName") != "" {
		return ""
	}
	return "spec.storageClassName is unset, so it takes whichever class is default here"
}

func classDeletesData(obj *unstructured.Unstructured) string {
	policy, ok := obj.Object["reclaimPolicy"].(string)
	if !ok || policy != deleteReclaim {
		return ""
	}
	return "reclaimPolicy is Delete, so the volume goes when the claim does"
}

func scalerFloorOfOne(obj *unstructured.Unstructured) string {
	floor, ok := numberAt(specAt(obj, specField), "minReplicas")
	if !ok || floor != 1 {
		return ""
	}
	ceiling, hasCeiling := numberAt(specAt(obj, specField), "maxReplicas")
	if hasCeiling && ceiling == 1 {
		return ""
	}
	return "minReplicas is 1, so it may take the workload down to a single pod"
}

func scalerHasNoMetrics(obj *unstructured.Unstructured) string {
	spec := specAt(obj, specField)
	if listedItems(spec, "metrics") > 0 {
		return ""
	}
	if _, ok := numberAt(spec, "targetCPUUtilizationPercentage"); ok {
		return ""
	}
	return "spec.metrics is empty, so there is nothing to scale on"
}
