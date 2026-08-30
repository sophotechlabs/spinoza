package checks

import (
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
	base := Baseline{TakenAt: "2026-08-01T00:00:00Z", Checks: []string{"something-else"}, Keys: map[string]bool{}}

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
		if id == "orphaned-config-map" {
			t.Fatal("a check that stood down was recorded as covered by the baseline")
		}
	}
}
