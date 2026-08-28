package issues

import (
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorAutoscaler = "hpa"

const autoscalerKind = "HorizontalPodAutoscaler"

func autoscalerFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, item := range snap.of(autoscalingGroup, autoscalerKind) {
		found, ok := autoscalerSymptom(item.obj)
		if !ok {
			continue
		}
		out = append(out, finding{
			detector: detectorAutoscaler,
			severity: found.severity,
			title:    found.title,
			detail:   found.detail,
			action:   found.action,
			kind:     item.desc.Kind,
			subject:  item,
			since:    conditionSince(item.obj, "ScalingActive"),
		})
	}
	return out
}

func autoscalerSymptom(obj *unstructured.Unstructured) (symptom, bool) {
	if blind, ok := metricsUnreadable(obj); ok {
		return blind, true
	}
	if frozen, ok := scalingDisabled(obj); ok {
		return frozen, true
	}
	if pinned, ok := replicasPinned(obj); ok {
		return pinned, true
	}
	return atMaximum(obj)
}

func metricsUnreadable(obj *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(obj, "ScalingActive")
	if !ok || unstr.At(entry, "status") != conditionFalse {
		return symptom{}, false
	}
	reason := unstr.At(entry, "reason")
	if reason == "ScalingDisabled" {
		return symptom{}, false
	}
	return symptom{
		severity: severityDegraded,
		title:    reasonOr(entry, "NoMetrics"),
		detail:   messageOr(entry, "the autoscaler cannot read the metric it scales on"),
		action:   "check metrics-server or the custom metrics adapter",
	}, true
}

func scalingDisabled(obj *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(obj, "AbleToScale")
	if !ok || unstr.At(entry, "status") != conditionFalse {
		return symptom{}, false
	}
	return symptom{
		severity: severityDegraded,
		title:    reasonOr(entry, "CannotScale"),
		detail:   messageOr(entry, "the autoscaler is not able to change the replica count"),
		action:   "check that the scale target exists and is scalable",
	}, true
}

func replicasPinned(obj *unstructured.Unstructured) (symptom, bool) {
	low := unstr.Int(obj, "spec", "minReplicas")
	high := unstr.Int(obj, "spec", "maxReplicas")
	if low == 0 || low != high {
		return symptom{}, false
	}
	return symptom{
		severity: severityWarning,
		title:    "Pinned",
		detail:   "minReplicas and maxReplicas are both " + strconv.FormatInt(low, 10) + ", so it cannot scale",
		action:   "widen the range, or drop the autoscaler",
	}, true
}

func atMaximum(obj *unstructured.Unstructured) (symptom, bool) {
	high := unstr.Int(obj, "spec", "maxReplicas")
	current := unstr.Int(obj, "status", "currentReplicas")
	if high == 0 || current < high {
		return symptom{}, false
	}
	if !conditionIsTrue(obj, "ScalingLimited") {
		return symptom{}, false
	}
	return symptom{
		severity: severityWarning,
		title:    "AtMaximum",
		detail:   "running at maxReplicas " + strconv.FormatInt(high, 10) + " and still asking for more",
		action:   "raise maxReplicas, or find out why each replica is loaded",
	}, true
}
