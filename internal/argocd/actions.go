package argocd

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	refreshAnnotation = "argocd.argoproj.io/refresh"
	normalRefresh     = "normal"
	fieldManager      = "spinoza"
)

type Action string

const (
	Sync    Action = "sync"
	Refresh Action = "refresh"
)

func IsArgoGroup(group string) bool {
	return group == Group
}

func Do(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef, action Action) (api.ArgoActionResult, error) {
	result := api.ArgoActionResult{Action: string(action)}
	if !IsArgoGroup(ref.Group) {
		return result, fmt.Errorf("%q is not an argo cd resource group", ref.Group)
	}
	if ref.Resource != applications {
		return result, fmt.Errorf("only applications can be synced or refreshed, not %q", ref.Resource)
	}
	patch, err := patchFor(action)
	if err != nil {
		return result, err
	}
	_, patchErr := targetFor(dyn, ref).Patch(ctx, ref.Name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
	if patchErr != nil {
		return api.ArgoActionResult{Action: string(action)}, patchErr
	}
	return result, nil
}

func patchFor(action Action) ([]byte, error) {
	switch action {
	case Sync:
		return json.Marshal(map[string]any{
			"operation": map[string]any{
				"initiatedBy": map[string]any{"username": fieldManager},
				"info":        []any{map[string]any{"name": "Reason", "value": "requested from spinoza"}},
				"sync":        map[string]any{},
			},
		})
	case Refresh:
		return json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]any{
					refreshAnnotation: normalRefresh,
				},
			},
		})
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

func targetFor(dyn dynamic.Interface, ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace)
}
