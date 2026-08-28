package issues

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func autoscalerDescriptor() api.ResourceDescriptor {
	return descriptor(autoscalingGroup, "v2", "horizontalpodautoscalers", autoscalerKind)
}

func autoscaler(name string, spec, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "autoscaling/v2",
		"kind":       autoscalerKind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "default",
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Format(time.RFC3339),
		},
		"spec":   spec,
		"status": status,
	}}
}

func autoscalerItems(objs ...*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	return map[string][]*unstructured.Unstructured{"horizontalpodautoscalers": objs}
}

func TestAnAutoscalerThatCannotReadMetricsIsDegraded(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(2), "maxReplicas": int64(10)}, map[string]any{
		"currentReplicas": int64(2),
		"conditions": []any{condition("ScalingActive", "False", map[string]any{
			"reason":  "FailedGetResourceMetric",
			"message": "no metrics returned from resource metrics API",
		})},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	row, ok := rowNamed(build(t, lister, catalog(autoscalerDescriptor())), "web")
	if !ok || row.Title != "FailedGetResourceMetric" || row.Severity != api.SeverityDegraded {
		t.Fatalf("row = %+v, want a degraded metrics row", row)
	}
}

func TestScalingDisabledIsNotReportedAsAMetricsProblem(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(1), "maxReplicas": int64(5)}, map[string]any{
		"conditions": []any{condition("ScalingActive", "False", map[string]any{"reason": "ScalingDisabled"})},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	if queue := build(t, lister, catalog(autoscalerDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want a deliberately disabled autoscaler left alone", queue.Rows)
	}
}

func TestAnAutoscalerThatCannotScaleIsDegraded(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(1), "maxReplicas": int64(5)}, map[string]any{
		"conditions": []any{condition("AbleToScale", "False", map[string]any{
			"reason": "FailedGetScale",
		})},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	row, _ := rowNamed(build(t, lister, catalog(autoscalerDescriptor())), "web")
	if row.Title != "FailedGetScale" || row.Severity != api.SeverityDegraded {
		t.Fatalf("row = %+v, want the scale target problem", row)
	}
}

func TestAnAbleToScaleConditionWithoutAReasonStillReports(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(1), "maxReplicas": int64(5)}, map[string]any{
		"conditions": []any{condition("AbleToScale", "False", nil)},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	row, _ := rowNamed(build(t, lister, catalog(autoscalerDescriptor())), "web")
	if row.Title != "CannotScale" || !contains(row.Detail, "not able to change") {
		t.Fatalf("row = %+v, want the fallbacks", row)
	}
}

func TestAPinnedAutoscalerIsAWarning(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(3), "maxReplicas": int64(3)}, map[string]any{})
	lister := &stubLister{items: autoscalerItems(obj)}

	row, _ := rowNamed(build(t, lister, catalog(autoscalerDescriptor())), "web")
	if row.Title != "Pinned" || row.Severity != api.SeverityWarning {
		t.Fatalf("row = %+v, want a pinned warning", row)
	}
	if !contains(row.Detail, "both 3") {
		t.Fatalf("detail = %q, want the replica count", row.Detail)
	}
}

func TestAnAutoscalerAtItsMaximumIsAWarning(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(2), "maxReplicas": int64(10)}, map[string]any{
		"currentReplicas": int64(10),
		"conditions": []any{condition("ScalingLimited", "True", map[string]any{
			"reason": "TooManyReplicas",
		})},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	row, _ := rowNamed(build(t, lister, catalog(autoscalerDescriptor())), "web")
	if row.Title != "AtMaximum" || !contains(row.Detail, "maxReplicas 10") {
		t.Fatalf("row = %+v, want the maximum warning", row)
	}
}

func TestAnAutoscalerAtItsMaximumWithoutTheLimitConditionIsQuiet(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(2), "maxReplicas": int64(10)}, map[string]any{
		"currentReplicas": int64(10),
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	if queue := build(t, lister, catalog(autoscalerDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none while it is merely at the top", queue.Rows)
	}
}

func TestAnAutoscalerBelowItsMaximumIsNotAnIssue(t *testing.T) {
	obj := autoscaler("web", map[string]any{"minReplicas": int64(2), "maxReplicas": int64(10)}, map[string]any{
		"currentReplicas": int64(4),
		"conditions":      []any{condition("ScalingActive", "True", nil)},
	})
	lister := &stubLister{items: autoscalerItems(obj)}

	if queue := build(t, lister, catalog(autoscalerDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestAnAutoscalerWithoutAMaximumIsNotJudged(t *testing.T) {
	obj := autoscaler("web", map[string]any{}, map[string]any{"currentReplicas": int64(4)})
	lister := &stubLister{items: autoscalerItems(obj)}

	if queue := build(t, lister, catalog(autoscalerDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}
