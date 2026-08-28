package issues

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

func certificateDescriptor() api.ResourceDescriptor {
	return descriptor("cert-manager.io", "v1", "certificates", "Certificate")
}

func custom(kind, name string, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "default",
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Format(time.RFC3339),
		},
		"status": status,
	}}
}

func cachedItems(objs ...*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	return map[string][]*unstructured.Unstructured{"certificates": objs}
}

func TestACustomResourceThatIsNotReadyIsReported(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "False", map[string]any{
			"reason":  "Failed",
			"message": "the order failed: DNS01 challenge timed out",
		})},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	row, ok := rowNamed(build(t, lister, catalog()), "wildcard")
	if !ok || row.Detector != detectorCondition || row.Severity != api.SeverityDegraded {
		t.Fatalf("row = %+v, want a degraded condition row", row)
	}
	if !contains(row.Detail, "DNS01 challenge") {
		t.Fatalf("detail = %q, want the condition message", row.Detail)
	}
}

func TestAnUnknownConditionIsMarkedUncertain(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "Unknown", nil)},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	row, _ := rowNamed(build(t, lister, catalog()), "wildcard")
	if !row.Uncertain {
		t.Fatalf("row = %+v, want the uncertainty stated", row)
	}
	if !contains(row.Detail, "Ready=Unknown") {
		t.Fatalf("detail = %q, want the condition spelled out", row.Detail)
	}
}

func TestAReadyCustomResourceIsNotAnIssue(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "True", nil)},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestANegativeConditionThatIsTrueIsReported(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Degraded", "True", map[string]any{"reason": "Backlogged"})},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	row, _ := rowNamed(build(t, lister, catalog()), "wildcard")
	if row.Title != "Backlogged" {
		t.Fatalf("row = %+v, want the negative condition reported", row)
	}
}

func TestANegativeConditionThatIsFalseIsQuiet(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Degraded", "False", nil)},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAnObjectWithoutConditionsIsQuiet(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestATerminatingCustomResourceIsQuiet(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "False", nil)},
	})
	stamp := metaNow()
	obj.SetDeletionTimestamp(&stamp)
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestKindsWithTheirOwnDetectorAreLeftToIt(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "False", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: deploymentItems(deployment)}

	queue := build(t, lister, catalog(deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want one row from the workload detector alone", queue.Rows)
	}
	if queue.Rows[0].Detector != detectorWorkload {
		t.Fatalf("detector = %q, want the workload detector", queue.Rows[0].Detector)
	}
}

func TestACachedTypeAlreadyCollectedIsNotReadTwice(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{
		items:  deploymentItems(deployment),
		cached: []api.ResourceDescriptor{deploymentDescriptor()},
	}

	queue := build(t, lister, catalog(deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want the deployment counted once", queue.Rows)
	}
	if queue.Rows[0].Folded != 0 {
		t.Fatalf("folded = %d, want nothing folded under a single object", queue.Rows[0].Folded)
	}
}

func TestADuplicateCachedTypeIsListedOnce(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "False", nil)},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor(), certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want one row", queue.Rows)
	}
}

func TestACustomKindThatOnlySharesANameIsStillJudged(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch.volcano.sh/v1alpha1",
		"kind":       kindJob,
		"metadata": map[string]any{
			"name":              "train",
			"namespace":         "default",
			"uid":               "uid-train",
			"creationTimestamp": testNow.Format(time.RFC3339),
		},
		"status": map[string]any{
			"conditions": []any{condition("Ready", "False", map[string]any{"reason": "Unschedulable"})},
		},
	}}
	desc := descriptor("batch.volcano.sh", "v1alpha1", "jobs", kindJob)
	lister := &stubLister{
		items:  map[string][]*unstructured.Unstructured{"jobs": {obj}},
		cached: []api.ResourceDescriptor{desc},
	}

	queue := build(t, lister, catalog())

	row, ok := rowNamed(queue, "train")
	if !ok || row.Detector != detectorCondition {
		t.Fatalf("rows = %+v, want a custom Job judged by its conditions", queue.Rows)
	}
}

func TestWhichConditionsCountAsBroken(t *testing.T) {
	cases := []struct {
		name       string
		conditions []any
		want       string
	}{
		{name: "no status at all", conditions: nil, want: ""},
		{name: "ready", conditions: []any{condition("Ready", "True", nil)}, want: ""},
		{name: "not ready", conditions: []any{condition("Ready", "False", nil)}, want: "Ready"},
		{name: "ready unknown", conditions: []any{condition("Ready", "Unknown", nil)}, want: "Ready"},
		{name: "available is read when ready is absent", conditions: []any{condition("Available", "False", nil)}, want: "Available"},
		{name: "healthy is read when the two above are absent", conditions: []any{condition("Healthy", "False", nil)}, want: "Healthy"},
		{name: "synced is read last of the positives", conditions: []any{condition("Synced", "False", nil)}, want: "Synced"},
		{
			name:       "a ready object is not judged on a later positive",
			conditions: []any{condition("Ready", "True", nil), condition("Synced", "False", nil)},
			want:       "",
		},
		{name: "stalled", conditions: []any{condition("Stalled", "True", nil)}, want: "Stalled"},
		{name: "not stalled", conditions: []any{condition("Stalled", "False", nil)}, want: ""},
		{name: "degraded", conditions: []any{condition("Degraded", "True", nil)}, want: "Degraded"},
		{name: "failed", conditions: []any{condition("Failed", "True", nil)}, want: "Failed"},
		{
			name:       "a vocabulary this fallback does not know",
			conditions: []any{condition("Established", "False", nil)},
			want:       "",
		},
		{
			name:       "noise among the conditions",
			conditions: []any{"not a condition", condition("Ready", "False", nil)},
			want:       "Ready",
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			obj := custom("Certificate", "wildcard", map[string]any{"conditions": item.conditions})
			entry, ok := badCondition(obj)
			if item.want == "" {
				if ok {
					t.Fatalf("condition = %v, want the object judged healthy", entry)
				}
				return
			}
			if !ok {
				t.Fatalf("condition = none, want %q reported", item.want)
			}
			if got := unstr.At(entry, "type"); got != item.want {
				t.Fatalf("condition = %q, want %q", got, item.want)
			}
		})
	}
}
