package issues

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	detectorNode        = "node"
	detectorStorage     = "storage"
	detectorTerminating = "terminating"
	detectorExtension   = "extension"
	detectorRouting     = "routing"
	detectorExpiry      = "expiry"
)

const expiryWarning = 21 * 24 * time.Hour

const (
	kindNode      = "Node"
	kindNamespace = "Namespace"
	kindClaim     = "PersistentVolumeClaim"
	kindCRD       = "CustomResourceDefinition"
	kindService   = "Service"
	kindEndpoints = "Endpoints"
)

const apiExtensionsGroup = "apiextensions.k8s.io"

const defaultGraceSeconds = 30

var nodePressures = map[string]string{
	"MemoryPressure":     "the node is short of memory",
	"DiskPressure":       "the node is short of disk",
	"PIDPressure":        "the node is short of process ids",
	"NetworkUnavailable": "the node's network is not configured",
}

func clusterFindings(snap *snapshot, now time.Time) []finding {
	out := nodeFindings(snap)
	out = append(out, claimFindings(snap)...)
	out = append(out, terminatingFindings(snap, now)...)
	out = append(out, definitionFindings(snap)...)
	out = append(out, routingFindings(snap)...)
	out = append(out, expiryFindings(snap, now)...)
	return out
}

func expiryFindings(snap *snapshot, now time.Time) []finding {
	out := []finding{}
	for _, items := range snap.byKind {
		for _, item := range items {
			found, ok := expirySymptom(item.obj, now)
			if !ok {
				continue
			}
			out = append(out, finding{
				detector: detectorExpiry,
				severity: found.severity,
				title:    found.title,
				detail:   found.detail,
				action:   found.action,
				kind:     item.desc.Kind,
				subject:  item,
				since:    item.obj.GetCreationTimestamp().Time,
			})
		}
	}
	return out
}

func expirySymptom(obj *unstructured.Unstructured, now time.Time) (symptom, bool) {
	at, err := time.Parse(time.RFC3339, unstr.String(obj, "status", "notAfter"))
	if err != nil {
		return symptom{}, false
	}
	left := at.Sub(now)
	if left > expiryWarning {
		return symptom{}, false
	}
	if left <= 0 {
		return symptom{
			severity: severityFatal,
			title:    "Expired",
			detail:   "the certificate expired " + durationLabel(left.Abs().Round(time.Hour)) + " ago",
			action:   "renew it, and find out why the issuer did not",
		}, true
	}
	return symptom{
		severity: severityDegraded,
		title:    "ExpiringSoon",
		detail:   "the certificate is valid for another " + durationLabel(left.Round(time.Hour)),
		action:   "check the issuer is renewing it",
	}, true
}

func definitionFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, item := range snap.of(apiExtensionsGroup, kindCRD) {
		entry, ok := conditionOf(item.obj, "Established")
		if !ok || unstr.At(entry, "status") == conditionTrue {
			continue
		}
		out = append(out, finding{
			detector: detectorExtension,
			severity: severityFatal,
			title:    reasonOr(entry, "NotEstablished"),
			detail:   messageOr(entry, "the apiserver is not serving this kind, so nothing can create one"),
			action:   "check the CRD's schema and its conversion webhook",
			kind:     kindCRD,
			subject:  item,
			since:    conditionSince(item.obj, "Established"),
		})
	}
	return out
}

func routingFindings(snap *snapshot) []finding {
	filled := endpointNames(snap)
	out := []finding{}
	for _, item := range snap.of("", kindService) {
		if !selectsPods(item.obj) || filled[serviceKey(item.obj)] {
			continue
		}
		out = append(out, finding{
			detector: detectorRouting,
			severity: severityFatal,
			title:    "NoEndpoints",
			detail:   "the Service selects pods and none of them is ready, so it answers nothing",
			action:   "look at the pods its selector matches and why none is ready",
			kind:     kindService,
			subject:  item,
			since:    item.obj.GetCreationTimestamp().Time,
		})
	}
	return out
}

func selectsPods(service *unstructured.Unstructured) bool {
	selector, found, err := unstructured.NestedMap(service.Object, "spec", "selector")
	return found && err == nil && len(selector) > 0
}

func serviceKey(obj *unstructured.Unstructured) string {
	return obj.GetNamespace() + "/" + obj.GetName()
}

func endpointNames(snap *snapshot) map[string]bool {
	out := map[string]bool{}
	for _, item := range snap.of("", kindEndpoints) {
		if readyAddresses(item.obj) == 0 {
			continue
		}
		out[serviceKey(item.obj)] = true
	}
	return out
}

func readyAddresses(obj *unstructured.Unstructured) int {
	total := 0
	for _, raw := range unstr.Slice(obj, "subsets") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		listed, isList := entry["addresses"].([]any)
		if !isList {
			continue
		}
		total += len(listed)
	}
	return total
}

func nodeFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, item := range snap.of("", kindNode) {
		found, ok := nodeSymptom(item.obj)
		if !ok {
			continue
		}
		out = append(out, finding{
			detector: detectorNode,
			severity: found.severity,
			title:    found.title,
			detail:   found.detail,
			action:   found.action,
			kind:     kindNode,
			subject:  item,
			since:    conditionSince(item.obj, "Ready"),
		})
	}
	return out
}

func nodeSymptom(node *unstructured.Unstructured) (symptom, bool) {
	if notReady, ok := nodeNotReady(node); ok {
		return notReady, true
	}
	if pressed, ok := nodeUnderPressure(node); ok {
		return pressed, true
	}
	return nodeCordoned(node)
}

func nodeNotReady(node *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(node, "Ready")
	if !ok {
		return symptom{}, false
	}
	status := unstr.At(entry, "status")
	if status == conditionTrue {
		return symptom{}, false
	}
	severity := severityFatal
	detail := messageOr(entry, "the kubelet is not reporting the node as ready")
	if status != conditionFalse {
		severity = severityDegraded
		detail = messageOr(entry, "the kubelet has stopped reporting at all")
	}
	return symptom{
		severity: severity,
		title:    reasonOr(entry, "NodeNotReady"),
		detail:   detail,
		action:   "check the kubelet on this node",
	}, true
}

func nodeUnderPressure(node *unstructured.Unstructured) (symptom, bool) {
	for name, meaning := range nodePressures {
		entry, ok := conditionOf(node, name)
		if !ok || unstr.At(entry, "status") != conditionTrue {
			continue
		}
		return symptom{
			severity: severityDegraded,
			title:    name,
			detail:   messageOr(entry, meaning),
			action:   "free what the node is short of, or move work off it",
		}, true
	}
	return symptom{}, false
}

func nodeCordoned(node *unstructured.Unstructured) (symptom, bool) {
	if !unstr.Bool(node, "spec", "unschedulable") {
		return symptom{}, false
	}
	return symptom{
		severity: severityWarning,
		title:    "Cordoned",
		detail:   "the node takes no new pods until it is uncordoned",
		action:   "uncordon it when whatever it was cordoned for is done",
	}, true
}

func claimFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, item := range snap.of("", kindClaim) {
		found, ok := claimSymptom(item.obj)
		if !ok {
			continue
		}
		out = append(out, finding{
			detector: detectorStorage,
			severity: found.severity,
			title:    found.title,
			detail:   found.detail,
			action:   found.action,
			kind:     kindClaim,
			subject:  item,
			since:    item.obj.GetCreationTimestamp().Time,
		})
	}
	return out
}

func claimSymptom(claim *unstructured.Unstructured) (symptom, bool) {
	if claim.GetDeletionTimestamp() != nil {
		return symptom{}, false
	}
	switch unstr.String(claim, "status", "phase") {
	case "Pending":
		return symptom{
			severity: severityFatal,
			title:    "ClaimPending",
			detail:   "the volume has not been provisioned, so every pod that wants it stays Pending",
			action:   "check the StorageClass and its provisioner",
		}, true
	case "Lost":
		return symptom{
			severity: severityFatal,
			title:    "ClaimLost",
			detail:   "the volume behind this claim is gone",
			action:   "restore the volume, or recreate the claim and its data",
		}, true
	}
	return symptom{}, false
}

func terminatingFindings(snap *snapshot, now time.Time) []finding {
	out := []finding{}
	for _, kind := range []string{kindPod, kindNamespace} {
		for _, item := range snap.of("", kind) {
			found, ok := terminatingSymptom(item.obj, kind, now)
			if !ok {
				continue
			}
			out = append(out, finding{
				detector: detectorTerminating,
				severity: found.severity,
				title:    found.title,
				detail:   found.detail,
				action:   found.action,
				kind:     kind,
				subject:  item,
				since:    item.obj.GetDeletionTimestamp().Time,
			})
		}
	}
	return out
}

func terminatingSymptom(obj *unstructured.Unstructured, kind string, now time.Time) (symptom, bool) {
	at := obj.GetDeletionTimestamp()
	if at == nil {
		return symptom{}, false
	}
	if now.Sub(at.Time) < graceOf(obj, kind) {
		return symptom{}, false
	}
	return symptom{
		severity: severityWarning,
		title:    "StuckTerminating",
		detail:   stuckDetail(obj, now),
		action:   "find what still holds it: a finalizer, or a container that will not stop",
	}, true
}

func graceOf(obj *unstructured.Unstructured, kind string) time.Duration {
	if kind != kindPod {
		return defaultStuckGrace
	}
	seconds := unstr.Int(obj, "spec", "terminationGracePeriodSeconds")
	if seconds <= 0 {
		seconds = defaultGraceSeconds
	}
	return time.Duration(seconds)*time.Second + defaultStuckGrace
}

func stuckDetail(obj *unstructured.Unstructured, now time.Time) string {
	waited := now.Sub(obj.GetDeletionTimestamp().Time).Round(time.Minute)
	detail := "asked to go " + durationLabel(waited) + " ago and still here"
	held := finalizersOf(obj)
	if held == "" {
		return detail
	}
	return detail + ", held by " + held
}

func finalizersOf(obj *unstructured.Unstructured) string {
	held := obj.GetFinalizers()
	if len(held) == 0 {
		return ""
	}
	return held[0]
}
