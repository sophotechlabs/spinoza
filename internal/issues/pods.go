package issues

import (
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorStartup = "pod-startup"

const (
	phaseFailed    = "Failed"
	phaseSucceeded = "Succeeded"
)

var containerStatusFields = []string{
	"initContainerStatuses",
	"containerStatuses",
	"ephemeralContainerStatuses",
}

var imageReasons = map[string]bool{
	"ImagePullBackOff":    true,
	"ErrImagePull":        true,
	"InvalidImageName":    true,
	"RegistryUnavailable": true,
	"ImageInspectError":   true,
}

var configReasons = map[string]bool{
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
	"StartError":                 true,
}

type symptom struct {
	severity int
	title    string
	detail   string
	action   string
}

func podFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, item := range snap.of("", kindPod) {
		found, ok := podSymptom(item.obj)
		if !ok {
			continue
		}
		moved := rolloutOf(snap, item)
		out = append(out, finding{
			detector:  detectorStartup,
			severity:  found.severity,
			title:     found.title,
			detail:    found.detail,
			action:    found.action,
			change:    moved.what,
			changedAt: moved.at,
			kind:      kindPod,
			subject:   item,
			since:     podSince(item.obj),
		})
	}
	return out
}

func podSymptom(pod *unstructured.Unstructured) (symptom, bool) {
	if pod.GetDeletionTimestamp() != nil {
		return symptom{}, false
	}
	phase := unstr.String(pod, "status", "phase")
	if phase == phaseSucceeded {
		return symptom{}, false
	}
	worstFault, found := worstContainerSymptom(pod)
	if found {
		return worstFault, true
	}
	if phase == phaseFailed {
		return failedPodSymptom(pod), true
	}
	return unschedulableSymptom(pod)
}

func worstContainerSymptom(pod *unstructured.Unstructured) (symptom, bool) {
	best := symptom{}
	seen := false
	for _, field := range containerStatusFields {
		for _, raw := range unstr.Slice(pod, "status", field) {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			found, hit := containerSymptomOf(entry)
			if !hit {
				continue
			}
			if !seen || found.severity > best.severity {
				best = found
				seen = true
			}
		}
	}
	return best, seen
}

func containerSymptomOf(entry map[string]any) (symptom, bool) {
	name := unstr.At(entry, "name")
	state, ok := entry["state"].(map[string]any)
	if !ok {
		return symptom{}, false
	}
	if waiting, isWaiting := state["waiting"].(map[string]any); isWaiting {
		return waitingSymptom(name, waiting, entry)
	}
	if terminated, isTerminated := state["terminated"].(map[string]any); isTerminated {
		return terminatedSymptom(name, terminated)
	}
	return symptom{}, false
}

func waitingSymptom(name string, waiting, entry map[string]any) (symptom, bool) {
	reason := unstr.At(waiting, "reason")
	message := unstr.At(waiting, "message")
	switch {
	case reason == "CrashLoopBackOff":
		return symptom{
			severity: severityFatal,
			title:    reason,
			detail:   crashDetail(name, entry),
			action:   "read the container's logs",
		}, true
	case imageReasons[reason]:
		return symptom{
			severity: severityFatal,
			title:    reason,
			detail:   containerLine(name, message, reason),
			action:   "check the image name, tag and pull secret",
		}, true
	case configReasons[reason]:
		return symptom{
			severity: severityFatal,
			title:    reason,
			detail:   containerLine(name, message, reason),
			action:   "check the referenced ConfigMap, Secret and command",
		}, true
	}
	return symptom{}, false
}

func terminatedSymptom(name string, terminated map[string]any) (symptom, bool) {
	reason := unstr.At(terminated, "reason")
	if reason != "OOMKilled" {
		return symptom{}, false
	}
	return symptom{
		severity: severityFatal,
		title:    reason,
		detail:   "container " + name + " was killed for exceeding its memory limit",
		action:   "raise the memory limit, or make it hold less",
	}, true
}

func crashDetail(name string, entry map[string]any) string {
	detail := "container " + name + " keeps exiting"
	last, ok := entry["lastState"].(map[string]any)
	if !ok {
		return detail
	}
	terminated, isTerminated := last["terminated"].(map[string]any)
	if !isTerminated {
		return detail
	}
	code, hasCode := terminated["exitCode"].(int64)
	if hasCode {
		detail += " with exit code " + strconv.FormatInt(code, 10)
	}
	reason := unstr.At(terminated, "reason")
	if reason != "" && reason != "Error" {
		detail += " (" + reason + ")"
	}
	restarts, hasRestarts := entry["restartCount"].(int64)
	if hasRestarts && restarts > 0 {
		detail += ", restarted " + strconv.FormatInt(restarts, 10) + " times"
	}
	return detail
}

func containerLine(name, message, reason string) string {
	if message != "" {
		return "container " + name + ": " + message
	}
	return "container " + name + ": " + reason
}

func failedPodSymptom(pod *unstructured.Unstructured) symptom {
	reason := unstr.String(pod, "status", "reason")
	message := unstr.String(pod, "status", "message")
	title := reason
	if title == "" {
		title = "PodFailed"
	}
	detail := message
	if detail == "" {
		detail = "the pod reached the Failed phase"
	}
	return symptom{
		severity: severityFatal,
		title:    title,
		detail:   detail,
		action:   "delete it, or fix what its controller creates",
	}
}

func unschedulableSymptom(pod *unstructured.Unstructured) (symptom, bool) {
	for _, raw := range unstr.Slice(pod, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != "PodScheduled" || unstr.At(entry, "status") == "True" {
			continue
		}
		message := unstr.At(entry, "message")
		if message == "" {
			message = "no node accepted the pod"
		}
		return symptom{
			severity: severityFatal,
			title:    "Unschedulable",
			detail:   message,
			action:   "free capacity, relax the affinity or tolerate the taint",
		}, true
	}
	return symptom{}, false
}

func podSince(pod *unstructured.Unstructured) time.Time {
	started := unstr.String(pod, "status", "startTime")
	at, err := time.Parse(time.RFC3339, started)
	if err == nil {
		return at
	}
	return pod.GetCreationTimestamp().Time
}
