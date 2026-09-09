package waste

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type fakeCluster struct {
	pods        []*unstructured.Unstructured
	replicaSets []*unstructured.Unstructured
	podErr      error
	ownerErr    error
	asked       []api.ObjectRef
}

func (fc *fakeCluster) ListKind(_ context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error) {
	fc.asked = append(fc.asked, ref)
	switch ref.Resource {
	case "pods":
		return fc.pods, fc.podErr
	case "replicasets":
		return fc.replicaSets, fc.ownerErr
	default:
		return nil, fc.ownerErr
	}
}

type fakeMeter struct {
	reading Reading
	err     error
	calls   int
}

func (fm *fakeMeter) Usage(_ context.Context, _ []Ref) (Reading, error) {
	fm.calls++
	if fm.err != nil {
		return Reading{}, fm.err
	}
	return fm.reading, nil
}

func reads(pods map[string]api.ResourceUsage, source, window string) *fakeMeter {
	return &fakeMeter{reading: Reading{Pods: pods, Source: source, Window: window}}
}

func yes() *bool {
	value := true
	return &value
}

func requesting(cpu, memory string) map[string]any {
	asked := map[string]any{}
	if cpu != "" {
		asked["cpu"] = cpu
	}
	if memory != "" {
		asked["memory"] = memory
	}
	return map[string]any{"resources": map[string]any{"requests": asked}}
}

func bareContainer() map[string]any {
	return map[string]any{"image": "nginx"}
}

func podOf(namespace, name string, containers ...map[string]any) *unstructured.Unstructured {
	list := make([]any, 0, len(containers))
	for _, one := range containers {
		list = append(list, one)
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"containers": list},
	}}
}

func replicaSetOf(namespace, name, owner string) *unstructured.Unstructured {
	out := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
	}}
	if owner != "" {
		ownedBy(out, "apps/v1", "Deployment", owner)
	}
	return out
}

func ownedBy(obj *unstructured.Unstructured, apiVersion, kind, name string) *unstructured.Unstructured {
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Controller: yes(),
	}})
	return obj
}

func rowFor(t *testing.T, rows []api.WasteRow, namespace, name string) api.WasteRow {
	t.Helper()
	for _, row := range rows {
		if row.Namespace == namespace && row.Name == name {
			return row
		}
	}
	t.Fatalf("no row for %s/%s in %+v", namespace, name, rows)
	return api.WasteRow{}
}

func built(t *testing.T, cluster *fakeCluster, meters ...Meter) api.WasteReport {
	t.Helper()
	report, err := Build(t.Context(), cluster, Request{Meters: meters})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return report
}

func TestRequestsAreSummedAcrossEveryContainerAndEveryPod(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("100m", "128Mi"), requesting("250m", "256Mi")),
		podOf("shop", "web-2", requesting("150m", "64Mi")),
	}}

	report := built(t, cluster)

	row := rowFor(t, report.Namespaces, "shop", "")
	for _, want := range []struct {
		what string
		got  int64
		want int64
	}{
		{"pods", int64(row.Pods), 2},
		{"cpu requested in millicores", row.CPURequested, 500},
		{"memory requested in mebibytes", row.MemRequested, 448},
	} {
		if want.got != want.want {
			t.Errorf("%s = %d, want %d", want.what, want.got, want.want)
		}
	}
}

func TestAPodThatReservesNothingIsCountedAndNamedRatherThanReadAsZero(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
		podOf("shop", "loose", bareContainer()),
	}}
	usage := map[string]api.ResourceUsage{
		"shop/web-1": {CPUMilli: 100, MemoryMi: 128},
		"shop/loose": {CPUMilli: 90, MemoryMi: 200},
	}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	row := rowFor(t, report.Namespaces, "shop", "")
	if row.Pods != 2 {
		t.Errorf("pods = %d, want both counted", row.Pods)
	}
	if row.CPURequested != 400 {
		t.Errorf("cpu requested = %d, want only the pod that asked for something", row.CPURequested)
	}
	if row.CPUReclaimable != 300 {
		t.Errorf("cpu reclaimable = %d, want the pod that reserves nothing to add nothing", row.CPUReclaimable)
	}
	if !strings.Contains(report.Reason, "1 pod reserves nothing") {
		t.Errorf("reason = %q, want it to count the pod that reserves nothing", report.Reason)
	}
	if !strings.Contains(report.Reason, "shop/loose") {
		t.Errorf("reason = %q, want it to name the pod that reserves nothing", report.Reason)
	}
}

func TestReclaimableFloorsAtZeroWhenAPodUsesMoreThanItReserved(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("100m", "128Mi")),
	}}
	usage := map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 400, MemoryMi: 512}}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	row := rowFor(t, report.Namespaces, "shop", "")
	if row.CPUReclaimable != 0 || row.MemReclaimable != 0 {
		t.Errorf("reclaimable = %d cpu, %d memory, want both floored at zero", row.CPUReclaimable, row.MemReclaimable)
	}
	if row.CPUUsed != 400 || row.MemUsed != 512 {
		t.Errorf("used = %d cpu, %d memory, want what was measured", row.CPUUsed, row.MemUsed)
	}
}

func TestTheRollupsHoldTwoWorkloadsInOneNamespaceAndABarePod(t *testing.T) {
	cluster := &fakeCluster{
		pods: []*unstructured.Unstructured{
			ownedBy(podOf("shop", "web-1", requesting("100m", "128Mi")), "apps/v1", "ReplicaSet", "web-abc"),
			ownedBy(podOf("shop", "web-2", requesting("100m", "128Mi")), "apps/v1", "ReplicaSet", "web-abc"),
			ownedBy(podOf("shop", "api-1", requesting("200m", "256Mi")), "apps/v1", "ReplicaSet", "api-xyz"),
			podOf("shop", "loose", requesting("50m", "32Mi")),
		},
		replicaSets: []*unstructured.Unstructured{
			replicaSetOf("shop", "web-abc", "web"),
			replicaSetOf("shop", "api-xyz", "api"),
		},
	}

	report := built(t, cluster)

	if len(report.Namespaces) != 1 {
		t.Fatalf("namespaces = %+v, want one row", report.Namespaces)
	}
	if report.Namespaces[0].Pods != 4 {
		t.Errorf("namespace pods = %d, want every pod counted once", report.Namespaces[0].Pods)
	}
	for _, want := range []struct {
		kind string
		name string
		pods int
		cpu  int64
	}{
		{"Deployment", "web", 2, 200},
		{"Deployment", "api", 1, 200},
		{"Pod", "loose", 1, 50},
	} {
		row := rowFor(t, report.Workloads, "shop", want.name)
		if row.Kind != want.kind {
			t.Errorf("%s kind = %q, want %q", want.name, row.Kind, want.kind)
		}
		if row.Pods != want.pods {
			t.Errorf("%s pods = %d, want %d", want.name, row.Pods, want.pods)
		}
		if row.CPURequested != want.cpu {
			t.Errorf("%s cpu requested = %d, want %d", want.name, row.CPURequested, want.cpu)
		}
	}
}

func TestWithNoUsageSourceTheReportSaysSoAndStillShowsWhatIsReserved(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
	}}

	report := built(t, cluster)

	if report.Measured {
		t.Error("measured = true, want the report to say nothing measured usage")
	}
	if !strings.Contains(report.Reason, notMeasured) {
		t.Errorf("reason = %q, want it to say nothing measured usage", report.Reason)
	}
	if !strings.Contains(report.Reason, noSource) {
		t.Errorf("reason = %q, want it to say why", report.Reason)
	}
	if report.Window != "" || report.Source != "" {
		t.Errorf("window = %q, source = %q, want neither claimed", report.Window, report.Source)
	}
	if len(report.PartialOn) != 0 {
		t.Errorf("partialOn = %v, want the reason to carry it rather than every row", report.PartialOn)
	}
	row := rowFor(t, report.Namespaces, "shop", "")
	if row.CPURequested != 400 || row.MemRequested != 512 {
		t.Errorf("row = %+v, want the reserved columns still filled", row)
	}
	if row.Measured {
		t.Error("row measured = true, want a row nothing measured to say so")
	}
	if row.CPUUsed != 0 || row.CPUReclaimable != 0 {
		t.Errorf("row = %+v, want no fabricated usage", row)
	}
}

func TestAWindowIsPreferredOverTheLiveReadingWhenBothAnswer(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
	}}
	over := reads(map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 100}}, "prometheus", "the last 30 minutes")
	now := reads(map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 380}}, "metrics-server", "right now")

	report := built(t, cluster, over, now)

	if report.Source != "prometheus" || report.Window != "the last 30 minutes" {
		t.Errorf("source = %q, window = %q, want the window reading", report.Source, report.Window)
	}
	if now.calls != 0 {
		t.Errorf("metrics-server calls = %d, want it left alone while prometheus answers", now.calls)
	}
	row := rowFor(t, report.Namespaces, "shop", "")
	if row.CPUUsed != 100 {
		t.Errorf("cpu used = %d, want the window reading", row.CPUUsed)
	}
}

func TestTheLiveReadingIsUsedWhenTheWindowFails(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
	}}
	over := &fakeMeter{err: errors.New("prometheus is unavailable")}
	now := reads(map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 380}}, "metrics-server", "right now")

	report := built(t, cluster, over, now)

	if !report.Measured {
		t.Fatal("measured = false, want the fallback to count")
	}
	if report.Source != "metrics-server" || report.Window != "right now" {
		t.Errorf("source = %q, window = %q, want the live reading", report.Source, report.Window)
	}
	row := rowFor(t, report.Namespaces, "shop", "")
	if row.CPUUsed != 380 {
		t.Errorf("cpu used = %d, want the live reading", row.CPUUsed)
	}
}

func TestANamespaceWhoseUsageCouldNotBeReadIsNamedRatherThanCountedAsZero(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
		podOf("quiet", "job-1", requesting("800m", "1024Mi")),
	}}
	usage := map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 100, MemoryMi: 128}}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	if len(report.PartialOn) != 1 || report.PartialOn[0] != "quiet" {
		t.Fatalf("partialOn = %v, want the namespace nothing measured", report.PartialOn)
	}
	quiet := rowFor(t, report.Namespaces, "quiet", "")
	if quiet.Measured {
		t.Error("quiet measured = true, want a namespace nothing measured to say so")
	}
	if quiet.CPURequested != 800 {
		t.Errorf("quiet cpu requested = %d, want what it reserves", quiet.CPURequested)
	}
	if quiet.CPUReclaimable != 0 || quiet.MemReclaimable != 0 {
		t.Errorf("quiet = %+v, want no reclaimable invented from unread usage", quiet)
	}
	shop := rowFor(t, report.Namespaces, "shop", "")
	if !shop.Measured {
		t.Error("shop measured = false, want the namespace that was read to say so")
	}
}

func TestAWorkloadWhoseUsageCouldNotBeReadIsNamedWhenItsNeighbourWasRead(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		ownedBy(podOf("shop", "web-1", requesting("400m", "512Mi")), "apps/v1", "ReplicaSet", "web-abc"),
		ownedBy(podOf("shop", "api-1", requesting("400m", "512Mi")), "apps/v1", "ReplicaSet", "api-xyz"),
	}}
	usage := map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 100, MemoryMi: 128}}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	if len(report.PartialOn) != 1 || report.PartialOn[0] != "shop/api-xyz" {
		t.Fatalf("partialOn = %v, want the workload nothing measured", report.PartialOn)
	}
}

func TestTheRowsAreOrderedByTheirCombinedShareOfWhatIsReclaimable(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("cpuheavy", "one", requesting("1000m", "100Mi")),
		podOf("memheavy", "one", requesting("200m", "4000Mi")),
	}}
	usage := map[string]api.ResourceUsage{"cpuheavy/one": {}, "memheavy/one": {}}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	if len(report.Namespaces) != 2 {
		t.Fatalf("namespaces = %+v, want two rows", report.Namespaces)
	}
	if report.Namespaces[0].Namespace != "memheavy" {
		t.Errorf(
			"first row = %q, want the namespace whose combined share is larger and not the one with more cpu",
			report.Namespaces[0].Namespace,
		)
	}
	if report.Namespaces[1].Namespace != "cpuheavy" {
		t.Errorf("second row = %q, want cpuheavy", report.Namespaces[1].Namespace)
	}
}

func TestAWorkloadIsNamedByTheOwnerAtTheTopOfTheChain(t *testing.T) {
	cluster := &fakeCluster{
		pods: []*unstructured.Unstructured{
			ownedBy(podOf("shop", "web-1", requesting("100m", "128Mi")), "apps/v1", "ReplicaSet", "web-abc"),
		},
		replicaSets: []*unstructured.Unstructured{replicaSetOf("shop", "web-abc", "web")},
	}

	report := built(t, cluster)

	row := rowFor(t, report.Workloads, "shop", "web")
	if row.Kind != "Deployment" {
		t.Errorf("kind = %q, want the top of the owner chain", row.Kind)
	}
}

func TestAWorkloadListThatCouldNotBeReadIsSaidSoAndTheOwnerIsStillNamed(t *testing.T) {
	cluster := &fakeCluster{
		pods: []*unstructured.Unstructured{
			ownedBy(podOf("shop", "web-1", requesting("100m", "128Mi")), "apps/v1", "ReplicaSet", "web-abc"),
		},
		ownerErr: errors.New("replicasets are forbidden here"),
	}

	report := built(t, cluster)

	row := rowFor(t, report.Workloads, "shop", "web-abc")
	if row.Kind != "ReplicaSet" {
		t.Errorf("kind = %q, want the owner the pod itself names", row.Kind)
	}
	if !strings.Contains(report.Reason, "replicasets could not be read") {
		t.Errorf("reason = %q, want it to say the owner chain was not walked", report.Reason)
	}
}

func TestAFailingPodListIsAnErrorRatherThanAnEmptyReport(t *testing.T) {
	cluster := &fakeCluster{podErr: errors.New("pods are forbidden here")}

	_, err := Build(t.Context(), cluster, Request{})

	if err == nil {
		t.Fatal("err = nil, want a failed pod list reported rather than an empty table")
	}
}

func TestAClusterWithNoPodsSaysSoRatherThanShowingAnEmptyTable(t *testing.T) {
	report := built(t, &fakeCluster{})

	if report.Reason != noPods {
		t.Errorf("reason = %q, want %q", report.Reason, noPods)
	}
	if report.Measured {
		t.Error("measured = true, want nothing claimed about usage")
	}
}

func TestOnlyTheNamespaceTheRequestNamesIsRead(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("100m", "128Mi")),
	}}

	_, err := Build(t.Context(), cluster, Request{Namespace: "shop"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(cluster.asked) == 0 {
		t.Fatal("nothing was listed")
	}
	for _, ref := range cluster.asked {
		if ref.Namespace != "shop" {
			t.Errorf("listed %s in %q, want the namespace the request names", ref.Resource, ref.Namespace)
		}
	}
}

func TestALongListOfPodsThatReserveNothingSaysHowManyItDidNotName(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "one", bareContainer()),
		podOf("shop", "two", bareContainer()),
		podOf("shop", "three", bareContainer()),
		podOf("shop", "four", bareContainer()),
		podOf("shop", "five", bareContainer()),
	}}

	report := built(t, cluster)

	if !strings.Contains(report.Reason, "5 pods reserve nothing") {
		t.Errorf("reason = %q, want the whole count", report.Reason)
	}
	if !strings.Contains(report.Reason, "and 2 more") {
		t.Errorf("reason = %q, want the cut list to say how many it did not name", report.Reason)
	}
}

func TestARequestIsReadWhateverShapeTheSpecHoldsIt(t *testing.T) {
	for _, one := range []struct {
		name   string
		cpu    any
		memory any
		want   int64
		asks   bool
	}{
		{name: "a quantity string", cpu: "250m", memory: "128Mi", want: 250, asks: true},
		{name: "a whole number of cores", cpu: int64(2), memory: int64(1024 * 1024), want: 2000, asks: true},
		{name: "a fraction of a core", cpu: 0.5, memory: float64(1024 * 1024), want: 500, asks: true},
		{name: "something that is not a quantity", cpu: "half a core", memory: true, want: 0, asks: false},
		{name: "nothing at all", cpu: nil, memory: nil, want: 0, asks: false},
	} {
		t.Run(one.name, func(t *testing.T) {
			asked := map[string]any{}
			if one.cpu != nil {
				asked["cpu"] = one.cpu
			}
			if one.memory != nil {
				asked["memory"] = one.memory
			}
			pod := podOf("shop", "web-1", map[string]any{
				"resources": map[string]any{"requests": asked},
			})

			cpu, _, asks := requestsOf(pod)

			if cpu != one.want {
				t.Errorf("cpu = %d, want %d", cpu, one.want)
			}
			if asks != one.asks {
				t.Errorf("asks = %t, want %t", asks, one.asks)
			}
		})
	}
}

func TestAContainerWithNoResourcesBlockReservesNothing(t *testing.T) {
	for _, one := range []struct {
		name      string
		container map[string]any
	}{
		{name: "no resources key", container: map[string]any{"image": "nginx"}},
		{name: "no requests key", container: map[string]any{"resources": map[string]any{"limits": map[string]any{"cpu": "1"}}}},
		{name: "not a container at all", container: nil},
	} {
		t.Run(one.name, func(t *testing.T) {
			pod := podOf("shop", "web-1")
			pod.Object["spec"] = map[string]any{"containers": []any{one.container}}

			cpu, mem, asks := requestsOf(pod)

			if cpu != 0 || mem != 0 || asks {
				t.Errorf("requests = %d cpu, %d memory, asks %t, want nothing reserved", cpu, mem, asks)
			}
		})
	}
}

func TestAnOwnerReferenceIsWalkedOnlyWhenItControlsThePod(t *testing.T) {
	loose := podOf("shop", "loose", requesting("100m", "128Mi"))
	loose.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: "node-1"}})
	core := ownedBy(podOf("shop", "core-1", requesting("100m", "128Mi")), "v1", "ReplicationController", "core")
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{loose, core}}

	report := built(t, cluster)

	bare := rowFor(t, report.Workloads, "shop", "loose")
	if bare.Kind != "Pod" {
		t.Errorf("kind = %q, want a pod nothing controls to stand as its own workload", bare.Kind)
	}
	owned := rowFor(t, report.Workloads, "shop", "core")
	if owned.Kind != "ReplicationController" {
		t.Errorf("kind = %q, want the controller in the core group", owned.Kind)
	}
}

func TestAnOwnerChainStopsAtTheDepthItIsAllowed(t *testing.T) {
	pod := ownedBy(podOf("shop", "web-1", requesting("100m", "128Mi")), "apps/v1", "ReplicaSet", "step-0")
	sets := make([]*unstructured.Unstructured, 0, ownerDepth+2)
	for at := range ownerDepth + 2 {
		set := replicaSetOf("shop", "step-"+strconv.Itoa(at), "")
		ownedBy(set, "apps/v1", "ReplicaSet", "step-"+strconv.Itoa(at+1))
		sets = append(sets, set)
	}
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{pod}, replicaSets: sets}

	report := built(t, cluster)

	if len(report.Workloads) != 1 {
		t.Fatalf("workloads = %+v, want one row", report.Workloads)
	}
	if report.Workloads[0].Name != "step-"+strconv.Itoa(ownerDepth-1) {
		t.Errorf("workload = %q, want the walk stopped at the depth it is allowed", report.Workloads[0].Name)
	}
}

func TestEverySourceFailingSaysWhatEachOneSaid(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("shop", "web-1", requesting("400m", "512Mi")),
	}}
	over := &fakeMeter{err: errors.New("prometheus is unavailable")}
	now := &fakeMeter{err: errors.New("metrics-server did not answer")}

	report := built(t, cluster, over, now)

	if report.Measured {
		t.Error("measured = true, want the report to say nothing measured usage")
	}
	for _, want := range []string{"prometheus is unavailable", "metrics-server did not answer"} {
		if !strings.Contains(report.Reason, want) {
			t.Errorf("reason = %q, want it to hold %q", report.Reason, want)
		}
	}
}

func TestRowsWithNothingReclaimableAreStillOrderedTheSameWayTwice(t *testing.T) {
	cluster := &fakeCluster{pods: []*unstructured.Unstructured{
		podOf("beta", "one", bareContainer()),
		podOf("alpha", "one", bareContainer()),
	}}
	usage := map[string]api.ResourceUsage{"beta/one": {}, "alpha/one": {}}

	report := built(t, cluster, reads(usage, "metrics-server", "right now"))

	if report.Namespaces[0].Namespace != "alpha" || report.Namespaces[1].Namespace != "beta" {
		t.Errorf("namespaces = %+v, want a stable order by name when nothing is reclaimable", report.Namespaces)
	}
}

func TestAFinishedPodReservesNothingAndIsLeftOut(t *testing.T) {
	cases := []struct {
		name  string
		phase string
		kept  bool
	}{
		{name: "a running pod", phase: "Running", kept: true},
		{name: "a pending pod", phase: "Pending", kept: true},
		{name: "a pod with no phase yet", phase: "", kept: true},
		{name: "a job pod that finished", phase: "Succeeded"},
		{name: "a pod that failed", phase: "Failed"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			pod := podOf("shop", "worker", requesting("500m", "512Mi"))
			if one.phase != "" {
				if err := unstructured.SetNestedField(pod.Object, one.phase, "status", "phase"); err != nil {
					t.Fatalf("set phase: %v", err)
				}
			}

			got := reservations([]*unstructured.Unstructured{pod}, nil)

			if one.kept && len(got) != 1 {
				t.Fatalf("a %s pod was left out", one.phase)
			}
			if !one.kept && len(got) != 0 {
				t.Fatalf("a %s pod was counted as reserving something", one.phase)
			}
		})
	}
}
