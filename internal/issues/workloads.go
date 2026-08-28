package issues

import (
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorWorkload = "workload-progress"

const titleShortOfReplicas = "NotEnoughReplicas"

const (
	conditionTrue  = "True"
	conditionFalse = "False"
)

func WorkloadUnhealthy(obj *unstructured.Unstructured, kind string) bool {
	switch kind {
	case kindDeployment, kindStatefulSet, kindReplicaSet, kindReplicationController:
		return unstr.Int(obj, "status", "readyReplicas") < unstr.Int(obj, "spec", "replicas")
	case kindDaemonSet:
		return unstr.Int(obj, "status", "numberReady") < unstr.Int(obj, "status", "desiredNumberScheduled")
	case kindJob:
		return conditionIsTrue(obj, "Failed")
	}
	return false
}

func workloadFindings(snap *snapshot, now time.Time, limits Limits) []finding {
	out := []finding{}
	for _, group := range workloadKinds() {
		for _, item := range snap.of(group.group, group.kind) {
			found, ok := workloadFinding(snap, item, now, limits)
			if !ok {
				continue
			}
			out = append(out, found)
		}
	}
	return out
}

type kindRef struct {
	group string
	kind  string
}

func workloadKinds() []kindRef {
	return []kindRef{
		{group: appsGroup, kind: kindDeployment},
		{group: appsGroup, kind: kindReplicaSet},
		{group: appsGroup, kind: kindStatefulSet},
		{group: appsGroup, kind: kindDaemonSet},
		{group: batchGroup, kind: kindJob},
		{group: "", kind: kindReplicationController},
	}
}

func workloadFinding(snap *snapshot, item object, now time.Time, limits Limits) (finding, bool) {
	found, ok := workloadSymptom(item.obj, item.desc.Kind, now, limits)
	if !ok {
		return finding{}, false
	}
	moved := changeOf(snap, item)
	return finding{
		detector:  detectorWorkload,
		severity:  found.severity,
		title:     found.title,
		detail:    found.detail,
		action:    found.action,
		change:    moved.what,
		changedAt: moved.at,
		kind:      item.desc.Kind,
		subject:   item,
		since:     transitionOf(item.obj),
	}, true
}

func workloadSymptom(obj *unstructured.Unstructured, kind string, now time.Time, limits Limits) (symptom, bool) {
	if obj.GetDeletionTimestamp() != nil {
		return symptom{}, false
	}
	if blocked, ok := replicaFailure(obj); ok {
		return blocked, true
	}
	if kind == kindJob {
		return jobSymptom(obj)
	}
	if stalled, ok := progressStalled(obj); ok {
		return stalled, true
	}
	return shortOfReplicas(obj, kind, now, limits)
}

func replicaFailure(obj *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(obj, "ReplicaFailure")
	if !ok || unstr.At(entry, "status") != conditionTrue {
		return symptom{}, false
	}
	message := unstr.At(entry, "message")
	if message == "" {
		message = "the controller could not create its pods"
	}
	return symptom{
		severity: severityFatal,
		title:    admissionTitle(unstr.At(entry, "reason"), message),
		detail:   message,
		action:   "fix what the message names, then let it retry",
	}, true
}

func admissionTitle(reason, message string) string {
	switch {
	case strings.Contains(message, "exceeded quota"):
		return "BlockedByQuota"
	case strings.Contains(message, "admission webhook"):
		return "BlockedByWebhook"
	case strings.Contains(message, "violates PodSecurity"):
		return "BlockedByPodSecurity"
	case reason != "":
		return reason
	}
	return "ReplicaFailure"
}

func jobSymptom(obj *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(obj, "Failed")
	if !ok || unstr.At(entry, "status") != conditionTrue {
		return symptom{}, false
	}
	message := unstr.At(entry, "message")
	if message == "" {
		message = "the job gave up after its retries"
	}
	return symptom{
		severity: severityFatal,
		title:    "JobFailed",
		detail:   message,
		action:   "read the failed pod's logs, then delete the job to retry",
	}, true
}

func progressStalled(obj *unstructured.Unstructured) (symptom, bool) {
	entry, ok := conditionOf(obj, "Progressing")
	if !ok || unstr.At(entry, "status") != conditionFalse {
		return symptom{}, false
	}
	severity := severityDegraded
	if unstr.Int(obj, "status", "availableReplicas") == 0 {
		severity = severityFatal
	}
	message := unstr.At(entry, "message")
	if message == "" {
		message = "the rollout stopped making progress"
	}
	return symptom{
		severity: severity,
		title:    reasonOr(entry, "RolloutStalled"),
		detail:   message,
		action:   "look at the newest replica set's pods",
	}, true
}

func shortOfReplicas(obj *unstructured.Unstructured, kind string, now time.Time, limits Limits) (symptom, bool) {
	ready, want := replicaCounts(obj, kind)
	if ready >= want {
		return symptom{}, false
	}
	if now.Sub(transitionOf(obj)) < limits.ReadyGrace {
		return symptom{}, false
	}
	severity := severityDegraded
	if ready == 0 {
		severity = severityFatal
	}
	return symptom{
		severity: severity,
		title:    titleShortOfReplicas,
		detail: strconv.FormatInt(ready, 10) + " of " + strconv.FormatInt(want, 10) +
			" replicas ready for longer than " + limits.ReadyGrace.String(),
		action: "look at the pods it owns",
	}, true
}

func replicaCounts(obj *unstructured.Unstructured, kind string) (ready, want int64) {
	if kind == kindDaemonSet {
		return unstr.Int(obj, "status", "numberReady"), unstr.Int(obj, "status", "desiredNumberScheduled")
	}
	return unstr.Int(obj, "status", "readyReplicas"), unstr.Int(obj, "spec", "replicas")
}

func conditionOf(obj *unstructured.Unstructured, name string) (map[string]any, bool) {
	for _, raw := range unstr.Slice(obj, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != name {
			continue
		}
		return entry, true
	}
	return nil, false
}

func conditionSince(obj *unstructured.Unstructured, name string) time.Time {
	entry, ok := conditionOf(obj, name)
	if !ok {
		return obj.GetCreationTimestamp().Time
	}
	at, err := time.Parse(time.RFC3339, unstr.At(entry, "lastTransitionTime"))
	if err != nil {
		return obj.GetCreationTimestamp().Time
	}
	return at
}

func conditionIsTrue(obj *unstructured.Unstructured, name string) bool {
	entry, ok := conditionOf(obj, name)
	if !ok {
		return false
	}
	return unstr.At(entry, "status") == conditionTrue
}

func reasonOr(entry map[string]any, fallback string) string {
	reason := unstr.At(entry, "reason")
	if reason == "" {
		return fallback
	}
	return reason
}
