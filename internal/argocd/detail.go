package argocd

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/faults"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const IsApplication = "Application"

func Detail(app *unstructured.Unstructured) api.GitopsApp {
	return api.GitopsApp{
		Controller:  api.ControllerArgo,
		Terminating: app.GetDeletionTimestamp() != nil,
		Kind:        app.GetKind(),
		Name:        app.GetName(),
		Namespace:   app.GetNamespace(),
		Source:      sourceOfApp(app),
		State:       stateOfApp(app),
		Issues:      issuesOfApp(app),
		Resources:   resourcesOfApp(app),
		History:     historyOfApp(app),
		Operation:   operationOfApp(app),
	}
}

func sourceOfApp(app *unstructured.Unstructured) api.GitopsSource {
	return api.GitopsSource{
		Repo:        sourceOf(app, "repoURL"),
		Path:        pathOf(app),
		Target:      sourceOf(app, "targetRevision"),
		Destination: destinationOf(app),
		Project:     unstr.String(app, "spec", "project"),
		SyncMode:    syncModeOf(app),
		Policy:      policyOf(app),
	}
}

func syncModeOf(app *unstructured.Unstructured) string {
	_, found := unstr.Map(app, "spec", "syncPolicy", "automated")
	if !found {
		return api.SyncModeManual
	}
	if AutoSyncing(app) {
		return api.SyncModeAuto
	}
	return api.SyncModeSuspended
}

func policyOf(app *unstructured.Unstructured) string {
	automated, found := unstr.Map(app, "spec", "syncPolicy", "automated")
	if !found {
		return ""
	}
	on := []string{}
	if flag(automated, "prune") {
		on = append(on, "prune")
	}
	if flag(automated, "selfHeal") {
		on = append(on, "self-heal")
	}
	if len(on) == 0 {
		return "neither prune nor self-heal"
	}
	return strings.Join(on, ", ")
}

func flag(entry map[string]any, key string) bool {
	value, ok := entry[key].(bool)
	if !ok {
		return false
	}
	return value
}

func stateOfApp(app *unstructured.Unstructured) api.GitopsState {
	return api.GitopsState{
		Sync:      unstr.String(app, "status", "sync", "status"),
		Health:    unstr.String(app, "status", "health", "status"),
		Revision:  revisionOf(app),
		CreatedAt: app.GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05Z"),
		SyncedAt:  syncedAtOf(app),
		Message:   messageOf(app),
	}
}

func syncedAtOf(app *unstructured.Unstructured) string {
	finished := unstr.String(app, "status", "operationState", "finishedAt")
	if finished != "" {
		return finished
	}
	return unstr.String(app, "status", "reconciledAt")
}

func resourcesOfApp(app *unstructured.Unstructured) []api.GitopsResource {
	out := []api.GitopsResource{}
	for _, raw := range unstr.Slice(app, "status", "resources") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := unstr.At(entry, "kind")
		name := unstr.At(entry, "name")
		if kind == "" || name == "" {
			continue
		}
		out = append(out, api.GitopsResource{
			Group:     unstr.At(entry, "group"),
			Version:   unstr.At(entry, "version"),
			Kind:      kind,
			Name:      name,
			Namespace: unstr.At(entry, "namespace"),
			Sync:      unstr.At(entry, "status"),
			Health:    healthOf(entry),
			Message:   healthMessageOf(entry),
		})
	}
	return out
}

func healthOf(entry map[string]any) string {
	health, ok := entry["health"].(map[string]any)
	if !ok {
		return ""
	}
	return unstr.At(health, "status")
}

func healthMessageOf(entry map[string]any) string {
	health, ok := entry["health"].(map[string]any)
	if !ok {
		return ""
	}
	return unstr.At(health, "message")
}

func historyOfApp(app *unstructured.Unstructured) []api.GitopsDeployment {
	out := []api.GitopsDeployment{}
	for _, raw := range unstr.Slice(app, "status", "history") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, carried := entry["id"].(int64)
		if !carried {
			continue
		}
		who, automated := initiatorOf(entry)
		out = append(out, api.GitopsDeployment{
			ID:          id,
			Revision:    unstr.At(entry, "revision"),
			Source:      sourceLabelOf(entry),
			StartedAt:   unstr.At(entry, "deployStartedAt"),
			DeployedAt:  unstr.At(entry, "deployedAt"),
			InitiatedBy: who,
			Automated:   automated,
		})
	}
	return out
}

func initiatorOf(entry map[string]any) (string, bool) {
	by, ok := entry["initiatedBy"].(map[string]any)
	if !ok {
		return "", false
	}
	return unstr.At(by, "username"), flag(by, "automated")
}

func sourceLabelOf(entry map[string]any) string {
	source, ok := entry["source"].(map[string]any)
	if !ok {
		return ""
	}
	path := unstr.At(source, "path")
	if path == "" {
		path = unstr.At(source, "chart")
	}
	return path
}

func operationOfApp(app *unstructured.Unstructured) *api.GitopsOperation {
	state, found := unstr.Map(app, "status", "operationState")
	if !found {
		return nil
	}
	phase := unstr.At(state, "phase")
	if phase == "" {
		return nil
	}
	message := unstr.At(state, "message")
	who, _ := operationInitiatorOf(state)
	return &api.GitopsOperation{
		Phase:       phase,
		Running:     phase == runningPhase || phase == terminatingPhase,
		Message:     message,
		Cause:       faults.Cause(message),
		Revision:    operationRevisionOf(state),
		StartedAt:   unstr.At(state, "startedAt"),
		FinishedAt:  unstr.At(state, "finishedAt"),
		InitiatedBy: who,
	}
}

func operationInitiatorOf(state map[string]any) (string, bool) {
	operation, ok := state["operation"].(map[string]any)
	if !ok {
		return "", false
	}
	return initiatorOf(operation)
}

func operationRevisionOf(state map[string]any) string {
	operation, ok := state["operation"].(map[string]any)
	if !ok {
		return ""
	}
	sync, ok := operation["sync"].(map[string]any)
	if !ok {
		return ""
	}
	return unstr.At(sync, "revision")
}
