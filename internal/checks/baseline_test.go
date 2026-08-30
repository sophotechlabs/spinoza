package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func fingerprintOfCluster(t *testing.T, objects ...*unstructured.Unstructured) Baseline {
	t.Helper()
	return Fingerprint(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, wholeCluster())
}

func against(t *testing.T, base Baseline, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Base = &base
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

// what a baseline says has changed

func TestNothingIsNewAgainstABaselineOfTheSameCluster(t *testing.T) {
	same := privilegedDeployment("api")

	report := against(t, fingerprintOfCluster(t, same), same)

	if group := groupNamed(t, report, privilegedCheck); group.NewCount != 0 {
		t.Fatalf("%d findings were new although nothing changed", group.NewCount)
	}
}

func TestAWorkloadAddedSinceTheBaselineIsNew(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"))

	report := against(t, base, privilegedDeployment("api"), privilegedDeployment("web"))

	group := groupNamed(t, report, privilegedCheck)
	if group.NewCount != 1 {
		t.Fatalf("%d findings were new, want the one added since", group.NewCount)
	}
	fresh := 0
	for _, finding := range group.Findings {
		if finding.New {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("%d findings said they were new", fresh)
	}
}

func TestAWorkloadFixedSinceTheBaselineIsCounted(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"), privilegedDeployment("web"))

	report := against(t, base, privilegedDeployment("api"))

	if group := groupNamed(t, report, privilegedCheck); group.Fixed != 1 {
		t.Fatalf("%d findings were counted fixed, want the one that went away", group.Fixed)
	}
}

func TestOnlyWhatIsNewCanBeAskedFor(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"))
	keep := wholeCluster()
	keep.Base = &base
	keep.OnlyNew = true

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), privilegedDeployment("web"),
	), descriptors(), api.Metrics{}, keep, 0)

	finding := onlyFinding(t, report, privilegedCheck)
	if objectFor(t, report, finding).Name != "web" {
		t.Fatalf("only-new returned %s, want the workload added since", objectFor(t, report, finding).Name)
	}
}

func TestMutingSomethingIsNotTheSameAsFixingIt(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"), privilegedDeployment("web"))
	keep := wholeCluster()
	keep.Base = &base
	keep.Mutes = []Mute{{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace)}}

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), privilegedDeployment("web"),
	), descriptors(), api.Metrics{}, keep, 0)

	if group := groupNamed(t, report, privilegedCheck); group.Fixed != 0 {
		t.Fatalf("%d findings were counted fixed when one was only muted", group.Fixed)
	}
}

func TestAnAuditOfOneNamespaceCountsNothingFixed(t *testing.T) {
	base := fingerprintOfCluster(t,
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"))
	keep := wholeCluster()
	keep.Base = &base
	keep.Namespace = testNamespace

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"),
	), descriptors(), api.Metrics{}, keep, 0)

	if group := groupNamed(t, report, privilegedCheck); group.Fixed != 0 {
		t.Fatalf("looking at one namespace reported %d findings fixed in the rest of the cluster",
			group.Fixed)
	}
}

// what a baseline refuses to guess

func TestACheckTheBaselineNeverRanReportsNothingAsNew(t *testing.T) {
	base := Baseline{TakenAt: "2026-08-01T00:00:00Z", Checks: []string{"something-else"}, Keys: map[string]string{}}

	report := against(t, base, privilegedDeployment("api"))

	group := groupNamed(t, report, privilegedCheck)
	if group.Baselined {
		t.Fatal("a check the baseline never ran said it was in it")
	}
	if group.NewCount != 0 {
		t.Fatalf("%d findings were called new by a baseline that never ran the check", group.NewCount)
	}
}

func TestNoBaselineMeansNothingIsMarkedAtAll(t *testing.T) {
	report := report(t, privilegedDeployment("api"))

	group := groupNamed(t, report, privilegedCheck)
	if group.NewCount != 0 || group.Fixed != 0 || group.Baselined {
		t.Fatalf("an audit with no baseline reported %d new, %d fixed", group.NewCount, group.Fixed)
	}
	if report.Baseline != "" {
		t.Fatalf("the report named a baseline it does not have: %q", report.Baseline)
	}
}

func TestTheReportSaysWhenItsBaselineWasTaken(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"))
	base.TakenAt = "2026-08-29T12:00:00Z"

	if report := against(t, base, privilegedDeployment("api")); report.Baseline != base.TakenAt {
		t.Fatalf("the report named %q as its baseline", report.Baseline)
	}
}

func TestWhatWentAwaySinceTheBaselineIsNamed(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"), privilegedDeployment("web"))

	report := against(t, base, privilegedDeployment("api"))

	group := groupNamed(t, report, privilegedCheck)
	if len(group.Gone) != 1 {
		t.Fatalf("named %v, want the one that went away", group.Gone)
	}
	if !strings.Contains(group.Gone[0], "web") {
		t.Fatalf("named %q, want the workload that is no longer there", group.Gone[0])
	}
	if group.Fixed != 1 {
		t.Fatalf("counted %d fixed beside %d named", group.Fixed, len(group.Gone))
	}
}

func TestMutingSomethingDoesNotMakeItLookGone(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"), privilegedDeployment("web"))
	keep := wholeCluster()
	keep.Base = &base
	keep.Mutes = []Mute{{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace)}}

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), privilegedDeployment("web"),
	), descriptors(), api.Metrics{}, keep, 0)

	if gone := groupNamed(t, report, privilegedCheck).Gone; len(gone) != 0 {
		t.Fatalf("muting one finding named %v as gone", gone)
	}
}

func TestNothingWentAwayWhenNothingChanged(t *testing.T) {
	same := privilegedDeployment("api")

	report := against(t, fingerprintOfCluster(t, same), same)

	if gone := groupNamed(t, report, privilegedCheck).Gone; len(gone) != 0 {
		t.Fatalf("named %v as gone although nothing changed", gone)
	}
}

func TestAnAuditOfOneNamespaceNamesNothingAsGone(t *testing.T) {
	base := fingerprintOfCluster(t,
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"))
	keep := wholeCluster()
	keep.Base = &base
	keep.Namespace = testNamespace

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"),
	), descriptors(), api.Metrics{}, keep, 0)

	if gone := groupNamed(t, report, privilegedCheck).Gone; len(gone) != 0 {
		t.Fatalf("looking at one namespace named %v as gone from the rest of the cluster", gone)
	}
}

func TestAnAuditOfTheWorkloadsAloneCountsNothingFixed(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"))
	keep := Filter{Base: &base}

	report := Run(t.Context(), newLister(privilegedDeployment("api")), descriptors(), api.Metrics{}, keep, 0)

	group := groupNamed(t, report, privilegedCheck)
	if group.Fixed != 0 || len(group.Gone) != 0 {
		t.Fatalf("narrowing to the workloads reported %d fixed and %v gone", group.Fixed, group.Gone)
	}
}

func TestACheckTheBaselineNeverRanNamesNothingAsGone(t *testing.T) {
	base := Baseline{TakenAt: "2026-08-01T00:00:00Z", Checks: []string{"something-else"}, Keys: map[string]string{}}

	report := against(t, base, privilegedDeployment("api"))

	if gone := groupNamed(t, report, privilegedCheck).Gone; len(gone) != 0 {
		t.Fatalf("a check the baseline never ran named %v as gone", gone)
	}
}

// what churns and what a baseline refuses to call new

func TestAPodWhoseNameWasGeneratedIsTheSameFindingAfterItIsReplaced(t *testing.T) {
	before := pod("web-7d9f8-x2klm", podSpec(container("app", nil)))
	before.SetGenerateName("web-7d9f8-")
	after := pod("web-7d9f8-q4jrt", podSpec(container("app", nil)))
	after.SetGenerateName("web-7d9f8-")

	report := against(t, fingerprintOfCluster(t, before), after)

	for _, group := range report.Groups {
		if group.NewCount != 0 {
			t.Fatalf("%s called %d findings new after a generated name changed",
				group.ID, group.NewCount)
		}
	}
}

func TestAPodNamedByHandIsANewFindingWhenItIsANewPod(t *testing.T) {
	base := fingerprintOfCluster(t, pod("first", podSpec(container("app", nil))))

	report := against(t, base, pod("second", podSpec(container("app", nil))))

	fresh := 0
	for _, group := range report.Groups {
		fresh += group.NewCount
	}
	if fresh == 0 {
		t.Fatal("a pod nobody had seen before was not reported as new")
	}
}

func TestACheckThatReadsLiveMeasurementIsNotComparedAtAll(t *testing.T) {
	usage := map[string]api.ResourceUsage{"apps/api": {CPUMilli: 1, MemoryMi: 1}}
	base := fingerprintOfCluster(t, privilegedDeployment("api"))
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(privilegedDeployment("api")), descriptors(),
		api.Metrics{Pods: usage}, keep, 0)

	group := groupNamed(t, report, "requests-far-above-usage")
	if !group.Measured {
		t.Fatal("a check reading live usage did not say it was measured")
	}
	if group.Baselined || group.NewCount != 0 || group.Fixed != 0 {
		t.Fatalf("a measured check was compared against the baseline: %+v", group)
	}
}

func TestAFindingAboutTheClusterItselfSurvivesABaseline(t *testing.T) {
	lister := newLister()
	lister.facts = Facts{
		ServerVersion:  "v1.24.0",
		ServedVersions: []string{"batch/v1beta1"},
	}
	base := Fingerprint(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster())
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, keep, 0)

	group := groupNamed(t, report, "serves-a-removed-api")
	if group.Total == 0 {
		t.Fatal("the cluster fact this is built on stopped firing")
	}
	if group.NewCount != 0 {
		t.Fatalf("a finding about the cluster itself was called new against its own baseline")
	}
}

// what the fingerprint covers

func TestAFingerprintIgnoresTheFilterTheCallerWasLookingThrough(t *testing.T) {
	keep := wholeCluster()
	keep.MinSeverity = severityHigh
	keep.Namespace = "nowhere"
	keep.Mutes = []Mute{{Check: privilegedCheck}}

	base := Fingerprint(t.Context(), newLister(privilegedDeployment("api")), descriptors(), api.Metrics{}, keep)

	if base.Counts[privilegedCheck] != 1 {
		t.Fatalf("the baseline counted %d for a muted, filtered check, want the finding itself",
			base.Counts[privilegedCheck])
	}
}

func TestAFingerprintLeavesOutTheChecksThatStoodDown(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"))

	for _, id := range base.Checks {
		if id == "requests-far-above-usage" {
			t.Fatal("a check with no metrics to read was recorded as covered by the baseline")
		}
	}
}

// what survives being carried to another cluster

func fromElsewhere(t *testing.T, objects ...*unstructured.Unstructured) Baseline {
	t.Helper()
	base := fingerprintOfCluster(t, objects...)
	base.Cluster = "https://somewhere.else"
	base.TakenAt = "2026-08-29T00:00:00Z"
	return base
}

func TestABaselineFromAnotherClusterMarksNothingNew(t *testing.T) {
	base := fromElsewhere(t, privilegedDeployment("api"))
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(
		privilegedDeployment("something-else-entirely"),
	), descriptors(), api.Metrics{}, keep, 0)

	group := groupNamed(t, report, privilegedCheck)
	if group.NewCount != 0 {
		t.Fatalf("%d findings were called new against another cluster's baseline", group.NewCount)
	}
	if group.Fixed != 0 || len(group.Gone) != 0 {
		t.Fatalf("another cluster's baseline reported %d fixed and %v gone", group.Fixed, group.Gone)
	}
}

func TestABaselineFromAnotherClusterStillSaysWhatEachCheckFound(t *testing.T) {
	base := fromElsewhere(t, privilegedDeployment("api"), privilegedDeployment("web"))
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(
		privilegedDeployment("only-one"),
	), descriptors(), api.Metrics{}, keep, 0)

	group := groupNamed(t, report, privilegedCheck)
	if !group.Ran {
		t.Fatal("the baseline ran this check and the report does not say so")
	}
	if group.Was != 2 {
		t.Fatalf("the baseline counted %d, want the two it found", group.Was)
	}
	if group.Total != 1 {
		t.Fatalf("this cluster reported %d, want the one it has", group.Total)
	}
}

func TestACheckTheBaselineNeverRanSaysItWasNotAsked(t *testing.T) {
	base := Baseline{
		TakenAt: "2026-08-29T00:00:00Z",
		Cluster: "https://somewhere.else",
		Checks:  []string{"something-else"},
		Counts:  map[string]int{},
		Keys:    map[string]string{},
	}
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(privilegedDeployment("api")), descriptors(), api.Metrics{}, keep, 0)

	if group := groupNamed(t, report, privilegedCheck); group.Ran {
		t.Fatal("a check the baseline never ran was reported as having a count of nought")
	}
}

func TestTheReportSaysHowManyWorkloadsTheBaselineSaw(t *testing.T) {
	base := fromElsewhere(t, privilegedDeployment("api"), privilegedDeployment("web"))
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(privilegedDeployment("one")), descriptors(), api.Metrics{}, keep, 0)

	if report.WasScanned != 2 {
		t.Fatalf("the report says the baseline saw %d workloads, want the two it did", report.WasScanned)
	}
	if report.Scanned != 1 {
		t.Fatalf("the report says this cluster has %d workloads", report.Scanned)
	}
}

func TestABaselineOfThisClusterStillDiffsFindingByFinding(t *testing.T) {
	base := fingerprintOfCluster(t, privilegedDeployment("api"), privilegedDeployment("web"))
	keep := wholeCluster()
	keep.Base = &base

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), privilegedDeployment("fresh"),
	), descriptors(), api.Metrics{}, keep, 0)

	group := groupNamed(t, report, privilegedCheck)
	if group.NewCount != 1 || len(group.Gone) != 1 {
		t.Fatalf("its own baseline reported %d new and %v gone", group.NewCount, group.Gone)
	}
	if group.Was != 2 {
		t.Fatalf("its own baseline counted %d", group.Was)
	}
}
