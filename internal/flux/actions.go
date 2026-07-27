package flux

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	groupSuffix         = ".toolkit.fluxcd.io"
	reconcileAnnotation = "reconcile.fluxcd.io/requestedAt"
	fieldManager        = "spinoza"
)

type Action string

const (
	Reconcile Action = "reconcile"
	Suspend   Action = "suspend"
	Resume    Action = "resume"
)

func IsFluxGroup(group string) bool {
	return strings.HasSuffix(group, groupSuffix)
}

func Do(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef, action Action, now time.Time) error {
	if !IsFluxGroup(ref.Group) {
		return fmt.Errorf("%q is not a flux resource group", ref.Group)
	}
	patch, err := patchFor(action, now)
	if err != nil {
		return err
	}
	_, patchErr := targetFor(dyn, ref).Patch(ctx, ref.Name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
	return patchErr
}

func patchFor(action Action, now time.Time) ([]byte, error) {
	switch action {
	case Reconcile:
		return json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					reconcileAnnotation: now.UTC().Format(time.RFC3339Nano),
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
	return json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{"suspend": value},
	})
}

func targetFor(dyn dynamic.Interface, ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace)
}
