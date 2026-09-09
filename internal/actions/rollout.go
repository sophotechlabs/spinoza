package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/rollout"
)

var errNoRevision = errors.New("undo needs a revision to go back to")

func (s *Service) undo(ctx context.Context, req Request) (api.ActionResult, error) {
	if req.Revision == 0 {
		return api.ActionResult{}, errNoRevision
	}
	template, err := rollout.Template(ctx, rollout.FromDynamic(s.dyn), req.Ref, req.Revision)
	if err != nil {
		return api.ActionResult{}, err
	}
	if req.DryRun {
		return api.ActionResult{
			Action:  string(Undo),
			DryRun:  true,
			Message: fmt.Sprintf("%s would go back to the pod template of revision %d.", req.Ref.Name, req.Revision),
		}, nil
	}
	patch, marshalErr := json.Marshal(map[string]any{
		specField: map[string]any{"template": template},
	})
	if marshalErr != nil {
		return api.ActionResult{}, marshalErr
	}
	_, patchErr := s.target(req.Ref).Patch(ctx, req.Ref.Name, types.MergePatchType, patch, patchOptions())
	if patchErr != nil {
		return api.ActionResult{}, patchErr
	}
	return api.ActionResult{
		Action:  string(Undo),
		Message: fmt.Sprintf("Rolled %s back to revision %d.", req.Ref.Name, req.Revision),
	}, nil
}
