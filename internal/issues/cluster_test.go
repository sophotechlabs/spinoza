package issues

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func clusterObject(kind, name string, spec, status map[string]any) object {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":              name,
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Add(-time.Hour).Format(time.RFC3339),
		},
		"spec":   spec,
		"status": status,
	}}
	return object{obj: obj, desc: api.ResourceDescriptor{
		Version: "v1", Resource: kind + "s", Kind: kind,
	}}
}

func nodeWith(name string, conditions []any, unschedulable bool) object {
	spec := map[string]any{}
	if unschedulable {
		spec["unschedulable"] = true
	}
	return clusterObject(kindNode, name, spec, map[string]any{"conditions": conditions})
}

func onlyNodeFinding(t *testing.T, snap *snapshot) finding {
	t.Helper()
	found := nodeFindings(snap)
	if len(found) != 1 {
		t.Fatalf("produced %d findings, want 1", len(found))
	}
	return found[0]
}

// what a node says about itself

func TestANodeThatIsNotReadyIsFatal(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{
		condition("Ready", conditionFalse, map[string]any{
			"reason": "KubeletNotReady", "message": "container runtime is down",
		}),
	}, false))

	found := onlyNodeFinding(t, snap)

	if found.severity != severityFatal {
		t.Fatalf("severity was %d, want fatal", found.severity)
	}
	if found.title != "KubeletNotReady" {
		t.Fatalf("title was %q", found.title)
	}
	if !contains(found.detail, "container runtime is down") {
		t.Fatalf("detail was %q, want the kubelet's own message", found.detail)
	}
}

func TestANodeWhoseKubeletWentQuietIsDegradedRatherThanFatal(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{
		condition("Ready", "Unknown", nil),
	}, false))

	found := onlyNodeFinding(t, snap)

	if found.severity != severityDegraded {
		t.Fatalf("severity was %d, want degraded for an Unknown Ready", found.severity)
	}
}

func TestAReadyNodeSaysNothing(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{condition("Ready", conditionTrue, nil)}, false))

	if found := nodeFindings(snap); len(found) != 0 {
		t.Fatalf("a ready node produced %d findings", len(found))
	}
}

func TestANodeUnderPressureIsReportedByTheConditionThatIsTrue(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{
		condition("Ready", conditionTrue, nil),
		condition("DiskPressure", conditionTrue, map[string]any{"message": "no space left"}),
	}, false))

	found := onlyNodeFinding(t, snap)

	if found.title != "DiskPressure" {
		t.Fatalf("title was %q", found.title)
	}
	if found.severity != severityDegraded {
		t.Fatalf("severity was %d, want degraded", found.severity)
	}
}

func TestACordonedNodeIsAWarningAndNotAFault(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{condition("Ready", conditionTrue, nil)}, true))

	found := onlyNodeFinding(t, snap)

	if found.title != "Cordoned" {
		t.Fatalf("title was %q", found.title)
	}
	if found.severity != severityWarning {
		t.Fatalf("severity was %d, want warning", found.severity)
	}
}

func TestBeingNotReadyOutranksBeingCordoned(t *testing.T) {
	snap := snapshotOf(nodeWith("worker", []any{condition("Ready", conditionFalse, nil)}, true))

	if found := onlyNodeFinding(t, snap); found.title == "Cordoned" {
		t.Fatal("a node that is down was reported only as cordoned")
	}
}

// claims that will never bind

func TestAPendingClaimIsFatal(t *testing.T) {
	snap := snapshotOf(clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Pending"}))

	found := claimFindings(snap)

	if len(found) != 1 || found[0].title != "ClaimPending" {
		t.Fatalf("produced %v", found)
	}
	if found[0].severity != severityFatal {
		t.Fatalf("severity was %d, want fatal", found[0].severity)
	}
}

func TestALostClaimIsReported(t *testing.T) {
	snap := snapshotOf(clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Lost"}))

	if found := claimFindings(snap); len(found) != 1 || found[0].title != "ClaimLost" {
		t.Fatalf("produced %v", found)
	}
}

func TestABoundClaimSaysNothing(t *testing.T) {
	snap := snapshotOf(clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Bound"}))

	if found := claimFindings(snap); len(found) != 0 {
		t.Fatalf("a bound claim produced %d findings", len(found))
	}
}

func TestAClaimOnItsWayOutIsNotReportedAsPending(t *testing.T) {
	item := clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Pending"})
	item.obj.SetDeletionTimestamp(&metav1.Time{Time: testNow})

	if found := claimFindings(snapshotOf(item)); len(found) != 0 {
		t.Fatal("a claim being deleted was reported as pending")
	}
}

// things that were asked to go and did not

func terminating(kind, name string, ago time.Duration, grace int64, finalizers []string) object {
	spec := map[string]any{}
	if grace > 0 {
		spec["terminationGracePeriodSeconds"] = grace
	}
	item := clusterObject(kind, name, spec, nil)
	item.obj.SetDeletionTimestamp(&metav1.Time{Time: testNow.Add(-ago)})
	if len(finalizers) > 0 {
		item.obj.SetFinalizers(finalizers)
	}
	return item
}

func TestAPodStuckTerminatingIsReportedOnceItsGraceIsSpent(t *testing.T) {
	snap := snapshotOf(terminating(kindPod, "api", 20*time.Minute, 30, []string{"kubernetes.io/pvc-protection"}))

	found := terminatingFindings(snap, testNow)

	if len(found) != 1 {
		t.Fatalf("produced %d findings, want 1", len(found))
	}
	if !contains(found[0].detail, "kubernetes.io/pvc-protection") {
		t.Fatalf("detail was %q, want the finalizer named", found[0].detail)
	}
}

func TestAPodInsideItsGracePeriodIsLeftAlone(t *testing.T) {
	snap := snapshotOf(terminating(kindPod, "api", 10*time.Second, 30, nil))

	if found := terminatingFindings(snap, testNow); len(found) != 0 {
		t.Fatal("a pod still inside its grace period was reported as stuck")
	}
}

func TestALongGracePeriodIsWaitedOutBeforeReporting(t *testing.T) {
	patient := terminating(kindPod, "api", 6*time.Minute, 3600, nil)

	if found := terminatingFindings(snapshotOf(patient), testNow); len(found) != 0 {
		t.Fatal("a pod with an hour of grace was called stuck after six minutes")
	}
}

func TestANamespaceStuckTerminatingIsReported(t *testing.T) {
	snap := snapshotOf(terminating(kindNamespace, "old", 30*time.Minute, 0, nil))

	found := terminatingFindings(snap, testNow)

	if len(found) != 1 || found[0].kind != kindNamespace {
		t.Fatalf("produced %v", found)
	}
	if !contains(found[0].detail, "and still here") {
		t.Fatalf("detail was %q", found[0].detail)
	}
}

func TestSomethingNobodyAskedToGoIsNotStuck(t *testing.T) {
	snap := snapshotOf(clusterObject(kindNamespace, "live", nil, nil))

	if found := terminatingFindings(snap, testNow); len(found) != 0 {
		t.Fatal("a namespace nobody deleted was reported as terminating")
	}
}

func TestTheClusterDetectorsRunTogether(t *testing.T) {
	snap := snapshotOf(
		nodeWith("worker", []any{condition("Ready", conditionFalse, nil)}, false),
		clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Pending"}),
		terminating(kindNamespace, "old", 30*time.Minute, 0, nil),
	)

	found := clusterFindings(snap, testNow)

	if len(found) != 3 {
		t.Fatalf("produced %d findings, want one from each detector", len(found))
	}
}

func TestANodeThatReportsNoReadyConditionAtAllSaysNothing(t *testing.T) {
	snap := snapshotOf(nodeWith("joining", []any{}, false))

	if found := nodeFindings(snap); len(found) != 0 {
		t.Fatalf("a node with no conditions yet produced %d findings", len(found))
	}
}
