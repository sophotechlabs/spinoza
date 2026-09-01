package issues

import (
	"strconv"
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

func TestReadyAddressesIgnoreMalformedEndpointSubsets(t *testing.T) {
	endpoints := clusterObject(kindEndpoints, "web", nil, nil).obj
	setNested(endpoints, []any{
		"not a subset",
		map[string]any{"addresses": "not a list"},
		map[string]any{"addresses": []any{map[string]any{"ip": "10.0.0.1"}}},
	}, "subsets")

	if got := readyAddresses(endpoints); got != 1 {
		t.Fatalf("ready addresses = %d, want 1", got)
	}
}

func TestAClaimOnItsWayOutIsNotReportedAsPending(t *testing.T) {
	item := clusterObject(kindClaim, "data", nil, map[string]any{"phase": "Pending"})
	item.obj.SetDeletionTimestamp(&metav1.Time{Time: testNow})

	if found := claimFindings(snapshotOf(item)); len(found) != 0 {
		t.Fatal("a claim being deleted was reported as pending")
	}
}

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

func crd(name string, conditions []any) object {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiExtensionsGroup + "/v1",
		"kind":       kindCRD,
		"metadata": map[string]any{
			"name":              name,
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Add(-time.Hour).Format(time.RFC3339),
		},
		"status": map[string]any{"conditions": conditions},
	}}
	return object{obj: obj, desc: api.ResourceDescriptor{
		Group: apiExtensionsGroup, Version: "v1", Resource: "customresourcedefinitions", Kind: kindCRD,
	}}
}

func serviceObject(name string, selector map[string]any) object {
	item := clusterObject(kindService, name, map[string]any{"selector": selector}, nil)
	item.obj.SetNamespace("default")
	return item
}

func endpointsFor(name string, addresses int) object {
	listed := make([]any, 0, addresses)
	for at := range addresses {
		listed = append(listed, map[string]any{"ip": "10.0.0." + strconv.Itoa(at+1)})
	}
	item := clusterObject(kindEndpoints, name, nil, nil)
	item.obj.SetNamespace("default")
	item.obj.Object["subsets"] = []any{map[string]any{"addresses": listed}}
	return item
}

func TestACustomResourceDefinitionTheApiserverRefusedIsFatal(t *testing.T) {
	snap := snapshotOf(crd("widgets.example.com", []any{
		condition("Established", conditionFalse, map[string]any{
			"reason": "SchemaInvalid", "message": "spec.versions[0].schema is invalid",
		}),
	}))

	found := definitionFindings(snap)

	if len(found) != 1 || found[0].title != "SchemaInvalid" {
		t.Fatalf("produced %v", found)
	}
	if found[0].severity != severityFatal {
		t.Fatalf("severity was %d, want fatal", found[0].severity)
	}
}

func TestAnEstablishedDefinitionSaysNothing(t *testing.T) {
	snap := snapshotOf(crd("widgets.example.com", []any{condition("Established", conditionTrue, nil)}))

	if found := definitionFindings(snap); len(found) != 0 {
		t.Fatalf("an established CRD produced %d findings", len(found))
	}
}

func TestADefinitionWithNoConditionsYetIsLeftAlone(t *testing.T) {
	if found := definitionFindings(snapshotOf(crd("widgets.example.com", nil))); len(found) != 0 {
		t.Fatal("a CRD the apiserver has not judged yet was reported")
	}
}

func TestAServiceWithNoReadyBackendIsFatal(t *testing.T) {
	snap := snapshotOf(serviceObject("api", map[string]any{"app": "api"}))

	found := routingFindings(snap)

	if len(found) != 1 || found[0].title != "NoEndpoints" {
		t.Fatalf("produced %v", found)
	}
}

func TestAServiceWithABackendSaysNothing(t *testing.T) {
	snap := snapshotOf(serviceObject("api", map[string]any{"app": "api"}), endpointsFor("api", 2))

	if found := routingFindings(snap); len(found) != 0 {
		t.Fatalf("a service with endpoints produced %d findings", len(found))
	}
}

func TestEndpointsWithNoAddressesDoNotCountAsABackend(t *testing.T) {
	snap := snapshotOf(serviceObject("api", map[string]any{"app": "api"}), endpointsFor("api", 0))

	if found := routingFindings(snap); len(found) != 1 {
		t.Fatal("an Endpoints object holding no addresses was read as a backend")
	}
}

func TestAServiceThatSelectsNothingIsNotJudgedOnEndpoints(t *testing.T) {
	external := serviceObject("db", nil)

	if found := routingFindings(snapshotOf(external)); len(found) != 0 {
		t.Fatal("a selectorless service was asked for endpoints")
	}
}

func certificate(name, notAfter string) object {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name": name, "namespace": "default", "uid": "uid-" + name,
			"creationTimestamp": testNow.Add(-time.Hour).Format(time.RFC3339),
		},
		"status": map[string]any{"notAfter": notAfter},
	}}
	return object{obj: obj, desc: api.ResourceDescriptor{
		Group: "cert-manager.io", Version: "v1", Resource: "certificates", Kind: "Certificate",
	}}
}

func TestACertificateNearingItsEndIsReported(t *testing.T) {
	snap := snapshotOf(certificate("wildcard", testNow.Add(72*time.Hour).Format(time.RFC3339)))

	found := expiryFindings(snap, testNow)

	if len(found) != 1 || found[0].title != "ExpiringSoon" {
		t.Fatalf("produced %v", found)
	}
	if found[0].severity != severityDegraded {
		t.Fatalf("severity was %d, want degraded", found[0].severity)
	}
}

func TestACertificateAlreadyPastItsEndIsFatal(t *testing.T) {
	snap := snapshotOf(certificate("wildcard", testNow.Add(-2*time.Hour).Format(time.RFC3339)))

	found := expiryFindings(snap, testNow)

	if len(found) != 1 || found[0].title != "Expired" {
		t.Fatalf("produced %v", found)
	}
	if found[0].severity != severityFatal {
		t.Fatalf("severity was %d, want fatal", found[0].severity)
	}
}

func TestACertificateWithMonthsLeftSaysNothing(t *testing.T) {
	snap := snapshotOf(certificate("wildcard", testNow.Add(60*24*time.Hour).Format(time.RFC3339)))

	if found := expiryFindings(snap, testNow); len(found) != 0 {
		t.Fatalf("a certificate with two months left produced %d findings", len(found))
	}
}

func TestSomethingWithNoExpiryAtAllIsNotJudgedOnOne(t *testing.T) {
	snap := snapshotOf(clusterObject(kindNamespace, "live", nil, nil))

	if found := expiryFindings(snap, testNow); len(found) != 0 {
		t.Fatal("an object publishing no notAfter was judged on its expiry")
	}
}

func TestAnUnparsableExpiryIsIgnored(t *testing.T) {
	snap := snapshotOf(certificate("wildcard", "whenever"))

	if found := expiryFindings(snap, testNow); len(found) != 0 {
		t.Fatal("an expiry nobody can parse produced a finding")
	}
}
