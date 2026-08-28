package argocd

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func detailed() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":              "podinfo",
			"namespace":         "argocd",
			"creationTimestamp": "2026-08-01T10:00:00Z",
		},
		"spec": map[string]any{
			"project": "default",
			"source": map[string]any{
				"repoURL":        "https://example.test/apps",
				"path":           "podinfo",
				"targetRevision": "main",
			},
			"destination": map[string]any{"server": "https://kubernetes.default.svc", "namespace": "web"},
			"syncPolicy": map[string]any{
				"automated": map[string]any{"prune": true, "selfHeal": true},
			},
		},
		"status": map[string]any{
			"sync":   map[string]any{"status": "Synced", "revision": "abc1234"},
			"health": map[string]any{"status": "Healthy"},
			"resources": []any{
				map[string]any{
					"group":     "apps",
					"version":   "v1",
					"kind":      "Deployment",
					"name":      "podinfo",
					"namespace": "web",
					"status":    "Synced",
					"health":    map[string]any{"status": "Healthy"},
				},
			},
			"history": []any{
				map[string]any{
					"id":              int64(0),
					"revision":        "abc1234",
					"deployStartedAt": "2026-08-02T10:00:00Z",
					"deployedAt":      "2026-08-02T10:00:05Z",
					"initiatedBy":     map[string]any{"automated": true},
					"source":          map[string]any{"path": "podinfo"},
				},
			},
			"operationState": map[string]any{
				"phase":      "Succeeded",
				"message":    "successfully synced",
				"startedAt":  "2026-08-02T10:00:00Z",
				"finishedAt": "2026-08-02T10:00:05Z",
				"operation": map[string]any{
					"initiatedBy": map[string]any{"username": "spinoza"},
					"sync":        map[string]any{"revision": "abc1234"},
				},
			},
		},
	}}
}

func TestDetailSplitsConfigFromDeploymentState(t *testing.T) {
	got := Detail(detailed())

	if got.Controller != api.ControllerArgo {
		t.Fatalf("controller = %q, want argocd", got.Controller)
	}
	if got.Source.Repo != "https://example.test/apps" || got.Source.Path != "podinfo" {
		t.Fatalf("source = %+v, want the repo and path from the spec", got.Source)
	}
	if got.Source.Target != "main" {
		t.Fatalf("target = %q, want main", got.Source.Target)
	}
	if got.Source.Destination != "https://kubernetes.default.svc web" {
		t.Fatalf("destination = %q", got.Source.Destination)
	}
	if got.State.Sync != "Synced" || got.State.Health != "Healthy" {
		t.Fatalf("state = %+v, want the controller's own verdict", got.State)
	}
	if got.State.Revision != "abc1234" {
		t.Fatalf("revision = %q, want abc1234", got.State.Revision)
	}
	if got.State.SyncedAt != "2026-08-02T10:00:05Z" {
		t.Fatalf("syncedAt = %q, want the finish of the last operation", got.State.SyncedAt)
	}
}

func TestDetailReadsTheAutomationPolicy(t *testing.T) {
	got := Detail(detailed())

	if got.Source.SyncMode != api.SyncModeAuto {
		t.Fatalf("syncMode = %q, want auto", got.Source.SyncMode)
	}
	if got.Source.Policy != "prune, self-heal" {
		t.Fatalf("policy = %q, want both flags named", got.Source.Policy)
	}
}

func TestDetailCallsAPausedApplicationSuspended(t *testing.T) {
	app := automating(detailed(), map[string]any{"enabled": false, "prune": true})

	got := Detail(app)

	if got.Source.SyncMode != api.SyncModeSuspended {
		t.Fatalf("syncMode = %q, want suspended", got.Source.SyncMode)
	}
	if got.Source.Policy != "prune" {
		t.Fatalf("policy = %q, want prune kept", got.Source.Policy)
	}
}

func TestDetailCallsAnApplicationWithNoAutomationManual(t *testing.T) {
	got := Detail(newApplication())

	if got.Source.SyncMode != api.SyncModeManual {
		t.Fatalf("syncMode = %q, want manual", got.Source.SyncMode)
	}
	if got.Source.Policy != "" {
		t.Fatalf("policy = %q, want none", got.Source.Policy)
	}
}

func TestDetailSaysWhenAutomationHasNeitherFlag(t *testing.T) {
	got := Detail(automating(detailed(), map[string]any{}))

	if got.Source.Policy != "neither prune nor self-heal" {
		t.Fatalf("policy = %q", got.Source.Policy)
	}
}

func TestDetailListsTheManagedResources(t *testing.T) {
	got := Detail(detailed())

	if len(got.Resources) != 1 {
		t.Fatalf("resources = %+v, want the one the controller named", got.Resources)
	}
	one := got.Resources[0]
	if one.Kind != "Deployment" || one.Name != "podinfo" || one.Namespace != "web" {
		t.Fatalf("resource = %+v", one)
	}
	if one.Sync != "Synced" || one.Health != "Healthy" {
		t.Fatalf("resource state = %+v", one)
	}
}

func TestDetailSkipsResourceEntriesWithNoKindOrName(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"kind": "Deployment"},
		map[string]any{"name": "web"},
		"not a map",
	}, "status", "resources")

	if got := Detail(app); len(got.Resources) != 0 {
		t.Fatalf("resources = %+v, want none of the half-written entries", got.Resources)
	}
}

func TestDetailReadsWhoDeployedEachRevision(t *testing.T) {
	got := Detail(detailed())

	if len(got.History) != 1 {
		t.Fatalf("history = %+v, want one entry", got.History)
	}
	entry := got.History[0]
	if entry.ID != 0 || entry.Revision != "abc1234" {
		t.Fatalf("entry = %+v", entry)
	}
	if !entry.Automated {
		t.Fatal("the entry was deployed by automation and does not say so")
	}
	if entry.Source != "podinfo" {
		t.Fatalf("source = %q, want the path that entry recorded", entry.Source)
	}
	if entry.DeployedAt != "2026-08-02T10:00:05Z" {
		t.Fatalf("deployedAt = %q", entry.DeployedAt)
	}
}

func TestDetailSkipsHistoryEntriesWithNoID(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"revision": "abc"},
		"not a map",
	}, "status", "history")

	if got := Detail(app); len(got.History) != 0 {
		t.Fatalf("history = %+v, want nothing without an id", got.History)
	}
}

func TestDetailReportsTheLastOperation(t *testing.T) {
	got := Detail(detailed())

	if got.Operation == nil {
		t.Fatal("no operation was reported")
	}
	if got.Operation.Phase != "Succeeded" || got.Operation.Running {
		t.Fatalf("operation = %+v, want a finished one", got.Operation)
	}
	if got.Operation.InitiatedBy != "spinoza" {
		t.Fatalf("initiatedBy = %q, want spinoza", got.Operation.InitiatedBy)
	}
	if got.Operation.Revision != "abc1234" {
		t.Fatalf("revision = %q", got.Operation.Revision)
	}
}

func TestDetailPinsAnOperationInFlight(t *testing.T) {
	got := Detail(operating(detailed(), runningPhase))

	if got.Operation == nil || !got.Operation.Running {
		t.Fatalf("operation = %+v, want it marked as running", got.Operation)
	}
}

func TestDetailTreatsTerminatingAsStillRunning(t *testing.T) {
	got := Detail(operating(detailed(), terminatingPhase))

	if got.Operation == nil || !got.Operation.Running {
		t.Fatalf("operation = %+v, want it marked as running", got.Operation)
	}
}

func TestDetailReportsNoOperationOnAnApplicationThatNeverSynced(t *testing.T) {
	if got := Detail(newApplication()); got.Operation != nil {
		t.Fatalf("operation = %+v, want nothing", got.Operation)
	}
}

func TestDetailStatesTheCauseOfAFailedOperation(t *testing.T) {
	app := detailed()
	_ = unstructured.SetNestedField(app.Object, "Failed", "status", "operationState", "phase")
	_ = unstructured.SetNestedField(app.Object,
		`one or more objects failed to apply, reason: namespaces "web" not found`,
		"status", "operationState", "message")

	got := Detail(app)

	want := "the destination namespace web does not exist; add CreateNamespace=true or create it in git"
	if got.Operation.Cause != want {
		t.Fatalf("cause = %q, want %q", got.Operation.Cause, want)
	}
}

func TestDetailFallsBackToTheReconcileTimeWhenNothingSyncedYet(t *testing.T) {
	app := newApplication()
	_ = unstructured.SetNestedField(app.Object, "2026-08-03T09:00:00Z", "status", "reconciledAt")

	if got := Detail(app); got.State.SyncedAt != "2026-08-03T09:00:00Z" {
		t.Fatalf("syncedAt = %q, want the reconcile time", got.State.SyncedAt)
	}
}

func TestDetailReadsAChartSourceAsThePath(t *testing.T) {
	app := newApplication()
	_ = unstructured.SetNestedMap(app.Object, map[string]any{
		"repoURL": "https://charts.example.test",
		"chart":   "podinfo",
	}, "spec", "source")

	if got := Detail(app); got.Source.Path != "podinfo" {
		t.Fatalf("path = %q, want the chart name", got.Source.Path)
	}
}

func TestDetailCopesWithStatusEntriesThatCarryNoNestedMaps(t *testing.T) {
	app := newApplication()
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"kind": "Deployment", "name": "web"},
	}, "status", "resources")
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"id": int64(1), "revision": "abc"},
	}, "status", "history")
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"phase": "Succeeded"}, "status", "operationState")

	got := Detail(app)

	if got.Resources[0].Health != "" || got.Resources[0].Message != "" {
		t.Fatalf("resource = %+v, want no invented health", got.Resources[0])
	}
	if got.History[0].InitiatedBy != "" || got.History[0].Automated {
		t.Fatalf("history = %+v, want no invented initiator", got.History[0])
	}
	if got.History[0].Source != "" {
		t.Fatalf("source = %q, want nothing", got.History[0].Source)
	}
	if got.Operation.InitiatedBy != "" || got.Operation.Revision != "" {
		t.Fatalf("operation = %+v, want no invented operation detail", got.Operation)
	}
}

func TestDetailReadsNoRevisionFromAnOperationWithoutASync(t *testing.T) {
	app := newApplication()
	_ = unstructured.SetNestedMap(app.Object, map[string]any{
		"phase":     "Succeeded",
		"operation": map[string]any{"initiatedBy": map[string]any{"username": "someone"}},
	}, "status", "operationState")

	got := Detail(app)

	if got.Operation.Revision != "" {
		t.Fatalf("revision = %q, want nothing", got.Operation.Revision)
	}
	if got.Operation.InitiatedBy != "someone" {
		t.Fatalf("initiatedBy = %q", got.Operation.InitiatedBy)
	}
}

func TestDetailIgnoresAnOperationStateWithNoPhase(t *testing.T) {
	app := newApplication()
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"message": "nothing yet"}, "status", "operationState")

	if got := Detail(app); got.Operation != nil {
		t.Fatalf("operation = %+v, want nothing without a phase", got.Operation)
	}
}
