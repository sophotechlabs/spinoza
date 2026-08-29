package checks

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func withFacts(t *testing.T, facts Facts) api.CheckReport {
	t.Helper()
	lister := newLister()
	lister.facts = facts
	return Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
}

// what the cluster says about its own versions

func TestAVersionARemovalIsComingForIsReported(t *testing.T) {
	report := withFacts(t, Facts{
		ServerVersion:  "v1.24.9+k3s1",
		ServedVersions: []string{"v1", "batch/v1beta1", "apps/v1"},
	})

	finding := onlyFinding(t, report, "serves-a-removed-api")
	if !strings.Contains(finding.Detail, "removed in 1.25") {
		t.Fatalf("detail was %q, want the release named", finding.Detail)
	}
	if !strings.Contains(finding.Detail, "this cluster is 1.24") {
		t.Fatalf("detail was %q, want the running version named", finding.Detail)
	}
}

func TestAVersionAlreadyPastItsRemovalIsNotReported(t *testing.T) {
	report := withFacts(t, Facts{
		ServerVersion:  "v1.31.0",
		ServedVersions: []string{"batch/v1beta1"},
	})

	if findingCount(t, report, "serves-a-removed-api") != 0 {
		t.Fatal("a version this cluster is already past was reported as a coming removal")
	}
}

func TestAVersionTheClusterDoesNotServeIsNotReported(t *testing.T) {
	report := withFacts(t, Facts{
		ServerVersion:  "v1.24.0",
		ServedVersions: []string{"v1", "apps/v1"},
	})

	if findingCount(t, report, "serves-a-removed-api") != 0 {
		t.Fatal("a version the cluster does not serve was reported")
	}
}

func TestAServerVersionNobodyCanReadStopsTheCheck(t *testing.T) {
	report := withFacts(t, Facts{
		ServerVersion:  "unknown",
		ServedVersions: []string{"batch/v1beta1"},
	})

	if findingCount(t, report, "serves-a-removed-api") != 0 {
		t.Fatal("the check guessed at a version it could not read")
	}
}

func TestEveryRemovalTheTableNamesIsCaughtOnAnOldEnoughCluster(t *testing.T) {
	served := make([]string, 0, len(removals))
	for _, one := range removals {
		served = append(served, one.groupVersion)
	}

	report := withFacts(t, Facts{ServerVersion: "v1.21.0", ServedVersions: served})

	if got := groupNamed(t, report, "serves-a-removed-api").Total; got != len(removals) {
		t.Fatalf("reported %d of the %d removals the table names", got, len(removals))
	}
}

// what the apiserver itself said

func TestTheApiserversOwnWarningIsPassedThrough(t *testing.T) {
	report := withFacts(t, Facts{
		Warnings: []string{"v1 Endpoints is deprecated in v1.33+; use discovery.k8s.io/v1 EndpointSlice"},
	})

	finding := onlyFinding(t, report, "apiserver-says-deprecated")
	if !strings.Contains(finding.Detail, "EndpointSlice") {
		t.Fatalf("detail was %q, want the apiserver's own words", finding.Detail)
	}
	if object := onlyObject(t, report, "apiserver-says-deprecated"); object.Name != "v1 Endpoints" {
		t.Fatalf("the finding was filed under %q, want what the warning is about", object.Name)
	}
}

func TestAWarningInAShapeNobodyPromisedStillReports(t *testing.T) {
	report := withFacts(t, Facts{Warnings: []string{"something the apiserver felt like saying"}})

	if object := onlyObject(t, report, "apiserver-says-deprecated"); object.Name != "the apiserver" {
		t.Fatalf("the finding was filed under %q", object.Name)
	}
}

func TestAClusterWithNothingToSayReportsNothing(t *testing.T) {
	report := withFacts(t, Facts{ServerVersion: "v1.36.0"})

	for _, id := range []string{"serves-a-removed-api", "apiserver-says-deprecated"} {
		if findingCount(t, report, id) != 0 {
			t.Fatalf("%s fired on a cluster with nothing to report", id)
		}
	}
}
