package issues

import (
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestACrashLoopIsFatalAndNamesTheExitCode(t *testing.T) {
	pod := newPod("web-1", withContainerEntry(map[string]any{
		"name":         "app",
		"restartCount": int64(7),
		"state": map[string]any{
			"waiting": map[string]any{"reason": "CrashLoopBackOff"},
		},
		"lastState": map[string]any{
			"terminated": map[string]any{"exitCode": int64(1), "reason": "Error"},
		},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, ok := rowNamed(queue, "web-1")
	if !ok {
		t.Fatalf("rows = %+v, want the crashlooping pod", queue.Rows)
	}
	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal", row.Severity)
	}
	if row.Title != "CrashLoopBackOff" {
		t.Fatalf("title = %q, want CrashLoopBackOff", row.Title)
	}
	if !contains(row.Detail, "exit code 1") || !contains(row.Detail, "restarted 7 times") {
		t.Fatalf("detail = %q, want the exit code and the restart count", row.Detail)
	}
}

func TestAnImagePullFailureCarriesTheMessage(t *testing.T) {
	pod := newPod("web-2", withContainer("app", map[string]any{
		"waiting": map[string]any{"reason": "ImagePullBackOff", "message": "pull access denied"},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-2")
	if row.Title != "ImagePullBackOff" || !contains(row.Detail, "pull access denied") {
		t.Fatalf("row = %+v, want the image pull message", row)
	}
}

func TestAConfigErrorIsReported(t *testing.T) {
	pod := newPod("web-3", withContainer("app", map[string]any{
		"waiting": map[string]any{"reason": "CreateContainerConfigError"},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-3")
	if row.Title != "CreateContainerConfigError" {
		t.Fatalf("title = %q, want the config error", row.Title)
	}
	if !contains(row.Detail, "container app") {
		t.Fatalf("detail = %q, want the container named", row.Detail)
	}
}

func TestAnOOMKillIsReported(t *testing.T) {
	pod := newPod("web-4", withContainer("app", map[string]any{
		"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-4")
	if row.Title != "OOMKilled" || row.Severity != api.SeverityFatal {
		t.Fatalf("row = %+v, want a fatal OOMKilled row", row)
	}
}

func TestATerminatedContainerThatIsNotOOMKilledIsNotAnIssue(t *testing.T) {
	pod := newPod(
		"web-5",
		withStartTime(testNow.Add(-time.Minute)),
		withContainer("app", map[string]any{
			"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)},
		}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	if len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAnUnschedulablePodIsReportedWithTheSchedulerMessage(t *testing.T) {
	pod := newPod(
		"web-6",
		withPhase(phasePending),
		withNode(""),
		withPodCondition(map[string]any{
			"type":    "PodScheduled",
			"status":  "False",
			"reason":  "Unschedulable",
			"message": "0/3 nodes are available: insufficient cpu",
		}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-6")
	if row.Title != "Unschedulable" || !contains(row.Detail, "insufficient cpu") {
		t.Fatalf("row = %+v, want the scheduler message", row)
	}
}

func TestAPodScheduledConditionWithoutAMessageStillReports(t *testing.T) {
	pod := newPod(
		"web-7",
		withPhase(phasePending),
		withNode(""),
		withPodCondition(map[string]any{"type": "PodScheduled", "status": "False"}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-7")
	if !contains(row.Detail, "no node accepted") {
		t.Fatalf("detail = %q, want the fallback text", row.Detail)
	}
}

func TestAFailedPodIsReportedWithItsReason(t *testing.T) {
	pod := newPod("web-8", withPhase(phaseFailed))
	setNested(pod, "Evicted", "status", "reason")
	setNested(pod, "the node was low on ephemeral-storage", "status", "message")
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-8")
	if row.Title != "Evicted" || !contains(row.Detail, "ephemeral-storage") {
		t.Fatalf("row = %+v, want the eviction reason", row)
	}
}

func TestAFailedPodWithoutAReasonFallsBackToItsPhase(t *testing.T) {
	pod := newPod("web-9", withPhase(phaseFailed))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-9")
	if row.Title != "PodFailed" || !contains(row.Detail, "Failed phase") {
		t.Fatalf("row = %+v, want the fallback", row)
	}
}

func TestASucceededPodIsNotAnIssue(t *testing.T) {
	pod := newPod("job-1", withPhase(phaseSucceeded))
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := build(t, lister, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestATerminatingPodIsNotAnIssue(t *testing.T) {
	pod := newPod(
		"web-10",
		withDeleted(),
		withContainer("app", map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := build(t, lister, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none while it is going away", queue.Rows)
	}
}

func TestAHealthyPodIsNotAnIssue(t *testing.T) {
	pod := newPod("web-11", withContainer("app", map[string]any{"running": map[string]any{}}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := build(t, lister, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestTheWorstContainerWins(t *testing.T) {
	pod := newPod(
		"web-12",
		withContainer("sidecar", map[string]any{"waiting": map[string]any{"reason": "PodInitializing"}}),
		withContainer("app", map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-12")
	if row.Title != "ImagePullBackOff" {
		t.Fatalf("title = %q, want the container that actually broke", row.Title)
	}
}

func TestAContainerStatusThatIsNotAMapIsSkipped(t *testing.T) {
	pod := newPod("web-13", withStartTime(testNow.Add(-time.Minute)))
	setNested(pod, []any{"not a status"}, "status", "containerStatuses")
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := build(t, lister, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want the noise skipped", queue.Rows)
	}
}

func TestAContainerWithoutAStateIsSkipped(t *testing.T) {
	pod := newPod("web-14", withStartTime(testNow.Add(-time.Minute)))
	appendNested(pod, map[string]any{"name": "app"}, "status", "containerStatuses")
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := build(t, lister, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAPodWithoutAStartTimeUsesItsCreationTime(t *testing.T) {
	pod := newPod("web-15", withContainer("app", map[string]any{
		"waiting": map[string]any{"reason": "CrashLoopBackOff"},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-15")
	if row.Since != testNow.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("since = %q, want the creation time", row.Since)
	}
}

func TestAStartTimeIsPreferredOverTheCreationTime(t *testing.T) {
	started := testNow.Add(-10 * time.Minute)
	pod := newPod(
		"web-16",
		withStartTime(started),
		withContainer("app", map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-16")
	if row.Since != started.Format(time.RFC3339) {
		t.Fatalf("since = %q, want the start time", row.Since)
	}
}

func TestACrashLoopWithoutALastStateStillReports(t *testing.T) {
	pod := newPod("web-17", withContainer("app", map[string]any{
		"waiting": map[string]any{"reason": "CrashLoopBackOff"},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-17")
	if row.Detail != "container app keeps exiting" {
		t.Fatalf("detail = %q, want the bare statement", row.Detail)
	}
}

func TestACrashLoopNamesANonErrorTerminationReason(t *testing.T) {
	pod := newPod("web-18", withContainerEntry(map[string]any{
		"name":  "app",
		"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
		"lastState": map[string]any{
			"terminated": map[string]any{"exitCode": int64(137), "reason": "OOMKilled"},
		},
	}))
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-18")
	if !contains(row.Detail, "(OOMKilled)") {
		t.Fatalf("detail = %q, want the termination reason", row.Detail)
	}
}
