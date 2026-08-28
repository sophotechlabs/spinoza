package issues

import (
	"context"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorStall = "post-bind-stall"

const (
	stallGrace      = 5 * time.Minute
	stallCandidates = 20
)

func stallFindings(ctx context.Context, events Events, snap *snapshot, reported map[string]bool, now time.Time) []finding {
	candidates := stallCandidatesOf(snap, reported, now)
	out := []finding{}
	for _, item := range candidates {
		found, err := events.Events(ctx, item.obj.GetNamespace(), item.uid())
		if err != nil || len(found) > 0 {
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
			since:     podSince(item.obj),
		})
	}
	return out
}

func stallCandidatesOf(snap *snapshot, reported map[string]bool, now time.Time) []object {
	out := []object{}
	for _, item := range snap.of("", kindPod) {
		if !stalledPod(item.obj, reported[item.uid()], now) {
			continue
		}
		out = append(out, item)
	}
	slices.SortStableFunc(out, func(left, right object) int {
		if !podSince(left.obj).Equal(podSince(right.obj)) {
			return podSince(left.obj).Compare(podSince(right.obj))
		}
		return strings.Compare(whereOf(left), whereOf(right))
	})
	if len(out) > stallCandidates {
		return out[:stallCandidates]
	}
	return out
}

func stalledPod(pod *unstructured.Unstructured, alreadyReported bool, now time.Time) bool {
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
	return now.Sub(podSince(pod)) >= stallGrace
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
	waited := now.Sub(podSince(pod)).Round(time.Minute)
	return "bound to " + node + " " + waited.String() + " ago, no container running and no events at all"
}
