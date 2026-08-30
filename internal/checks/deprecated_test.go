package checks

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// a tripwire, not a guarantee

func TestTheRemovalTableHasBeenLookedAtThisCycle(t *testing.T) {
	// The table is hand-written from what Kubernetes publishes each release.
	// Nothing keeps it current, so this fails once the client the repo builds
	// against has moved far enough past the newest entry that the table is
	// almost certainly missing a removal. Look at the release notes, add what
	// is new, and raise the bound.
	const lookedAtThrough = 32

	newest := 0
	for _, one := range removals {
		if one.minor > newest {
			newest = one.minor
		}
	}
	if newest != lookedAtThrough {
		t.Fatalf("the newest removal in the table is 1.%d and the table claims to be current "+
			"through 1.%d; reconcile the two", newest, lookedAtThrough)
	}

	client := clientMinor(t)
	if client-newest > staleAfter {
		t.Fatalf("this repo builds against 1.%d and the removal table stops at 1.%d; "+
			"read the release notes since and add what is missing", client, newest)
	}
}

// The releases the table may fall behind the client by before it is treated as
// out of date. Kubernetes removes a beta API every other release or so.
const staleAfter = 6

// clientMinor is the Kubernetes minor this repo builds against, read from the
// client-go requirement rather than written down beside the table: a number
// nobody updates measures nothing, and the whole point of the tripwire is to
// notice a bump that left the table behind.
func clientMinor(t *testing.T) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	found := regexp.MustCompile(`k8s\.io/client-go v0\.(\d+)\.`).FindSubmatch(body)
	if found == nil {
		t.Fatal("go.mod no longer requires k8s.io/client-go the way this reads it")
	}
	minor, convErr := strconv.Atoi(string(found[1]))
	if convErr != nil {
		t.Fatalf("client-go minor: %v", convErr)
	}
	return minor
}
