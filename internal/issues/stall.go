package issues

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
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
	quiet, complained := whatHappened(bounded, events, candidates, snap.failures, limits)
	out := []finding{}
	for _, item := range candidates {
		if said, ok := complained[item.uid()]; ok {
			moved := rolloutOf(snap, item)
			out = append(out, finding{
				detector:  detectorStall,
				severity:  severityFatal,
				title:     said.Reason,
				detail:    said.Message,
				action:    "fix what the kubelet is complaining about, then let it retry",
				change:    moved.what,
				changedAt: moved.at,
				kind:      kindPod,
				subject:   item,
				since:     podBoundAt(item.obj),
			})
			continue
		}
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

func whatHappened(
	ctx context.Context, events Events, candidates []object, failures *listerr.Collector, limits Limits,
) (quiet map[string]bool, complained map[string]api.Event) {
	quietAt := make([]bool, len(candidates))
	saidAt := make([]api.Event, len(candidates))
	slots := make(chan struct{}, limits.StallReader)
	var wg sync.WaitGroup
	for index, item := range candidates {
		what := "asking what happened to " + item.obj.GetName()
		wg.Add(1)
		safe.Go(what, func() {
			defer wg.Done()
			defer func() {
				failures.RecordPanic("events", what, recover())
			}()
			slots <- struct{}{}
			defer func() { <-slots }()
			found, err := events.Events(ctx, item.obj.GetNamespace(), item.uid())
			failures.Record("events", err)
			if err != nil {
				return
			}
			if len(found) == 0 {
				quietAt[index] = true
				return
			}
			if worst, ok := newestWarning(found); ok {
				saidAt[index] = worst
			}
		})
	}
	wg.Wait()
	quiet = map[string]bool{}
	complained = map[string]api.Event{}
	for index, item := range candidates {
		if quietAt[index] {
			quiet[item.uid()] = true
			continue
		}
		if saidAt[index].Reason != "" {
			complained[item.uid()] = saidAt[index]
		}
	}
	return quiet, complained
}

func newestWarning(found []api.Event) (api.Event, bool) {
	for _, one := range found {
		if one.Type != "Warning" || one.Reason == "" {
			continue
		}
		return one, true
	}
	return api.Event{}, false
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
	return "bound to " + node + " " + durationLabel(waited) + " ago, no container running and no events at all"
}
