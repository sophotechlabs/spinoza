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
)

const (
	kindNode      = "Node"
	kindNamespace = "Namespace"
	kindClaim     = "PersistentVolumeClaim"
)

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
	return out
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
	detail := "asked to go " + waited.String() + " ago and still here"
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
