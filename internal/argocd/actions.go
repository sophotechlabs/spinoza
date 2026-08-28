package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	refreshAnnotation = "argocd.argoproj.io/refresh"
	normalRefresh     = "normal"
	hardRefresh       = "hard"
	fieldManager      = "spinoza"
)

const (
	runningPhase     = "Running"
	terminatingPhase = "Terminating"
)

var ErrRefused = errors.New("argo cd refused")

type refusal struct {
	reason string
}

func (r refusal) Error() string {
	return r.reason
}

func (r refusal) Unwrap() error {
	return ErrRefused
}

func refuse(format string, args ...any) error {
	return refusal{reason: fmt.Sprintf(format, args...)}
}

type Action string

const (
	Sync        Action = "sync"
	Refresh     Action = "refresh"
	HardRefresh Action = "hard-refresh"
	Terminate   Action = "terminate"
	Suspend     Action = "suspend"
	Resume      Action = "resume"
	Rollback    Action = "rollback"
)

type Resource struct {
	Group     string
	Kind      string
	Name      string
	Namespace string
}

type Request struct {
	Action     Action
	Prune      bool
	DryRun     bool
	Force      bool
	Replace    bool
	ServerSide bool
	ApplyOnly  bool
	Revision   int64
	Resources  []Resource
}

func IsArgoGroup(group string) bool {
	return group == Group
}

func Do(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef, req Request) (api.ArgoActionResult, error) {
	result := api.ArgoActionResult{Action: string(req.Action)}
	if !IsArgoGroup(ref.Group) {
		return result, fmt.Errorf("%q is not an argo cd resource group", ref.Group)
	}
	if ref.Resource != applications {
		return result, fmt.Errorf("only applications can be operated on, not %q", ref.Resource)
	}
	target := targetFor(dyn, ref)
	patch, err := patchFor(ctx, target, ref, req)
	if err != nil {
		return result, err
	}
	patched, patchErr := target.Patch(ctx, ref.Name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
	if patchErr != nil {
		return api.ArgoActionResult{Action: string(req.Action)}, patchErr
	}
	return landed(result, patched, req.Action)
}

func landed(result api.ArgoActionResult, patched *unstructured.Unstructured, action Action) (api.ArgoActionResult, error) {
	if action == Suspend || action == Resume {
		return automationLanded(result, patched, action)
	}
	if action == Terminate {
		return terminationLanded(result, patched)
	}
	return result, nil
}

func automationLanded(result api.ArgoActionResult, patched *unstructured.Unstructured, action Action) (api.ArgoActionResult, error) {
	if AutoSyncing(patched) != (action == Resume) {
		return result, refuse("this argo cd ignored automated.enabled, so auto-sync is unchanged; pausing it needs argo cd 2.14 or newer")
	}
	return result, nil
}

func terminationLanded(result api.ArgoActionResult, patched *unstructured.Unstructured) (api.ArgoActionResult, error) {
	phase := unstr.String(patched, "status", "operationState", "phase")
	if phase == runningPhase {
		return result, refuse("this argo cd kept the operation running, so the application crd takes no status patch; terminate it from the argo cd api instead")
	}
	return result, nil
}

func patchFor(ctx context.Context, target dynamic.ResourceInterface, ref api.ObjectRef, req Request) ([]byte, error) {
	switch req.Action {
	case Sync:
		return json.Marshal(operationOf(syncOf(req)))
	case Refresh:
		return refreshPatch(normalRefresh)
	case HardRefresh:
		return refreshPatch(hardRefresh)
	case Suspend:
		return automatedPatch(ctx, target, ref, false)
	case Resume:
		return automatedPatch(ctx, target, ref, true)
	case Terminate:
		return terminatePatch(ctx, target, ref)
	case Rollback:
		return rollbackPatch(ctx, target, ref, req)
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
}

func refreshPatch(depth string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				refreshAnnotation: depth,
			},
		},
	})
}

func operationOf(sync map[string]any) map[string]any {
	return map[string]any{
		"operation": map[string]any{
			"initiatedBy": map[string]any{"username": fieldManager},
			"info":        []any{map[string]any{"name": "Reason", "value": "requested from spinoza"}},
			"sync":        sync,
		},
	}
}

func syncOf(req Request) map[string]any {
	sync := map[string]any{}
	if req.Prune {
		sync["prune"] = true
	}
	if req.DryRun {
		sync["dryRun"] = true
	}
	options := optionsOf(req)
	if len(options) > 0 {
		sync["syncOptions"] = options
	}
	strategy := strategyOf(req)
	if strategy != nil {
		sync["syncStrategy"] = strategy
	}
	if len(req.Resources) > 0 {
		sync["resources"] = resourcesOf(req.Resources)
	}
	return sync
}

func optionsOf(req Request) []any {
	out := []any{}
	if req.Replace {
		out = append(out, "Replace=true")
	}
	if req.ServerSide {
		out = append(out, "ServerSideApply=true")
	}
	return out
}

func strategyOf(req Request) map[string]any {
	if req.ApplyOnly {
		return map[string]any{"apply": map[string]any{"force": req.Force}}
	}
	if req.Force {
		return map[string]any{"hook": map[string]any{"force": true}}
	}
	return nil
}

func resourcesOf(resources []Resource) []any {
	out := make([]any, 0, len(resources))
	for _, one := range resources {
		entry := map[string]any{"kind": one.Kind, "name": one.Name}
		if one.Group != "" {
			entry["group"] = one.Group
		}
		if one.Namespace != "" {
			entry["namespace"] = one.Namespace
		}
		out = append(out, entry)
	}
	return out
}

func automatedPatch(ctx context.Context, target dynamic.ResourceInterface, ref api.ObjectRef, on bool) ([]byte, error) {
	app, err := target.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if AutoSyncing(app) == on {
		return nil, alreadyThere(ref.Name, on)
	}
	return json.Marshal(map[string]any{
		"spec": map[string]any{
			"syncPolicy": map[string]any{
				"automated": map[string]any{"enabled": on},
			},
		},
	})
}

func alreadyThere(name string, on bool) error {
	if on {
		return refuse("%s already syncs itself", name)
	}
	return refuse("%s does not sync itself", name)
}

func AutoSyncing(app *unstructured.Unstructured) bool {
	automated, found := unstr.Map(app, "spec", "syncPolicy", "automated")
	if !found {
		return false
	}
	enabled, carried := automated["enabled"].(bool)
	if !carried {
		return true
	}
	return enabled
}

func terminatePatch(ctx context.Context, target dynamic.ResourceInterface, ref api.ObjectRef) ([]byte, error) {
	app, err := target.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	phase := unstr.String(app, "status", "operationState", "phase")
	if phase != runningPhase {
		return nil, notRunning(ref.Name, phase)
	}
	return json.Marshal(map[string]any{
		"status": map[string]any{
			"operationState": map[string]any{"phase": terminatingPhase},
		},
	})
}

func notRunning(name, phase string) error {
	if phase == "" {
		return refuse("%s has no operation to terminate", name)
	}
	return refuse("%s has no operation running; the last one is %s", name, phase)
}

func rollbackPatch(ctx context.Context, target dynamic.ResourceInterface, ref api.ObjectRef, req Request) ([]byte, error) {
	app, err := target.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if AutoSyncing(app) {
		return nil, refuse("%s syncs itself; suspend auto-sync before rolling back", ref.Name)
	}
	entry, found := historyEntry(app, req.Revision)
	if !found {
		return nil, refuse("%s has no deployment %d in its history", ref.Name, req.Revision)
	}
	sync := syncOf(req)
	carry(sync, entry, "revision", "revisions", "source", "sources")
	return json.Marshal(operationOf(sync))
}

func carry(sync, entry map[string]any, fields ...string) {
	for _, field := range fields {
		value, found := entry[field]
		if !found {
			continue
		}
		sync[field] = value
	}
}

func historyEntry(app *unstructured.Unstructured, wanted int64) (map[string]any, bool) {
	for _, raw := range unstr.Slice(app, "status", "history") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, carried := entry["id"].(int64)
		if !carried || id != wanted {
			continue
		}
		return entry, true
	}
	return nil, false
}

func targetFor(dyn dynamic.Interface, ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace)
}
