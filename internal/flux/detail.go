package flux

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/faults"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	kustomizations = "kustomizations"
	helmReleases   = "helmreleases"
	readyTrue      = "True"
	readyFalse     = "False"
)

func IsSource(desc api.ResourceDescriptor) bool {
	if desc.Group != "source.toolkit.fluxcd.io" {
		return false
	}
	return sourceResources[desc.Resource]
}

func Applies(desc api.ResourceDescriptor) bool {
	if desc.Group == "kustomize.toolkit.fluxcd.io" && desc.Resource == kustomizations {
		return true
	}
	return desc.Group == helmGroup && desc.Resource == helmReleases
}

func Detail(obj *unstructured.Unstructured, desc api.ResourceDescriptor) api.GitopsApp {
	return api.GitopsApp{
		Controller:  api.ControllerFlux,
		Terminating: obj.GetDeletionTimestamp() != nil,
		Kind:        desc.Kind,
		Name:        obj.GetName(),
		Namespace:   obj.GetNamespace(),
		Source:      sourceOfApplier(obj, desc),
		State:       stateOfApplier(obj, desc),
		Issues:      issuesOfApplier(obj),
		Resources:   resourcesOfApplier(obj),
		History:     historyOfApplier(obj),
		Operation:   operationOfApplier(obj),
	}
}

func sourceOfApplier(obj *unstructured.Unstructured, desc api.ResourceDescriptor) api.GitopsSource {
	return api.GitopsSource{
		Repo:        sourceOf(obj),
		Path:        pathOfApplier(obj, desc),
		Destination: destinationOfApplier(obj),
		SyncMode:    syncModeOfApplier(obj),
		Policy:      policyOfApplier(obj),
	}
}

func pathOfApplier(obj *unstructured.Unstructured, desc api.ResourceDescriptor) string {
	if desc.Resource == kustomizations {
		return unstr.String(obj, "spec", "path")
	}
	chart := unstr.String(obj, "spec", "chart", "spec", "chart")
	if chart != "" {
		return chart
	}
	return unstr.String(obj, "spec", "chartRef", "name")
}

func destinationOfApplier(obj *unstructured.Unstructured) string {
	target := unstr.String(obj, "spec", "targetNamespace")
	if target != "" {
		return target
	}
	return obj.GetNamespace()
}

func syncModeOfApplier(obj *unstructured.Unstructured) string {
	if unstr.Bool(obj, "spec", "suspend") {
		return api.SyncModeSuspended
	}
	return api.SyncModeAuto
}

func policyOfApplier(obj *unstructured.Unstructured) string {
	on := []string{}
	if unstr.Bool(obj, "spec", "prune") {
		on = append(on, "prune")
	}
	if unstr.Bool(obj, "spec", "force") {
		on = append(on, "force")
	}
	if len(on) == 0 {
		return ""
	}
	return strings.Join(on, ", ")
}

func stateOfApplier(obj *unstructured.Unstructured, desc api.ResourceDescriptor) api.GitopsState {
	ready, message := unstr.Ready(obj)
	return api.GitopsState{
		Sync:      syncWordOf(ready),
		Health:    healthWordOf(ready),
		Revision:  revisionOf(obj, desc),
		CreatedAt: obj.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
		SyncedAt:  unstr.String(obj, "status", "lastHandledReconcileAt"),
		Message:   message,
	}
}

func syncWordOf(ready string) string {
	if ready == readyTrue {
		return "Synced"
	}
	if ready == readyFalse {
		return "OutOfSync"
	}
	return "Unknown"
}

func healthWordOf(ready string) string {
	if ready == readyTrue {
		return "Healthy"
	}
	if ready == readyFalse {
		return "Degraded"
	}
	return "Progressing"
}

func resourcesOfApplier(obj *unstructured.Unstructured) []api.GitopsResource {
	out := []api.GitopsResource{}
	for _, raw := range unstr.Slice(obj, "status", "inventory", "entries") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		namespace, name, group, kind := splitInventoryID(unstr.At(entry, "id"))
		if kind == "" || name == "" {
			continue
		}
		out = append(out, api.GitopsResource{
			Group:     group,
			Version:   unstr.At(entry, "v"),
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		})
	}
	return out
}

func splitInventoryID(id string) (namespace, name, group, kind string) {
	parts := strings.Split(id, "_")
	if len(parts) != 4 {
		return "", "", "", ""
	}
	group = parts[2]
	if group == "''" {
		group = ""
	}
	return parts[0], parts[1], group, parts[3]
}

func historyOfApplier(obj *unstructured.Unstructured) []api.GitopsDeployment {
	out := []api.GitopsDeployment{}
	for _, raw := range unstr.Slice(obj, "status", "history") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		version, carried := entry["version"].(int64)
		if !carried {
			continue
		}
		out = append(out, api.GitopsDeployment{
			ID:         version,
			Revision:   unstr.At(entry, "chartVersion"),
			Source:     unstr.At(entry, "chartName"),
			StartedAt:  unstr.At(entry, "firstDeployed"),
			DeployedAt: unstr.At(entry, "lastDeployed"),
		})
	}
	return out
}

func operationOfApplier(obj *unstructured.Unstructured) *api.GitopsOperation {
	if reconciling(obj) {
		return &api.GitopsOperation{
			Phase:     "Running",
			Running:   true,
			Message:   conditionMessage(obj, "Reconciling"),
			Revision:  unstr.String(obj, "status", "lastAttemptedRevision"),
			StartedAt: unstr.String(obj, "status", "lastHandledReconcileAt"),
		}
	}
	ready, message := unstr.Ready(obj)
	if ready == "" {
		return nil
	}
	phase := "Succeeded"
	if ready == readyFalse {
		phase = "Failed"
	}
	return &api.GitopsOperation{
		Phase:      phase,
		Message:    message,
		Cause:      faults.Cause(message),
		Revision:   revisionAttempted(obj),
		FinishedAt: conditionUpdated(obj, "Ready"),
	}
}

func revisionAttempted(obj *unstructured.Unstructured) string {
	attempted := unstr.String(obj, "status", "lastAttemptedRevision")
	if attempted != "" {
		return attempted
	}
	return unstr.String(obj, "status", "lastAppliedRevision")
}

func reconciling(obj *unstructured.Unstructured) bool {
	return conditionStatus(obj, "Reconciling") == readyTrue
}

func conditionStatus(obj *unstructured.Unstructured, kind string) string {
	entry, found := conditionOf(obj, kind)
	if !found {
		return ""
	}
	return unstr.At(entry, "status")
}

func conditionMessage(obj *unstructured.Unstructured, kind string) string {
	entry, found := conditionOf(obj, kind)
	if !found {
		return ""
	}
	return unstr.At(entry, "message")
}

func conditionUpdated(obj *unstructured.Unstructured, kind string) string {
	entry, found := conditionOf(obj, kind)
	if !found {
		return ""
	}
	return unstr.At(entry, "lastTransitionTime")
}

func conditionOf(obj *unstructured.Unstructured, kind string) (map[string]any, bool) {
	for _, raw := range unstr.Slice(obj, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != kind {
			continue
		}
		return entry, true
	}
	return nil, false
}

func issuesOfApplier(obj *unstructured.Unstructured) []api.GitopsIssue {
	out := []api.GitopsIssue{}
	if obj.GetDeletionTimestamp() != nil {
		out = append(out, api.GitopsIssue{
			Severity: api.SeverityWarning,
			Title:    "This object is being deleted",
			Detail:   heldByFinalizers(obj.GetFinalizers()),
		})
	}
	if unstr.Bool(obj, "spec", "suspend") {
		out = append(out, api.GitopsIssue{
			Severity: api.SeverityWarning,
			Title:    "Nothing will reconcile this",
			Detail:   "it is suspended; nothing changes until someone resumes it",
			Subject:  "drift",
		})
	}
	out = append(out, failingConditions(obj)...)
	if len(out) == 0 {
		return nil
	}
	return out
}

func heldByFinalizers(finalizers []string) string {
	if len(finalizers) == 0 {
		return "no finalizers left"
	}
	return "held by " + strings.Join(finalizers, ", ")
}

func failingConditions(obj *unstructured.Unstructured) []api.GitopsIssue {
	out := []api.GitopsIssue{}
	for _, raw := range unstr.Slice(obj, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := unstr.At(entry, "type")
		if !failing(kind, unstr.At(entry, "status")) {
			continue
		}
		message := unstr.At(entry, "message")
		out = append(out, api.GitopsIssue{
			Severity: api.SeverityFatal,
			Title:    unstr.At(entry, "reason"),
			Detail:   withCause(message),
			Subject:  kind,
		})
	}
	return out
}

func failing(kind, status string) bool {
	if kind == "Ready" || kind == "Healthy" {
		return status == readyFalse
	}
	if kind == "Stalled" {
		return status == readyTrue
	}
	return false
}

func withCause(message string) string {
	cause := faults.Cause(message)
	if cause == "" {
		return message
	}
	return cause + " — " + message
}
