package issues

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func buildWith(t *testing.T, lister *stubLister, events *stubEvents, descs map[string]api.ResourceDescriptor) api.IssueQueue {
	t.Helper()
	return buildLimited(t, lister, events, descs, testLimits())
}

func silentPod(name string) *stubLister {
	pod := newPod(
		name,
		withPhase(phasePending),
		withStartTime(testNow.Add(-10*time.Minute)),
	)
	return &stubLister{items: itemsOf("pods", pod)}
}

func TestAPodThatWentQuietAfterBindingIsReported(t *testing.T) {
	lister := silentPod("web-1")
	events := &stubEvents{}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))

	row, ok := rowNamed(queue, "web-1")
	if !ok || row.Detector != detectorStall {
		t.Fatalf("row = %+v, want the stall row", row)
	}
	if !row.Uncertain {
		t.Fatalf("row = %+v, want the guess stated as a guess", row)
	}
	if !contains(row.Detail, "bound to node-a") || !contains(row.Detail, "no events at all") {
		t.Fatalf("detail = %q, want what was observed", row.Detail)
	}
	if !contains(row.Detail, "10m ago") {
		t.Fatalf("detail = %q, want the elapsed time in interface units", row.Detail)
	}
	if row.Severity != api.SeverityWarning {
		t.Fatalf("severity = %q, want warning", row.Severity)
	}
}

func TestAPodWithEventsIsNotAStall(t *testing.T) {
	lister := silentPod("web-2")
	events := &stubEvents{byUID: map[string][]api.Event{
		"uid-web-2": {{Reason: "Pulling"}},
	}}

	if queue := buildWith(t, lister, events, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none once the kubelet has said something", queue.Rows)
	}
}

func TestAnEventLookupThatFailsReportsNothingAndNamesTheGap(t *testing.T) {
	lister := silentPod("web-3")
	events := &stubEvents{err: errors.New("forbidden")}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))
	if len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want silence rather than a guess", queue.Rows)
	}
	if !contains(queue.Error, "events") || !contains(queue.Error, "forbidden") {
		t.Fatalf("error = %q, want the missing event evidence named", queue.Error)
	}
}

func TestAnEventLookupPanicReportsNothingAndNamesTheGap(t *testing.T) {
	lister := silentPod("web-3")
	events := &stubEvents{panics: true}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))
	if len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want silence rather than a guess", queue.Rows)
	}
	if !contains(queue.Error, "events") {
		t.Fatalf("error = %q, want the panicking event reader named", queue.Error)
	}
}

func TestAPodInsideTheGraceIsNotAStall(t *testing.T) {
	pod := newPod("web-4", withPhase(phasePending), withStartTime(testNow.Add(-time.Minute)))
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := buildWith(t, lister, &stubEvents{}, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want the pod given its grace", queue.Rows)
	}
}

func TestAnUnboundPodIsNotAStall(t *testing.T) {
	pod := newPod(
		"web-5",
		withPhase(phasePending),
		withNode(""),
		withStartTime(testNow.Add(-time.Hour)),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := buildWith(t, lister, &stubEvents{}, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want an unscheduled pod left to the scheduler", queue.Rows)
	}
}

func TestAPodWithARunningContainerIsNotAStall(t *testing.T) {
	pod := newPod(
		"web-6",
		withStartTime(testNow.Add(-time.Hour)),
		withContainer("app", map[string]any{"running": map[string]any{}}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := buildWith(t, lister, &stubEvents{}, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAPodAlreadyReportedIsNotAskedAboutAgain(t *testing.T) {
	pod := newPod(
		"web-7",
		withPhase(phasePending),
		withStartTime(testNow.Add(-time.Hour)),
		withContainer("app", map[string]any{
			"waiting": map[string]any{"reason": "ImagePullBackOff"},
		}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}
	events := &stubEvents{}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))

	if len(events.asked) != 0 {
		t.Fatalf("asked = %v, want no event lookup for a pod already explained", events.asked)
	}
	row, _ := rowNamed(queue, "web-7")
	if row.Detector != detectorStartup {
		t.Fatalf("detector = %q, want the startup detector", row.Detector)
	}
}

func TestATerminatingPodIsNotAStall(t *testing.T) {
	pod := newPod(
		"web-8",
		withPhase(phasePending),
		withStartTime(testNow.Add(-time.Hour)),
		withDeleted(),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	if queue := buildWith(t, lister, &stubEvents{}, catalog(podDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestOnlyTheOldestCandidatesAreAskedAbout(t *testing.T) {
	pods := make([]*unstructured.Unstructured, 0, defaultCandidates+5)
	for index := range defaultCandidates + 5 {
		name := "web-" + strconv.Itoa(index)
		pods = append(pods, newPod(
			name,
			withPhase(phasePending),
			withStartTime(testNow.Add(-time.Duration(index+10)*time.Minute)),
		))
	}
	lister := &stubLister{items: itemsOf("pods", pods...)}
	events := &stubEvents{}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))

	asked := events.askedAbout()
	if len(asked) != defaultCandidates {
		t.Fatalf("asked = %d pods, want the %d oldest", len(asked), defaultCandidates)
	}
	if len(queue.Rows) != defaultCandidates {
		t.Fatalf("rows = %d, want one per candidate asked about", len(queue.Rows))
	}
	if !slices.Contains(asked, "uid-web-24") {
		t.Fatalf("asked = %v, want the oldest candidate among them", asked)
	}
	if slices.Contains(asked, "uid-web-0") {
		t.Fatalf("asked = %v, want the newest candidates left out", asked)
	}
}

func TestAPodTheKubeletComplainedAboutIsReportedInItsOwnWords(t *testing.T) {
	lister := silentPod("web-5")
	events := &stubEvents{byUID: map[string][]api.Event{
		"uid-web-5": {{
			Type:    "Warning",
			Reason:  "FailedMount",
			Message: `MountVolume.SetUp failed for volume "creds": secret "api" not found`,
		}},
	}}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))

	row, ok := rowNamed(queue, "web-5")
	if !ok {
		t.Fatalf("rows = %+v, want the pod reported", queue.Rows)
	}
	if row.Title != "FailedMount" {
		t.Fatalf("title = %q, want the kubelet's own reason", row.Title)
	}
	if !contains(row.Detail, `secret "api" not found`) {
		t.Fatalf("detail = %q, want the kubelet's own message", row.Detail)
	}
	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal", row.Severity)
	}
	if row.Uncertain {
		t.Fatal("a message the kubelet sent was reported as a guess")
	}
}

func TestTheNewestWarningIsTheOneReported(t *testing.T) {
	lister := silentPod("web-6")
	events := &stubEvents{byUID: map[string][]api.Event{
		"uid-web-6": {
			{Type: "Normal", Reason: "Scheduled"},
			{Type: "Warning", Reason: "FailedAttachVolume", Message: "still attaching"},
			{Type: "Warning", Reason: "FailedMount", Message: "older"},
		},
	}}

	queue := buildWith(t, lister, events, catalog(podDescriptor()))

	row, _ := rowNamed(queue, "web-6")
	if row.Title != "FailedAttachVolume" {
		t.Fatalf("title = %q, want the first warning the reader was given", row.Title)
	}
}
