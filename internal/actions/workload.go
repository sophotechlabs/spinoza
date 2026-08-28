package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const restartAnnotation = "kubectl.kubernetes.io/restartedAt"

func (s *Service) scale(ctx context.Context, req Request) (api.ActionResult, error) {
	if req.Replicas < 0 {
		return api.ActionResult{}, errors.New("replicas must be zero or more")
	}
	patch, err := json.Marshal(map[string]any{
		specField: map[string]any{"replicas": req.Replicas},
	})
	if err != nil {
		return api.ActionResult{}, err
	}
	_, patchErr := s.target(req.Ref).Patch(ctx, req.Ref.Name, types.MergePatchType, patch, patchOptions(), "scale")
	if patchErr != nil {
		return api.ActionResult{}, patchErr
	}
	return api.ActionResult{
		Action:  string(Scale),
		Message: fmt.Sprintf("Scaled %s to %d %s.", req.Ref.Name, req.Replicas, plural(req.Replicas)),
	}, nil
}

func plural(replicas int64) string {
	if replicas == 1 {
		return "replica"
	}
	return "replicas"
}

func (s *Service) restart(ctx context.Context, req Request, now time.Time) (api.ActionResult, error) {
	stamp := now.UTC().Format(time.RFC3339)
	patch, err := json.Marshal(map[string]any{
		specField: map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{restartAnnotation: stamp},
				},
			},
		},
	})
	if err != nil {
		return api.ActionResult{}, err
	}
	_, patchErr := s.target(req.Ref).Patch(ctx, req.Ref.Name, types.MergePatchType, patch, patchOptions())
	if patchErr != nil {
		return api.ActionResult{}, patchErr
	}
	return api.ActionResult{
		Action:  string(Restart),
		Message: fmt.Sprintf("Rollout restart requested at %s.", stamp),
	}, nil
}

func patchOptions() metav1.PatchOptions {
	return metav1.PatchOptions{FieldManager: fieldManager}
}

func (s *Service) target(ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return s.dyn.Resource(gvr)
	}
	return s.dyn.Resource(gvr).Namespace(ref.Namespace)
}
