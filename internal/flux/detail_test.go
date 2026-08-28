package flux

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func appliedKustomization() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":              "apps",
			"namespace":         "flux-system",
			"creationTimestamp": "2026-08-01T10:00:00Z",
		},
		"spec": map[string]any{
			"path":      "./apps/production",
			"prune":     true,
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		},
		"status": map[string]any{
			"lastAppliedRevision":    "main@sha1:abc1234",
			"lastHandledReconcileAt": "2026-08-02T10:00:00Z",
			"conditions": []any{
				map[string]any{
					"type":               "Ready",
					"status":             "True",
					"reason":             "ReconciliationSucceeded",
					"message":            "Applied revision: main@sha1:abc1234",
					"lastTransitionTime": "2026-08-02T10:00:05Z",
				},
			},
			"inventory": map[string]any{
				"entries": []any{
					map[string]any{"id": "web_podinfo_apps_Deployment", "v": "v1"},
					map[string]any{"id": "web_podinfo__Service", "v": "v1"},
					map[string]any{"id": "broken", "v": "v1"},
					"not a map",
				},
			},
		},
	}}
}

func TestAppliesRecognisesTheTwoAppliers(t *testing.T) {
	if !Applies(kustomizationDesc()) {
		t.Fatal("a kustomization is an applier")
	}
	if !Applies(helmReleaseDesc()) {
		t.Fatal("a helmrelease is an applier")
	}
	if Applies(api.ResourceDescriptor{Group: "source.toolkit.fluxcd.io", Resource: "gitrepositories"}) {
		t.Fatal("a source is not an applier")
	}
}

func TestKustomizationDetailSplitsConfigFromState(t *testing.T) {
	got := Detail(appliedKustomization(), kustomizationDesc())

	if got.Controller != api.ControllerFlux {
		t.Fatalf("controller = %q, want flux", got.Controller)
	}
	if got.Source.Repo != "GitRepository/flux-system" {
		t.Fatalf("repo = %q, want the sourceRef until it is resolved", got.Source.Repo)
	}
	if got.Source.Path != "./apps/production" {
		t.Fatalf("path = %q", got.Source.Path)
	}
	if got.Source.Destination != "flux-system" {
		t.Fatalf("destination = %q, want the object's own namespace", got.Source.Destination)
	}
	if got.Source.SyncMode != api.SyncModeAuto || got.Source.Policy != "prune" {
		t.Fatalf("source = %+v, want auto with prune", got.Source)
	}
	if got.State.Sync != "Synced" || got.State.Health != "Healthy" {
		t.Fatalf("state = %+v", got.State)
	}
	if got.State.Revision != "main@sha1:abc1234" {
		t.Fatalf("revision = %q", got.State.Revision)
	}
}

func TestKustomizationDetailReadsTheInventory(t *testing.T) {
	got := Detail(appliedKustomization(), kustomizationDesc())

	if len(got.Resources) != 2 {
		t.Fatalf("resources = %+v, want the two readable inventory entries", got.Resources)
	}
	first := got.Resources[0]
	if first.Namespace != "web" || first.Name != "podinfo" || first.Group != "apps" || first.Kind != "Deployment" {
		t.Fatalf("first = %+v", first)
	}
	second := got.Resources[1]
	if second.Group != "" || second.Kind != "Service" {
		t.Fatalf("second = %+v, want the core group left empty", second)
	}
}

func TestASuspendedKustomizationSaysNothingWillReconcileIt(t *testing.T) {
	obj := appliedKustomization()
	_ = unstructured.SetNestedField(obj.Object, true, "spec", "suspend")

	got := Detail(obj, kustomizationDesc())

	if got.Source.SyncMode != api.SyncModeSuspended {
		t.Fatalf("syncMode = %q, want suspended", got.Source.SyncMode)
	}
	found := false
	for _, one := range got.Issues {
		if one.Title == "Nothing will reconcile this" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %+v, want one saying it is suspended", got.Issues)
	}
}

func TestAFailingKustomizationBecomesAnIssueWithItsCause(t *testing.T) {
	obj := appliedKustomization()
	_ = unstructured.SetNestedSlice(obj.Object, []any{
		map[string]any{
			"type":    "Ready",
			"status":  "False",
			"reason":  "BuildFailed",
			"message": `namespaces "shop" not found`,
		},
	}, "status", "conditions")

	got := Detail(obj, kustomizationDesc())

	if len(got.Issues) != 1 {
		t.Fatalf("issues = %+v, want one", got.Issues)
	}
	if got.Issues[0].Title != "BuildFailed" {
		t.Fatalf("title = %q, want the reason", got.Issues[0].Title)
	}
	if !strings.HasPrefix(got.Issues[0].Detail, "the destination namespace shop does not exist") {
		t.Fatalf("detail = %q, want the cause first", got.Issues[0].Detail)
	}
	if got.State.Sync != "OutOfSync" || got.State.Health != "Degraded" {
		t.Fatalf("state = %+v, want it read as failing", got.State)
	}
}

func TestAFailureWithNoKnownCauseKeepsTheMessage(t *testing.T) {
	obj := appliedKustomization()
	_ = unstructured.SetNestedSlice(obj.Object, []any{
		map[string]any{"type": "Ready", "status": "False", "reason": "Odd", "message": "something odd"},
	}, "status", "conditions")

	got := Detail(obj, kustomizationDesc())

	if got.Issues[0].Detail != "something odd" {
		t.Fatalf("detail = %q", got.Issues[0].Detail)
	}
}

func TestAStalledKustomizationIsAnIssue(t *testing.T) {
	obj := appliedKustomization()
	_ = unstructured.SetNestedSlice(obj.Object, []any{
		map[string]any{"type": "Ready", "status": "True", "reason": "Fine", "message": "ok"},
		map[string]any{"type": "Stalled", "status": "True", "reason": "RetriesExhausted", "message": "gave up"},
	}, "status", "conditions")

	got := Detail(obj, kustomizationDesc())

	if len(got.Issues) != 1 || got.Issues[0].Title != "RetriesExhausted" {
		t.Fatalf("issues = %+v, want the stall reported", got.Issues)
	}
}

func TestAnObjectWithNoConditionsReadsAsUnknown(t *testing.T) {
	obj := appliedKustomization()
	unstructured.RemoveNestedField(obj.Object, "status", "conditions")

	got := Detail(obj, kustomizationDesc())

	if got.State.Sync != "Unknown" || got.State.Health != "Progressing" {
		t.Fatalf("state = %+v, want it read as not yet known", got.State)
	}
	if got.Operation != nil {
		t.Fatalf("operation = %+v, want nothing to report", got.Operation)
	}
}

func TestAReconcilingKustomizationPinsARunningOperation(t *testing.T) {
	obj := appliedKustomization()
	_ = unstructured.SetNestedSlice(obj.Object, []any{
		map[string]any{"type": "Ready", "status": "Unknown", "reason": "Progressing", "message": "building"},
		map[string]any{"type": "Reconciling", "status": "True", "message": "Reconciliation in progress"},
	}, "status", "conditions")
	_ = unstructured.SetNestedField(obj.Object, "main@sha1:def5678", "status", "lastAttemptedRevision")

	got := Detail(obj, kustomizationDesc())

	if got.Operation == nil || !got.Operation.Running {
		t.Fatalf("operation = %+v, want it running", got.Operation)
	}
	if got.Operation.Revision != "main@sha1:def5678" {
		t.Fatalf("revision = %q, want the one being attempted", got.Operation.Revision)
	}
	if got.Operation.Message != "Reconciliation in progress" {
		t.Fatalf("message = %q", got.Operation.Message)
	}
}

func TestASettledKustomizationReportsItsLastReconciliation(t *testing.T) {
	got := Detail(appliedKustomization(), kustomizationDesc())

	if got.Operation == nil || got.Operation.Phase != "Succeeded" {
		t.Fatalf("operation = %+v, want a finished one", got.Operation)
	}
	if got.Operation.FinishedAt != "2026-08-02T10:00:05Z" {
		t.Fatalf("finishedAt = %q, want the ready transition", got.Operation.FinishedAt)
	}
	if got.Operation.Revision != "main@sha1:abc1234" {
		t.Fatalf("revision = %q, want the applied one when none was attempted", got.Operation.Revision)
	}
}

func TestAKustomizationBeingDeletedNamesItsFinalizers(t *testing.T) {
	obj := appliedKustomization()
	now := metav1.Now()
	obj.SetDeletionTimestamp(&now)
	obj.SetFinalizers([]string{"finalizers.fluxcd.io"})

	got := Detail(obj, kustomizationDesc())

	if !strings.Contains(got.Issues[0].Detail, "finalizers.fluxcd.io") {
		t.Fatalf("detail = %q, want the finalizer named", got.Issues[0].Detail)
	}
}

func TestAKustomizationBeingDeletedWithNoFinalizersSaysSo(t *testing.T) {
	obj := appliedKustomization()
	now := metav1.Now()
	obj.SetDeletionTimestamp(&now)

	got := Detail(obj, kustomizationDesc())

	if !strings.Contains(got.Issues[0].Detail, "no finalizers left") {
		t.Fatalf("detail = %q", got.Issues[0].Detail)
	}
}

func TestHelmReleaseDetailReadsTheChartAndItsHistory(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata": map[string]any{
			"name":              "podinfo",
			"namespace":         "flux-system",
			"creationTimestamp": "2026-08-01T10:00:00Z",
		},
		"spec": map[string]any{
			"targetNamespace": "web",
			"force":           true,
			"chart": map[string]any{
				"spec": map[string]any{
					"chart":     "podinfo",
					"sourceRef": map[string]any{"kind": "HelmRepository", "name": "podinfo"},
				},
			},
		},
		"status": map[string]any{
			"history": []any{
				map[string]any{
					"version":       int64(3),
					"chartName":     "podinfo",
					"chartVersion":  "6.7.0",
					"firstDeployed": "2026-08-01T11:00:00Z",
					"lastDeployed":  "2026-08-02T11:00:00Z",
				},
				map[string]any{"chartVersion": "6.6.0"},
				"not a map",
			},
		},
	}}

	got := Detail(obj, helmReleaseDesc())

	if got.Source.Path != "podinfo" {
		t.Fatalf("path = %q, want the chart name", got.Source.Path)
	}
	if got.Source.Repo != "HelmRepository/podinfo" {
		t.Fatalf("repo = %q", got.Source.Repo)
	}
	if got.Source.Destination != "web" {
		t.Fatalf("destination = %q, want the target namespace", got.Source.Destination)
	}
	if got.Source.Policy != "force" {
		t.Fatalf("policy = %q", got.Source.Policy)
	}
	if len(got.History) != 1 {
		t.Fatalf("history = %+v, want the one entry with a version", got.History)
	}
	if got.History[0].ID != 3 || got.History[0].Revision != "6.7.0" {
		t.Fatalf("entry = %+v", got.History[0])
	}
}

func TestHelmReleaseDetailFallsBackToTheChartRef(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "podinfo", "namespace": "flux-system"},
		"spec": map[string]any{
			"chartRef": map[string]any{"kind": "OCIRepository", "name": "podinfo-oci"},
		},
	}}

	got := Detail(obj, helmReleaseDesc())

	if got.Source.Path != "podinfo-oci" {
		t.Fatalf("path = %q, want the chartRef name", got.Source.Path)
	}
	if got.Source.Repo != "OCIRepository/podinfo-oci" {
		t.Fatalf("repo = %q", got.Source.Repo)
	}
}
