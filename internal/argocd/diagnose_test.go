package argocd

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func issueTitled(issues []api.GitopsIssue, title string) (api.GitopsIssue, bool) {
	for _, one := range issues {
		if one.Title == title {
			return one, true
		}
	}
	return api.GitopsIssue{}, false
}

func issueAbout(issues []api.GitopsIssue, subject string) (api.GitopsIssue, bool) {
	for _, one := range issues {
		if one.Subject == subject {
			return one, true
		}
	}
	return api.GitopsIssue{}, false
}

func TestAHealthyApplicationHasNoIssues(t *testing.T) {
	if got := Detail(detailed()); got.Issues != nil {
		t.Fatalf("issues = %+v, want none", got.Issues)
	}
}

func TestConditionsBecomeTypedIssues(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"type": "ComparisonError", "message": "authentication required"},
		map[string]any{"type": "OrphanedResourceWarning", "message": "Secret/db is orphaned"},
		map[string]any{"type": "SomethingElse", "message": "who knows"},
	}, "status", "conditions")

	issues := Detail(app).Issues

	comparison, found := issueAbout(issues, "ComparisonError")
	if !found || comparison.Severity != api.SeverityDegraded {
		t.Fatalf("comparison issue = %+v, want it degraded", comparison)
	}
	if !strings.Contains(comparison.Detail, "repository credentials") {
		t.Fatalf("detail = %q, want it to say what to do", comparison.Detail)
	}
	orphaned, found := issueAbout(issues, "OrphanedResourceWarning")
	if !found || orphaned.Severity != api.SeverityWarning {
		t.Fatalf("orphan issue = %+v, want a warning", orphaned)
	}
	other, found := issueAbout(issues, "SomethingElse")
	if !found || other.Severity != api.SeverityInfo {
		t.Fatalf("unclassified issue = %+v, want info", other)
	}
	if other.Detail != "" {
		t.Fatalf("detail = %q, want nothing invented for a type we do not know", other.Detail)
	}
}

func TestConditionsWithoutATypeAreSkipped(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"message": "no type here"},
		"not a map",
	}, "status", "conditions")

	if got := Detail(app); got.Issues != nil {
		t.Fatalf("issues = %+v, want none", got.Issues)
	}
}

func TestAFailedOperationBecomesAnIssueWithItsCause(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")
	_ = unstructured.SetNestedField(app.Object, "Operation terminated", "status", "operationState", "message")

	issue, found := issueAbout(Detail(app).Issues, "operation")

	if !found {
		t.Fatal("a failed operation raised no issue")
	}
	if issue.Title != "The last operation failed" {
		t.Fatalf("title = %q", issue.Title)
	}
	if !strings.HasPrefix(issue.Detail, "the operation was stopped") {
		t.Fatalf("detail = %q, want the cause first", issue.Detail)
	}
	if !strings.HasSuffix(issue.Detail, "Operation terminated") {
		t.Fatalf("detail = %q, want the raw message kept", issue.Detail)
	}
}

func TestAFailureWithNoKnownCauseKeepsTheRawMessage(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Error", "status", "operationState", "phase")
	_ = unstructured.SetNestedField(app.Object, "something odd happened", "status", "operationState", "message")

	issue, _ := issueAbout(Detail(app).Issues, "operation")

	if issue.Detail != "something odd happened" {
		t.Fatalf("detail = %q, want the message as it came", issue.Detail)
	}
}

func TestAFailureWithNoMessageAtAllStatesTheCauseAlone(t *testing.T) {
	if got := detailOf(""); got != "" {
		t.Fatalf("detail = %q, want nothing", got)
	}
}

func TestARunningOperationIsNotAnIssue(t *testing.T) {
	app := operating(detailed(), runningPhase)

	if _, found := issueAbout(Detail(app).Issues, "operation"); found {
		t.Fatal("an operation still running was reported as a problem")
	}
}

func TestDriftWithAutoSyncOffSaysNothingWillReconcileIt(t *testing.T) {
	app := automating(detailed(), map[string]any{"enabled": false})
	_ = unstructured.SetNestedField(app.Object, outOfSync, "status", "sync", "status")

	issue, found := issueAbout(Detail(app).Issues, "drift")

	if !found {
		t.Fatal("drift with auto-sync off raised no issue")
	}
	if issue.Title != "Nothing will reconcile this" {
		t.Fatalf("title = %q", issue.Title)
	}
}

func TestDriftThatKeepsComingBackIsCalledOut(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, outOfSync, "status", "sync", "status")

	issue, found := issueAbout(Detail(app).Issues, "drift")

	if !found {
		t.Fatal("a synced-then-out-of-sync loop raised no issue")
	}
	if issue.Title != "Synced, and out of sync again" {
		t.Fatalf("title = %q", issue.Title)
	}
	if !strings.Contains(issue.Detail, "mutating webhook") {
		t.Fatalf("detail = %q, want the likely culprits named", issue.Detail)
	}
}

func TestDriftIsNotCalledALoopWhileTheLastSyncFailed(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, outOfSync, "status", "sync", "status")
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")

	if _, found := issueAbout(Detail(app).Issues, "drift"); found {
		t.Fatal("a failed sync was reported as a drift loop")
	}
}

func TestASyncedApplicationRaisesNoDriftIssue(t *testing.T) {
	if _, found := issueAbout(Detail(detailed()).Issues, "drift"); found {
		t.Fatal("a synced application was reported as drifting")
	}
}

func TestBrokenResourcesEachGetAnIssue(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{
			"group": "apps", "version": "v1", "kind": "Deployment", "name": "podinfo", "namespace": "web",
			"status": "Synced", "health": map[string]any{"status": "Degraded", "message": "0/3 ready"},
		},
		map[string]any{
			"version": "v1", "kind": "Service", "name": "podinfo", "namespace": "web",
			"status": "OutOfSync", "health": map[string]any{"status": "Missing"},
		},
	}, "status", "resources")

	issues := Detail(app).Issues

	broken, found := issueAbout(issues, "Deployment/podinfo")
	if !found || broken.Severity != api.SeverityFatal {
		t.Fatalf("degraded issue = %+v, want the same word the issues queue uses", broken)
	}
	if broken.Detail != "0/3 ready" {
		t.Fatalf("detail = %q, want the health message", broken.Detail)
	}
	gone, found := issueAbout(issues, "Service/podinfo")
	if !found || gone.Severity != api.SeverityDegraded {
		t.Fatalf("missing issue = %+v, want it ranked under a degraded one", gone)
	}
}

func TestAResourceTheOperationAlreadyBlamedIsNotRepeated(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")
	_ = unstructured.SetNestedField(app.Object, `Deployment "podinfo" failed to apply`, "status", "operationState", "message")
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{
			"group": "apps", "version": "v1", "kind": "Deployment", "name": "podinfo", "namespace": "web",
			"health": map[string]any{"status": "Degraded"},
		},
	}, "status", "resources")

	if _, found := issueAbout(Detail(app).Issues, "Deployment/podinfo"); found {
		t.Fatal("the resource the operation already named appeared a second time")
	}
}

func TestTheSameBrokenResourceIsCountedOnce(t *testing.T) {
	app := detailed()
	entry := map[string]any{
		"group": "apps", "version": "v1", "kind": "Deployment", "name": "podinfo", "namespace": "web",
		"health": map[string]any{"status": "Degraded"},
	}
	_ = unstructured.SetNestedSlice(app.Object, []any{entry, entry}, "status", "resources")

	count := 0
	for _, one := range Detail(app).Issues {
		if one.Subject == "Deployment/podinfo" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the same broken deployment raised %d issues, want 1", count)
	}
}

func TestHealthyResourcesRaiseNoIssue(t *testing.T) {
	if _, found := issueAbout(Detail(detailed()).Issues, "Deployment/podinfo"); found {
		t.Fatal("a healthy deployment raised an issue")
	}
}

func TestAnApplicationBeingDeletedNamesTheFinalizersHoldingIt(t *testing.T) {
	app := detailed()
	now := metav1.Now()
	app.SetDeletionTimestamp(&now)
	app.SetFinalizers([]string{"resources-finalizer.argocd.argoproj.io"})

	issue, found := issueTitled(Detail(app).Issues, "This application is being deleted")

	if !found {
		t.Fatal("an application being deleted raised no issue")
	}
	if !strings.Contains(issue.Detail, "resources-finalizer.argocd.argoproj.io") {
		t.Fatalf("detail = %q, want the finalizer named", issue.Detail)
	}
}

func TestAnApplicationBeingDeletedWithNoFinalizersSaysSo(t *testing.T) {
	app := detailed()
	now := metav1.Now()
	app.SetDeletionTimestamp(&now)
	app.SetFinalizers(nil)

	issue, _ := issueTitled(Detail(app).Issues, "This application is being deleted")

	if !strings.Contains(issue.Detail, "no finalizers left") {
		t.Fatalf("detail = %q", issue.Detail)
	}
}

// the words this page uses are the words the issues queue uses

func TestAFailedOperationIsAsFatalHereAsItIsInTheQueue(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")

	issue, _ := issueAbout(Detail(app).Issues, "operation")

	if issue.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want %q so one failure reads the same on both surfaces", issue.Severity, api.SeverityFatal)
	}
}

func TestEverySeverityComesFromTheSharedVocabulary(t *testing.T) {
	known := map[string]bool{
		api.SeverityFatal:    true,
		api.SeverityDegraded: true,
		api.SeverityWarning:  true,
		api.SeverityInfo:     true,
	}
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")
	_ = unstructured.SetNestedField(app.Object, outOfSync, "status", "sync", "status")
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"type": "ComparisonError", "message": "nope"},
		map[string]any{"type": "OrphanedResourceWarning", "message": "stray"},
		map[string]any{"type": "SomethingElse", "message": "who knows"},
	}, "status", "conditions")
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"kind": "Service", "name": "api", "health": map[string]any{"status": "Missing"}},
	}, "status", "resources")

	issues := Detail(app).Issues

	if len(issues) < 4 {
		t.Fatalf("issues = %+v, want the whole spread", issues)
	}
	for _, one := range issues {
		if !known[one.Severity] {
			t.Fatalf("severity = %q on %q, want one the queue also uses", one.Severity, one.Title)
		}
	}
}
