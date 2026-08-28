package flux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const specField = "spec"

const (
	groupSuffix         = ".toolkit.fluxcd.io"
	reconcileAnnotation = "reconcile.fluxcd.io/requestedAt"
	fieldManager        = "spinoza"
)

type Action string

const (
	Reconcile       Action = "reconcile"
	ReconcileSource Action = "reconcile-with-source"
	Suspend         Action = "suspend"
	Resume          Action = "resume"
)

var ErrNoSource = errors.New("this object names no source to reconcile first")

func IsFluxGroup(group string) bool {
	return strings.HasSuffix(group, groupSuffix)
}

func Do(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, ref api.ObjectRef, action Action, now time.Time) (api.FluxActionResult, error) {
	result := api.FluxActionResult{Action: string(action)}
	if !IsFluxGroup(ref.Group) {
		return result, fmt.Errorf("%q is not a flux resource group", ref.Group)
	}
	if action == Reconcile || action == ReconcileSource {
		result.RequestedAt = now.UTC().Format(time.RFC3339Nano)
	}
	if action == ReconcileSource {
		sourceErr := reconcileSource(ctx, dyn, descs, ref, result.RequestedAt)
		if sourceErr != nil {
			return api.FluxActionResult{Action: string(action)}, sourceErr
		}
	}
	patch, err := patchFor(action, result.RequestedAt)
	if err != nil {
		return result, err
	}
	_, patchErr := targetFor(dyn, ref).Patch(ctx, ref.Name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
	if patchErr != nil {
		return api.FluxActionResult{Action: string(action)}, patchErr
	}
	return result, nil
}

func patchFor(action Action, requestedAt string) ([]byte, error) {
	switch action {
	case Reconcile, ReconcileSource:
		return json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]any{
					reconcileAnnotation: requestedAt,
				},
			},
		})
	case Suspend:
		return suspendPatch(true)
	case Resume:
		return suspendPatch(false)
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

func suspendPatch(value bool) ([]byte, error) {
	return json.Marshal(map[string]any{
		specField: map[string]any{"suspend": value},
	})
}

func reconcileSource(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, ref api.ObjectRef, requestedAt string) error {
	obj, err := targetFor(dyn, ref).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	kind, name, namespace := SourceRef(obj)
	if kind == "" || name == "" {
		return fmt.Errorf("%w: %s", ErrNoSource, ref.Name)
	}
	if namespace == "" {
		namespace = ref.Namespace
	}
	desc, known := descriptorOf(descs, kind)
	if !known {
		return fmt.Errorf("%w: this cluster serves no %s", ErrNoSource, kind)
	}
	source := api.ObjectRef{
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Namespace: namespace,
		Name:      name,
	}
	patch, err := patchFor(Reconcile, requestedAt)
	if err != nil {
		return err
	}
	_, patchErr := targetFor(dyn, source).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
	return patchErr
}

func SourceRef(obj *unstructured.Unstructured) (kind, name, namespace string) {
	for _, path := range [][]string{
		{specField, "chartRef"},
		{specField, "sourceRef"},
		{specField, "chart", specField, "sourceRef"},
	} {
		kind = unstr.String(obj, append(path, "kind")...)
		name = unstr.String(obj, append(path, "name")...)
		if kind != "" && name != "" {
			return kind, name, unstr.String(obj, append(path, "namespace")...)
		}
	}
	return "", "", ""
}

func descriptorOf(descs map[string]api.ResourceDescriptor, kind string) (api.ResourceDescriptor, bool) {
	for _, desc := range descs {
		if !IsFluxGroup(desc.Group) {
			continue
		}
		if desc.Kind != kind {
			continue
		}
		return desc, true
	}
	return api.ResourceDescriptor{}, false
}

func targetFor(dyn dynamic.Interface, ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace)
}
