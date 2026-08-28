package checks

import (
	"errors"
	"strings"
	"testing"

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
	finding := onlyFinding(t, found, "privileged-containers")
	if finding.Kind != "Deployment" {
		t.Fatalf("the finding landed on a %s", finding.Kind)
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
	if onlyFinding(t, found, "privileged-containers").Kind != "Pod" {
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

	if lister.warmed != 1 {
		t.Fatalf("warmed %d times, want 1", lister.warmed)
	}
}

func TestOnlyTheKindsTheChecksReadAreWarmed(t *testing.T) {
	asked := needed(descriptors())

	if len(asked) != len(targets) {
		t.Fatalf("asked for %d types, want %d", len(asked), len(targets))
	}
	for _, desc := range asked {
		if desc.Resource == "" {
			t.Fatal("a descriptor with no resource reached the warm list")
		}
	}
}

func TestATypeTheClusterDoesNotServeIsSkipped(t *testing.T) {
	descs := descriptors()
	delete(descs, "batch/v1/cronjobs")

	asked := needed(descs)

	if len(asked) != len(targets)-1 {
		t.Fatalf("asked for %d types, want %d", len(asked), len(targets)-1)
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
			if finding.Object.Name != other.Findings[j].Object.Name {
				t.Fatalf("%s finding %d differs: %s and %s",
					group.ID, j, finding.Object.Name, other.Findings[j].Object.Name)
			}
		}
	}
}

func TestReplicaCountsComeFromTheRightFieldPerKind(t *testing.T) {
	cases := map[string]int64{
		"Deployment":            1,
		"StatefulSet":           1,
		"ReplicaSet":            1,
		"ReplicationController": 1,
		"Job":                   1,
		"CronJob":               1,
		"Pod":                   1,
	}
	for kind, want := range cases {
		obj := workload(kind, "thing", podSpec(container("app", nil)))
		if kind == "Pod" {
			obj = pod("thing", podSpec(container("app", nil)))
		}
		if got := replicasOf(obj, kind); got != want {
			t.Fatalf("%s reported %d replicas, want %d", kind, got, want)
		}
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
