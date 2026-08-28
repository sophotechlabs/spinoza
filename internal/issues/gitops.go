package issues

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorGitOps = "gitops-sync"

const applicationsKind = "Application"

func gitopsFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, items := range snap.byKind {
		for _, item := range items {
			found, ok := gitopsFinding(item)
			if !ok {
				continue
			}
			out = append(out, found)
		}
	}
	return out
}

func gitopsFinding(item object) (finding, bool) {
	if isFluxGroup(item.desc.Group) {
		return fluxFinding(item)
	}
	if item.desc.Group == argoGroup && item.desc.Kind == applicationsKind {
		return argoFinding(item)
	}
	return finding{}, false
}

func fluxFinding(item object) (finding, bool) {
	obj := item.obj
	if unstr.Bool(obj, "spec", "suspend") {
		return finding{}, false
	}
	found, ok := fluxSymptom(obj)
	if !ok {
		return finding{}, false
	}
	return finding{
		detector:  detectorGitOps,
		severity:  found.severity,
		title:     found.title,
		detail:    found.detail,
		action:    found.action,
		change:    fluxRevision(obj),
		changedAt: transitionOf(obj),
		kind:      item.desc.Kind,
		subject:   item,
		since:     conditionSince(obj, "Ready"),
	}, true
}

func fluxSymptom(obj *unstructured.Unstructured) (symptom, bool) {
	if stalled, ok := conditionOf(obj, "Stalled"); ok && unstr.At(stalled, "status") == conditionTrue {
		return symptom{
			severity: severityFatal,
			title:    reasonOr(stalled, "Stalled"),
			detail:   messageOr(stalled, "flux stopped retrying this object"),
			action:   "fix what the message names, then reconcile it",
		}, true
	}
	ready, ok := conditionOf(obj, "Ready")
	if !ok || unstr.At(ready, "status") == conditionTrue {
		return symptom{}, false
	}
	severity := severityDegraded
	if unstr.At(ready, "status") == conditionFalse {
		severity = severityFatal
	}
	return symptom{
		severity: severity,
		title:    reasonOr(ready, "NotReady"),
		detail:   messageOr(ready, "the object has not become ready"),
		action:   "reconcile it, then read what comes back",
	}, true
}

func fluxRevision(obj *unstructured.Unstructured) string {
	for _, path := range [][]string{
		{"status", "lastAttemptedRevision"},
		{"status", "lastAppliedRevision"},
		{"status", "artifact", "revision"},
	} {
		found := unstr.String(obj, path...)
		if found != "" {
			return shortened(found)
		}
	}
	return ""
}

func argoFinding(item object) (finding, bool) {
	obj := item.obj
	found, ok := argoSymptom(obj)
	if !ok {
		return finding{}, false
	}
	at := argoChangedAt(obj)
	return finding{
		detector:  detectorGitOps,
		severity:  found.severity,
		title:     found.title,
		detail:    found.detail,
		action:    found.action,
		change:    shortened(unstr.String(obj, "status", "sync", "revision")),
		changedAt: at,
		kind:      item.desc.Kind,
		subject:   item,
		since:     at,
	}, true
}

func argoSymptom(obj *unstructured.Unstructured) (symptom, bool) {
	phase := unstr.String(obj, "status", "operationState", "phase")
	if phase == "Failed" || phase == "Error" {
		return symptom{
			severity: severityFatal,
			title:    "SyncFailed",
			detail:   messageOrText(unstr.String(obj, "status", "operationState", "message"), "the last sync did not finish"),
			action:   "fix the manifest, then sync again",
		}, true
	}
	health := unstr.String(obj, "status", "health", "status")
	if health == "Degraded" || health == "Missing" {
		return symptom{
			severity: severityFatal,
			title:    health,
			detail:   messageOrText(unstr.String(obj, "status", "health", "message"), "argo reports the application as "+health),
			action:   "open the application and look at the resource it marks unhealthy",
		}, true
	}
	if unstr.String(obj, "status", "sync", "status") == "OutOfSync" {
		return symptom{
			severity: severityWarning,
			title:    "OutOfSync",
			detail:   "the cluster does not match the revision argo tracks",
			action:   "sync it, or find out what changed it in the cluster",
		}, true
	}
	return symptom{}, false
}

func argoChangedAt(obj *unstructured.Unstructured) time.Time {
	at, err := time.Parse(time.RFC3339, unstr.String(obj, "status", "operationState", "finishedAt"))
	if err == nil {
		return at
	}
	return transitionOf(obj)
}

func messageOr(entry map[string]any, fallback string) string {
	return messageOrText(unstr.At(entry, "message"), fallback)
}

func messageOrText(message, fallback string) string {
	if message == "" {
		return fallback
	}
	return message
}
