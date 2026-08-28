package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	evictConcurrency = 8
	evictBudget      = 30 * time.Second
)

type podPlan struct {
	pod     *corev1.Pod
	outcome api.PodOutcome
}

func (s *Service) setSchedulable(ctx context.Context, ref api.ObjectRef, schedulable bool) (api.ActionResult, error) {
	err := s.patchSchedulable(ctx, ref, schedulable)
	if err != nil {
		return api.ActionResult{}, err
	}
	if schedulable {
		return api.ActionResult{
			Action:  string(Uncordon),
			Message: fmt.Sprintf("%s accepts new pods.", ref.Name),
		}, nil
	}
	return api.ActionResult{
		Action:  string(Cordon),
		Message: fmt.Sprintf("%s no longer accepts new pods. Running pods stay.", ref.Name),
	}, nil
}

func (s *Service) patchSchedulable(ctx context.Context, ref api.ObjectRef, schedulable bool) error {
	patch, err := json.Marshal(map[string]any{
		specField: map[string]any{"unschedulable": !schedulable},
	})
	if err != nil {
		return err
	}
	_, patchErr := s.target(ref).Patch(ctx, ref.Name, types.MergePatchType, patch, patchOptions())
	return patchErr
}

func (s *Service) drain(ctx context.Context, req Request) (api.ActionResult, error) {
	plans, err := s.planFor(ctx, req)
	if err != nil {
		return api.ActionResult{}, err
	}
	if req.DryRun {
		return api.ActionResult{
			Action:  string(Drain),
			DryRun:  true,
			Message: planMessage(plans),
			Pods:    outcomesOf(plans),
		}, nil
	}
	blocked := countOf(plans, api.OutcomeBlocked)
	if blocked > 0 {
		return api.ActionResult{}, fmt.Errorf("%d %s cannot be evicted safely; review them and drain with force to proceed", blocked, podWord(blocked))
	}
	cordonErr := s.patchSchedulable(ctx, req.Ref, false)
	if cordonErr != nil {
		return api.ActionResult{}, cordonErr
	}
	bounded, cancel := context.WithTimeout(ctx, evictBudget)
	defer cancel()
	outcomes := s.evictAll(bounded, plans)
	return api.ActionResult{
		Action:  string(Drain),
		Message: drainMessage(outcomes, errors.Is(bounded.Err(), context.DeadlineExceeded)),
		Pods:    outcomes,
	}, nil
}

func (s *Service) planFor(ctx context.Context, req Request) ([]podPlan, error) {
	list, err := s.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + req.Ref.Name,
	})
	if err != nil {
		return nil, err
	}
	plans := make([]podPlan, 0, len(list.Items))
	for i := range list.Items {
		pod := &list.Items[i]
		outcome, reason := classify(pod, req.Force)
		plans = append(plans, podPlan{
			pod: pod,
			outcome: api.PodOutcome{
				Namespace: pod.Namespace,
				Name:      pod.Name,
				Outcome:   outcome,
				Reason:    reason,
			},
		})
	}
	return plans, nil
}

func classify(pod *corev1.Pod, force bool) (string, string) {
	if pod.DeletionTimestamp != nil {
		return api.OutcomeSkipped, "already terminating"
	}
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return api.OutcomeSkipped, "already " + strings.ToLower(string(pod.Status.Phase))
	}
	if _, mirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; mirror {
		return api.OutcomeSkipped, "static pod, the kubelet owns it"
	}
	owner := metav1.GetControllerOf(pod)
	if owner != nil && owner.Kind == "DaemonSet" {
		return api.OutcomeSkipped, "daemonset pod, it would be recreated here"
	}
	if force {
		return api.OutcomeEvict, ""
	}
	if owner == nil {
		return api.OutcomeBlocked, "no controller owns it, nothing would recreate it"
	}
	volume := emptyDirVolume(pod)
	if volume != "" {
		return api.OutcomeBlocked, fmt.Sprintf("emptyDir volume %q would lose its data", volume)
	}
	return api.OutcomeEvict, ""
}

func emptyDirVolume(pod *corev1.Pod) string {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return volume.Name
		}
	}
	return ""
}

func (s *Service) evictAll(ctx context.Context, plans []podPlan) []api.PodOutcome {
	outcomes := make([]api.PodOutcome, len(plans))
	slots := make(chan struct{}, evictConcurrency)
	var group sync.WaitGroup
	for i := range plans {
		outcomes[i] = plans[i].outcome
		if plans[i].outcome.Outcome != api.OutcomeEvict {
			continue
		}
		group.Add(1)
		go safe.Run("evicting "+plans[i].pod.Namespace+"/"+plans[i].pod.Name, func() {
			defer group.Done()
			slots <- struct{}{}
			defer func() {
				<-slots
			}()
			outcome, reason := s.evictOne(ctx, plans[i].pod)
			outcomes[i].Outcome = outcome
			outcomes[i].Reason = reason
		})
	}
	group.Wait()
	return outcomes
}

func (s *Service) evictOne(ctx context.Context, pod *corev1.Pod) (string, string) {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name},
	}
	for {
		err := s.cs.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
		if err == nil {
			return api.OutcomeEvicted, ""
		}
		if apierrors.IsNotFound(err) {
			return api.OutcomeSkipped, "already gone"
		}
		if !apierrors.IsTooManyRequests(err) {
			if ctx.Err() != nil {
				return api.OutcomeFailed, "the drain ran out of time before this pod was evicted"
			}
			return api.OutcomeFailed, err.Error()
		}
		wait := s.sleep(ctx)
		if !wait {
			return api.OutcomeBlocked, "a PodDisruptionBudget still forbids the eviction, and the drain ran out of time"
		}
	}
}

func (s *Service) sleep(ctx context.Context) bool {
	timer := time.NewTimer(s.retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func outcomesOf(plans []podPlan) []api.PodOutcome {
	out := make([]api.PodOutcome, 0, len(plans))
	for _, plan := range plans {
		out = append(out, plan.outcome)
	}
	return out
}

func countOf(plans []podPlan, outcome string) int {
	total := 0
	for _, plan := range plans {
		if plan.outcome.Outcome == outcome {
			total++
		}
	}
	return total
}

func tally(outcomes []api.PodOutcome, outcome string) int {
	total := 0
	for _, entry := range outcomes {
		if entry.Outcome == outcome {
			total++
		}
	}
	return total
}

func podWord(count int) string {
	if count == 1 {
		return "pod"
	}
	return "pods"
}

func planMessage(plans []podPlan) string {
	outcomes := outcomesOf(plans)
	parts := []string{
		fmt.Sprintf("%d %s to evict", tally(outcomes, api.OutcomeEvict), podWord(tally(outcomes, api.OutcomeEvict))),
	}
	parts = appendCount(parts, tally(outcomes, api.OutcomeSkipped), "left in place")
	parts = appendCount(parts, tally(outcomes, api.OutcomeBlocked), "blocked")
	return strings.Join(parts, ", ") + "."
}

func drainMessage(outcomes []api.PodOutcome, expired bool) string {
	evicted := tally(outcomes, api.OutcomeEvicted)
	parts := []string{
		fmt.Sprintf("Cordoned. Eviction requested for %d %s", evicted, podWord(evicted)),
	}
	parts = appendCount(parts, tally(outcomes, api.OutcomeSkipped), "left in place")
	parts = appendCount(parts, tally(outcomes, api.OutcomeBlocked), "still blocked")
	parts = appendCount(parts, tally(outcomes, api.OutcomeFailed), "failed")
	message := strings.Join(parts, ", ") + "."
	if expired {
		return message + " The drain ran out of time, so run it again to finish."
	}
	return message
}

func appendCount(parts []string, count int, label string) []string {
	if count == 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%d %s", count, label))
}
