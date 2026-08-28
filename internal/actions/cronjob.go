package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const instantiateAnnotation = "cronjob.kubernetes.io/instantiate"

var jobsGVR = schema.GroupVersionResource{Group: batchGroup, Version: "v1", Resource: "jobs"}

func (s *Service) setSuspended(ctx context.Context, ref api.ObjectRef, suspended bool) (api.ActionResult, error) {
	patch, err := json.Marshal(map[string]any{
		specField: map[string]any{"suspend": suspended},
	})
	if err != nil {
		return api.ActionResult{}, err
	}
	_, patchErr := s.target(ref).Patch(ctx, ref.Name, types.MergePatchType, patch, patchOptions())
	if patchErr != nil {
		return api.ActionResult{}, patchErr
	}
	if suspended {
		return api.ActionResult{
			Action:  string(Suspend),
			Message: fmt.Sprintf("Suspended %s. Runs already going are left alone.", ref.Name),
		}, nil
	}
	return api.ActionResult{
		Action:  string(Resume),
		Message: fmt.Sprintf("Resumed %s.", ref.Name),
	}, nil
}

func (s *Service) trigger(ctx context.Context, ref api.ObjectRef, now time.Time) (api.ActionResult, error) {
	cron, err := s.target(ref).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return api.ActionResult{}, err
	}
	job, err := jobFrom(cron, now)
	if err != nil {
		return api.ActionResult{}, err
	}
	made, createErr := s.jobs(ref).Create(ctx, job, metav1.CreateOptions{FieldManager: fieldManager})
	if createErr != nil {
		return api.ActionResult{}, createErr
	}
	return api.ActionResult{
		Action:  string(Trigger),
		Message: fmt.Sprintf("Started job %s.", made.GetName()),
	}, nil
}

func (s *Service) jobs(ref api.ObjectRef) dynamic.ResourceInterface {
	return s.dyn.Resource(jobsGVR).Namespace(ref.Namespace)
}

// The job is owned by its cron job so deleting one takes the other with it,
// but not controlled by it: a run started by hand must not count towards the
// schedule's concurrency or history limits.
func jobFrom(cron *unstructured.Unstructured, now time.Time) (*unstructured.Unstructured, error) {
	spec, found := unstr.Map(cron, specField, "jobTemplate", specField)
	if !found {
		return nil, errors.New("the cron job carries no job template")
	}
	annotations, _ := unstr.Map(cron, specField, "jobTemplate", "metadata", "annotations")
	if annotations == nil {
		annotations = map[string]any{}
	}
	annotations[instantiateAnnotation] = "manual"
	meta := map[string]any{
		"name":        fmt.Sprintf("%s-%d", cron.GetName(), now.Unix()),
		"namespace":   cron.GetNamespace(),
		"annotations": annotations,
		"ownerReferences": []any{map[string]any{
			"apiVersion": batchGroup + "/v1",
			"kind":       "CronJob",
			"name":       cron.GetName(),
			"uid":        string(cron.GetUID()),
		}},
	}
	if labels, ok := unstr.Map(cron, specField, "jobTemplate", "metadata", "labels"); ok {
		meta["labels"] = labels
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": batchGroup + "/v1",
		"kind":       "Job",
		"metadata":   meta,
		specField:    spec,
	}}, nil
}
