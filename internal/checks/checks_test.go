package checks

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestEveryCheckIsReportedEvenOnAnEmptyCluster(t *testing.T) {
	found := report(t)

	if len(found.Groups) == 0 {
		t.Fatal("an empty cluster produced no checks")
	}
	if found.Scanned != 0 {
		t.Fatalf("scanned %d objects on an empty cluster", found.Scanned)
	}
	for _, group := range found.Groups {
		if len(group.Findings) != 0 {
			t.Fatalf("%s found something on an empty cluster", group.ID)
		}
	}
}

func TestEveryCheckCarriesItsOwnDescription(t *testing.T) {
	seen := map[string]bool{}
	for _, group := range report(t).Groups {
		if seen[group.ID] {
			t.Fatalf("%s is registered twice", group.ID)
		}
		seen[group.ID] = true
		if group.Title == "" || group.Wrong == "" || group.Remedy == "" {
			t.Fatalf("%s is missing a title, an explanation or a remedy", group.ID)
		}
		if group.Category == "" || group.Severity == "" {
			t.Fatalf("%s is missing a category or a severity", group.ID)
		}
	}
}

func TestOnlySecurityChecksCarryFrameworkLabels(t *testing.T) {
	for _, group := range report(t).Groups {
		if group.Category == categorySecurity {
			if len(group.Frameworks) == 0 {
				t.Fatalf("%s is a security check with no framework label", group.ID)
			}
			continue
		}
		if len(group.Frameworks) != 0 {
			t.Fatalf("%s claims a framework label it cannot cite", group.ID)
		}
	}
}

func manyDeployments(count int) []*unstructured.Unstructured {
	out := make([]*unstructured.Unstructured, 0, count)
	for i := range count {
		out = append(out, deployment(fmt.Sprintf("load-%04d", i), podSpec(container("app", nil))))
	}
	return out
}

func TestAGroupBeyondTheCapKeepsTheRealTotal(t *testing.T) {
	found := report(t, manyDeployments(findingsShown+40)...)
	group := groupNamed(t, found, "requests-missing")

	if group.Total != findingsShown+40 {
		t.Fatalf("total = %d, want %d", group.Total, findingsShown+40)
	}
	if len(group.Findings) != findingsShown {
		t.Fatalf("carried %d findings, want the cap of %d", len(group.Findings), findingsShown)
	}
	if !group.Truncated {
		t.Fatal("a capped group did not say it was truncated")
	}
}

func TestAGroupUnderTheCapIsNotMarkedTruncated(t *testing.T) {
	found := report(t, manyDeployments(3)...)
	group := groupNamed(t, found, "requests-missing")

	if group.Total != 3 || len(group.Findings) != 3 {
		t.Fatalf("total = %d, findings = %d, want 3 and 3", group.Total, len(group.Findings))
	}
	if group.Truncated {
		t.Fatal("a group inside the cap claimed truncation")
	}
}

func TestACappedGroupKeepsTheFirstFindingsInOrder(t *testing.T) {
	found := report(t, manyDeployments(findingsShown+40)...)
	group := groupNamed(t, found, "requests-missing")

	if objectFor(t, found, group.Findings[0]).Name != "load-0000" {
		t.Fatalf("first kept finding was %s", objectFor(t, found, group.Findings[0]).Name)
	}
	last := group.Findings[len(group.Findings)-1]
	if objectFor(t, found, last).Name != fmt.Sprintf("load-%04d", findingsShown-1) {
		t.Fatalf("last kept finding was %s", objectFor(t, found, last).Name)
	}
}

func TestAnObjectIsListedOnceHoweverManyChecksFlagIt(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))))

	flagged := 0
	for _, group := range found.Groups {
		flagged += len(group.Findings)
	}
	if flagged < 2 {
		t.Fatalf("only %d checks fired, so this proves nothing about deduping", flagged)
	}
	if len(found.Objects) != 1 {
		t.Fatalf("the report carries %d objects for one workload", len(found.Objects))
	}
}

func TestTheDictionaryHoldsOnlyObjectsAKeptFindingPointsAt(t *testing.T) {
	found := report(t, manyDeployments(findingsShown+40)...)

	used := map[int]bool{}
	for _, group := range found.Groups {
		for _, finding := range group.Findings {
			used[finding.Ref] = true
		}
	}
	if len(found.Objects) != len(used) {
		t.Fatalf("%d objects listed but only %d referenced", len(found.Objects), len(used))
	}
	if len(found.Objects) > findingsShown {
		t.Fatalf("%d objects survived a cap of %d", len(found.Objects), findingsShown)
	}
}

func TestEveryFindingResolvesToItsOwnObject(t *testing.T) {
	found := report(
		t,
		deployment("api", podSpec(container("app", nil))),
		pod("standalone", podSpec(container("app", nil))),
	)

	for _, group := range found.Groups {
		for _, finding := range group.Findings {
			object := objectFor(t, found, finding)
			if object.Name == "" || object.Kind == "" || object.Resource == "" {
				t.Fatalf("%s resolved to a half-built object: %+v", group.ID, object)
			}
		}
	}
}

func TestAWorkloadsOwnedObjectsAreNotCheckedAgain(t *testing.T) {
	owner := deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	}))))
	set := ownedBy(workload("ReplicaSet", "api-abc", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))), "Deployment", "api")
	running := ownedBy(pod("api-abc-1", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))), "ReplicaSet", "api-abc")

	found := report(t, owner, set, running)

	if found.Scanned != 1 {
		t.Fatalf("scanned %d objects, want only the Deployment", found.Scanned)
	}
	landed := onlyObject(t, found, "privileged-containers")
	if landed.Kind != "Deployment" {
		t.Fatalf("the finding landed on a %s", landed.Kind)
	}
}

func ownedByGroup(obj *unstructured.Unstructured, apiVersion, kind, name string) *unstructured.Unstructured {
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
	}})
	return obj
}

func TestAnOwnerOfTheSameKindInAnotherGroupIsNotTheOwner(t *testing.T) {
	native := deployment("api", podSpec(container("app", nil)))
	stranger := ownedByGroup(
		pod("rogue", podSpec(container("app", withSecurity(map[string]any{"privileged": true})))),
		"custom.example/v1", "Deployment", "api",
	)

	found := report(t, native, stranger)

	if found.Scanned != 2 {
		t.Fatalf("scanned %d, want both: the pod's owner is a Deployment of another group", found.Scanned)
	}
	if onlyObject(t, found, "privileged-containers").Kind != "Pod" {
		t.Fatal("the pod was folded into an unrelated apps/v1 Deployment of the same name")
	}
}

func TestAPodOwnedBySomethingSpinozaDoesNotHoldIsStillChecked(t *testing.T) {
	running := ownedBy(pod("ceph-osd-0", podSpec(container("osd", withSecurity(map[string]any{
		"privileged": true,
	})))), "CephCluster", "rook")

	found := report(t, running)

	if found.Scanned != 1 {
		t.Fatalf("scanned %d objects, want the pod", found.Scanned)
	}
	if onlyObject(t, found, "privileged-containers").Kind != "Pod" {
		t.Fatal("a pod owned by an unknown kind was skipped")
	}
}

func TestANakedPodIsCheckedAtItsOwnPath(t *testing.T) {
	found := report(t, pod("standalone", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))))

	finding := onlyFinding(t, found, "privileged-containers")
	want := "spec:\n  containers:\n    - name: app\n      securityContext:\n        privileged: false\n"
	if finding.Patch != want {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestACronJobIsCheckedThroughItsJobTemplate(t *testing.T) {
	found := report(t, cronJob("nightly", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))))

	finding := onlyFinding(t, found, "privileged-containers")
	if !strings.HasPrefix(finding.Patch, "spec:\n  jobTemplate:\n    spec:\n      template:\n        spec:\n") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestAnInitContainerIsPatchedInItsOwnList(t *testing.T) {
	spec := podSpec(container("app", withSecurity(map[string]any{"privileged": false})))
	spec["initContainers"] = []any{container("setup", withSecurity(map[string]any{"privileged": true}))}

	finding := onlyFinding(t, report(t, deployment("api", spec)), "privileged-containers")

	if !strings.Contains(finding.Patch, "initContainers:") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestAKindWithNoTemplateProducesNoContainers(t *testing.T) {
	broken := deployment("api", podSpec(container("app", nil)))
	delete(broken.Object, "spec")

	found := report(t, broken)

	if found.Scanned != 1 {
		t.Fatalf("scanned %d objects", found.Scanned)
	}
	if findingCount(t, found, "privilege-escalation") != 0 {
		t.Fatal("a workload with no template produced container findings")
	}
}

func TestAContainerListThatIsNotAListIsIgnored(t *testing.T) {
	found := report(t, deployment("api", map[string]any{"containers": "not a list"}))

	if findingCount(t, found, "privilege-escalation") != 0 {
		t.Fatal("an unreadable container list produced findings")
	}
}

func TestAContainerEntryThatIsNotAMapIsSkipped(t *testing.T) {
	found := report(t, deployment("api", map[string]any{
		"containers": []any{"not a container", container("app", nil)},
	}))

	if findingCount(t, found, "privilege-escalation") != 1 {
		t.Fatal("the readable container beside a broken one was lost")
	}
}

func TestAListFailureIsReportedWithoutLosingTheRest(t *testing.T) {
	lister := newLister(deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))))
	lister.errs["pods"] = errors.New("pods are forbidden")

	found := Run(t.Context(), lister, descriptors(), api.Metrics{})

	if !strings.Contains(found.Error, "pods") {
		t.Fatalf("the failure was not reported: %q", found.Error)
	}
	if findingCount(t, found, "privileged-containers") != 1 {
		t.Fatal("a failure on one type lost the findings from another")
	}
}

func TestTheInformersAreWarmedOnce(t *testing.T) {
	lister := newLister()

	Run(t.Context(), lister, descriptors(), api.Metrics{})

	if got := lister.warmCount(); got != 1 {
		t.Fatalf("warmed %d times, want 1", got)
	}
}

func TestOnlyTheKindsTheChecksReadAreWarmed(t *testing.T) {
	asked, absent := needed(descriptors())

	if len(asked) != len(targets) {
		t.Fatalf("asked for %d types, want %d", len(asked), len(targets))
	}
	if len(absent) != 0 {
		t.Fatalf("reported %v as undiscovered on a full catalog", absent)
	}
	for _, desc := range asked {
		if desc.Resource == "" {
			t.Fatal("a descriptor with no resource reached the warm list")
		}
	}
}

func TestATypeDiscoveryHasNotListedIsNamedInTheReport(t *testing.T) {
	descs := descriptors()
	delete(descs, "batch/v1/cronjobs")

	asked, absent := needed(descs)
	if len(asked) != len(targets)-1 {
		t.Fatalf("asked for %d types, want %d", len(asked), len(targets)-1)
	}
	if !slices.Equal(absent, []string{"cronjobs"}) {
		t.Fatalf("absent was %v", absent)
	}

	found := Run(t.Context(), newLister(), descs, api.Metrics{})
	if !strings.Contains(found.Error, "cronjobs") {
		t.Fatalf("the report did not say cronjobs went unaudited: %q", found.Error)
	}
}

func TestAnEmptyCatalogueSaysNothingWasAudited(t *testing.T) {
	found := Run(t.Context(), newLister(), map[string]api.ResourceDescriptor{}, api.Metrics{})

	if found.Scanned != 0 {
		t.Fatalf("scanned %d with no catalog", found.Scanned)
	}
	if !strings.Contains(found.Error, "not discovered yet") {
		t.Fatalf("an audit that read nothing reported %q", found.Error)
	}
	for _, group := range found.Groups {
		if len(group.Findings) != 0 {
			t.Fatalf("%s found something with no catalog", group.ID)
		}
	}
}

func TestAListFailureAndAnAbsentTypeAreBothReported(t *testing.T) {
	descs := descriptors()
	delete(descs, "batch/v1/cronjobs")
	lister := newLister()
	lister.errs["pods"] = errors.New("pods are forbidden")

	found := Run(t.Context(), lister, descs, api.Metrics{})

	if !strings.Contains(found.Error, "pods") || !strings.Contains(found.Error, "cronjobs") {
		t.Fatalf("only one of the two problems was reported: %q", found.Error)
	}
}

func TestFindingsAreOrderedTheSameWayEveryRun(t *testing.T) {
	first := report(
		t,
		deployment("zeta", podSpec(container("app", nil))),
		deployment("alpha", podSpec(container("app", nil))),
		pod("mid", podSpec(container("app", nil))),
	)
	second := report(
		t,
		pod("mid", podSpec(container("app", nil))),
		deployment("alpha", podSpec(container("app", nil))),
		deployment("zeta", podSpec(container("app", nil))),
	)

	if len(first.Groups) != len(second.Groups) {
		t.Fatal("two runs produced different check lists")
	}
	for i, group := range first.Groups {
		other := second.Groups[i]
		if group.ID != other.ID {
			t.Fatalf("check %d differs: %s and %s", i, group.ID, other.ID)
		}
		for j, finding := range group.Findings {
			left := objectFor(t, first, finding)
			right := objectFor(t, second, other.Findings[j])
			if left.Name != right.Name {
				t.Fatalf("%s finding %d differs: %s and %s", group.ID, j, left.Name, right.Name)
			}
		}
	}
}

func TestAKindWithNoCountReportsOneReplica(t *testing.T) {
	cases := []struct {
		kind string
		want int64
	}{
		{kind: "Deployment", want: 1},
		{kind: "StatefulSet", want: 1},
		{kind: "ReplicaSet", want: 1},
		{kind: "ReplicationController", want: 1},
		{kind: "Job", want: 1},
		{kind: "CronJob", want: 1},
		{kind: "Pod", want: 1},
		{kind: "DaemonSet", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			obj := workload(tc.kind, "thing", podSpec(container("app", nil)))
			if tc.kind == "Pod" {
				obj = pod("thing", podSpec(container("app", nil)))
			}
			got := replicasOf(obj, tc.kind)
			if got != tc.want {
				t.Fatalf("%s reported %d replicas, want %d", tc.kind, got, tc.want)
			}
		})
	}
}

func TestADaemonSetCountsTheNodesItWants(t *testing.T) {
	agent := workload("DaemonSet", "agent", podSpec(container("app", nil)))
	agent.Object["status"] = map[string]any{"desiredNumberScheduled": int64(4)}

	if got := replicasOf(agent, "DaemonSet"); got != 4 {
		t.Fatalf("reported %d replicas, want 4", got)
	}
}

func TestAJobReadsItsParallelism(t *testing.T) {
	job := workload("Job", "batch", podSpec(container("app", nil)))
	spec := specOf(job)
	spec["parallelism"] = int64(6)

	if got := replicasOf(job, "Job"); got != 6 {
		t.Fatalf("reported %d replicas, want 6", got)
	}
}

func TestAnOwnerChainDeeperThanTheLimitStops(t *testing.T) {
	first := ownedBy(workload("ReplicaSet", "one", podSpec(container("app", nil))), "ReplicaSet", "two")
	second := ownedBy(workload("ReplicaSet", "two", podSpec(container("app", nil))), "ReplicaSet", "one")
	running := ownedBy(pod("cycle", podSpec(container("app", nil))), "ReplicaSet", "one")

	found := report(t, first, second, running)

	if found.Scanned != 0 {
		t.Fatalf("scanned %d objects, want none: every object here is owned", found.Scanned)
	}
}

func TestATemplateThatIsNotAMapProducesNoContainers(t *testing.T) {
	broken := deployment("api", podSpec(container("app", nil)))
	spec := specOf(broken)
	spec["template"] = map[string]any{"spec": "not a map"}

	found := report(t, broken)

	if found.Scanned != 1 {
		t.Fatalf("scanned %d objects", found.Scanned)
	}
	if findingCount(t, found, "privilege-escalation") != 0 {
		t.Fatal("an unreadable pod template produced container findings")
	}
}
