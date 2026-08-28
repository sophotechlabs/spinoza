package issues

import (
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func kustomizationDescriptor() api.ResourceDescriptor {
	return descriptor("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization")
}

func argoDescriptor() api.ResourceDescriptor {
	return descriptor(argoGroup, "v1alpha1", "applications", applicationsKind)
}

func fluxObject(name string, status, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "flux-system",
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Format(time.RFC3339),
		},
		"spec":   spec,
		"status": status,
	}}
}

func argoObject(name string, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       applicationsKind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "argocd",
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Format(time.RFC3339),
		},
		"status": status,
	}}
}

func TestAStalledKustomizationIsFatal(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions": []any{
			condition("Ready", "False", map[string]any{"reason": "BuildFailed"}),
			condition("Stalled", "True", map[string]any{
				"reason":  "BuildFailed",
				"message": "kustomize build failed: accumulating resources",
			}),
		},
		"lastAttemptedRevision": "main@sha1:9f8e7d6c5b4a3f2e1d0c",
	}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	row, ok := rowNamed(build(t, lister, catalog(kustomizationDescriptor())), "apps")
	if !ok || row.Severity != api.SeverityFatal || row.Title != "BuildFailed" {
		t.Fatalf("row = %+v, want a fatal stalled kustomization", row)
	}
	if row.Change != "9f8e7d6" {
		t.Fatalf("change = %q, want the short revision", row.Change)
	}
}

func TestANotReadyKustomizationIsReported(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions":          []any{condition("Ready", "False", map[string]any{"message": "dependency not ready"})},
		"lastAppliedRevision": "main@sha1:aaaaaaaaaaaaaaaaaaaa",
	}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(kustomizationDescriptor())), "apps")
	if row.Title != "NotReady" || !contains(row.Detail, "dependency not ready") {
		t.Fatalf("row = %+v, want the not-ready message", row)
	}
}

func TestAnUnknownReadyStateIsDegraded(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions": []any{condition("Ready", "Unknown", map[string]any{"reason": "Progressing"})},
	}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(kustomizationDescriptor())), "apps")
	if row.Severity != api.SeverityDegraded {
		t.Fatalf("severity = %q, want degraded while it is still unknown", row.Severity)
	}
}

func TestASuspendedKustomizationIsNotAnIssue(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions": []any{condition("Ready", "False", nil)},
	}, map[string]any{"suspend": true})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	if queue := build(t, lister, catalog(kustomizationDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want a suspended object left alone", queue.Rows)
	}
}

func TestAReadyKustomizationIsNotAnIssue(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions": []any{condition("Ready", "True", nil)},
	}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	if queue := build(t, lister, catalog(kustomizationDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAKustomizationWithoutConditionsIsNotAnIssue(t *testing.T) {
	obj := fluxObject("apps", map[string]any{}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	if queue := build(t, lister, catalog(kustomizationDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAnArtifactRevisionIsUsedWhenNothingElseIsThere(t *testing.T) {
	obj := fluxObject("apps", map[string]any{
		"conditions": []any{condition("Ready", "False", nil)},
		"artifact":   map[string]any{"revision": "v1.2.3"},
	}, map[string]any{})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"kustomizations": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(kustomizationDescriptor())), "apps")
	if row.Change != "v1.2.3" {
		t.Fatalf("change = %q, want the artifact revision", row.Change)
	}
}

func TestAFailedArgoSyncIsFatal(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"operationState": map[string]any{
			"phase":      "Failed",
			"message":    "one or more objects failed to apply",
			"finishedAt": testNow.Format(time.RFC3339),
		},
		"sync": map[string]any{"revision": "1a2b3c4d5e6f7a8b9c0d", "status": "OutOfSync"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")
	if row.Title != "SyncFailed" || row.Severity != api.SeverityFatal {
		t.Fatalf("row = %+v, want a fatal sync failure", row)
	}
	if row.Change != "1a2b3c4" {
		t.Fatalf("change = %q, want the short revision", row.Change)
	}
}

func TestADegradedArgoApplicationIsFatal(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"health": map[string]any{"status": "Degraded", "message": "Deployment web is degraded"},
		"sync":   map[string]any{"status": "Synced"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")
	if row.Title != "Degraded" || !contains(row.Detail, "is degraded") {
		t.Fatalf("row = %+v, want the health message", row)
	}
}

func TestAMissingArgoApplicationUsesTheFallbackDetail(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"health": map[string]any{"status": "Missing"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")
	if !contains(row.Detail, "argo reports the application as Missing") {
		t.Fatalf("detail = %q, want the fallback", row.Detail)
	}
}

func TestAnOutOfSyncApplicationIsOnlyAWarning(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"health": map[string]any{"status": "Healthy"},
		"sync":   map[string]any{"status": "OutOfSync"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")
	if row.Severity != api.SeverityWarning || row.Title != "OutOfSync" {
		t.Fatalf("row = %+v, want a warning", row)
	}
}

func TestAHealthySyncedApplicationIsNotAnIssue(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"health": map[string]any{"status": "Healthy"},
		"sync":   map[string]any{"status": "Synced"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	if queue := build(t, lister, catalog(argoDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAnApplicationWithoutAFinishTimeFallsBackToItsConditions(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"health":     map[string]any{"status": "Degraded"},
		"conditions": []any{condition("Ready", "False", nil)},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")
	if row.Since != testNow.Add(-30*time.Minute).Format(time.RFC3339) {
		t.Fatalf("since = %q, want the condition transition", row.Since)
	}
}

func TestAFailedSyncStatesItsCause(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"operationState": map[string]any{
			"phase":   "Failed",
			"message": `one or more objects failed to apply, reason: namespaces "shop" not found`,
		},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, ok := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")

	if !ok {
		t.Fatal("a failed sync raised no row")
	}
	if !strings.HasPrefix(row.Detail, "the destination namespace shop does not exist") {
		t.Fatalf("detail = %q, want the cause first", row.Detail)
	}
	if !strings.HasSuffix(row.Detail, "not found") {
		t.Fatalf("detail = %q, want the raw message kept", row.Detail)
	}
}

func TestAFailedSyncWithNoKnownCauseKeepsItsMessage(t *testing.T) {
	obj := argoObject("web", map[string]any{
		"operationState": map[string]any{"phase": "Failed", "message": "something odd"},
	})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"applications": {obj}}}

	row, _ := rowNamed(build(t, lister, catalog(argoDescriptor())), "web")

	if row.Detail != "something odd" {
		t.Fatalf("detail = %q", row.Detail)
	}
}
