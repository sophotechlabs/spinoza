package issues

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/safe"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorStall = "post-bind-stall"

func stallFindings(ctx context.Context, events Events, snap *snapshot, reported map[string]bool, now time.Time, limits Limits) []finding {
	candidates := stallCandidatesOf(snap, reported, now, limits)
	if len(candidates) == 0 {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, limits.StallBudget)
	defer cancel()
	quiet := quietOnes(bounded, events, candidates, limits)
	out := []finding{}
	for _, item := range candidates {
		if !quiet[item.uid()] {
			continue
		}
		moved := rolloutOf(snap, item)
		out = append(out, finding{
			detector:  detectorStall,
			severity:  severityWarning,
			title:     "SilentAfterBinding",
			detail:    stallDetail(item.obj, now),
			action:    "check the kubelet on " + unstr.String(item.obj, "spec", "nodeName"),
			change:    moved.what,
			changedAt: moved.at,
			uncertain: true,
			kind:      kindPod,
			subject:   item,
			since:     podBoundAt(item.obj),
		})
	}
	return out
}

func quietOnes(ctx context.Context, events Events, candidates []object, limits Limits) map[string]bool {
	quiet := make([]bool, len(candidates))
	slots := make(chan struct{}, limits.StallReader)
	var wg sync.WaitGroup
	for index, item := range candidates {
		wg.Add(1)
		go safe.Run("asking what happened to "+item.obj.GetName(), func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			found, err := events.Events(ctx, item.obj.GetNamespace(), item.uid())
			quiet[index] = err == nil && len(found) == 0
		})
	}
	wg.Wait()
	out := map[string]bool{}
	for index, item := range candidates {
		if quiet[index] {
			out[item.uid()] = true
		}
	}
	return out
}

func stallCandidatesOf(snap *snapshot, reported map[string]bool, now time.Time, limits Limits) []object {
	out := []object{}
	for _, item := range snap.of("", kindPod) {
		if !stalledPod(item.obj, reported[item.uid()], now, limits) {
			continue
		}
		out = append(out, item)
	}
	slices.SortStableFunc(out, func(left, right object) int {
		if !podBoundAt(left.obj).Equal(podBoundAt(right.obj)) {
			return podBoundAt(left.obj).Compare(podBoundAt(right.obj))
		}
		return strings.Compare(whereOf(left), whereOf(right))
	})
	if len(out) > limits.Candidates {
		return out[:limits.Candidates]
	}
	return out
}

func stalledPod(pod *unstructured.Unstructured, alreadyReported bool, now time.Time, limits Limits) bool {
	if alreadyReported || pod.GetDeletionTimestamp() != nil {
		return false
	}
	if unstr.String(pod, "spec", "nodeName") == "" {
		return false
	}
	phase := unstr.String(pod, "status", "phase")
	if phase == phaseSucceeded || phase == phaseFailed {
		return false
	}
	if anyContainerRunning(pod) {
		return false
	}
	return now.Sub(podBoundAt(pod)) >= limits.StallGrace
}

func anyContainerRunning(pod *unstructured.Unstructured) bool {
	for _, field := range containerStatusFields {
		for _, raw := range unstr.Slice(pod, "status", field) {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			state, hasState := entry["state"].(map[string]any)
			if !hasState {
				continue
			}
			if _, running := state["running"]; running {
				return true
			}
		}
	}
	return false
}

func stallDetail(pod *unstructured.Unstructured, now time.Time) string {
	node := unstr.String(pod, "spec", "nodeName")
	waited := now.Sub(podBoundAt(pod)).Round(time.Minute)
	return "bound to " + node + " " + waited.String() + " ago, no container running and no events at all"
}
