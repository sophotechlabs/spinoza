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

func TestAMalformedMinorVersionIsNotGuessed(t *testing.T) {
	for _, version := range []string{"v1.next", "1.36beta", "v1.+"} {
		if got := minorOf(version); got != 0 {
			t.Fatalf("minorOf(%q) = %d, want 0", version, got)
		}
	}
}

func TestEveryRemovalTheMarkersNameIsCaughtOnAnOldEnoughCluster(t *testing.T) {
	const running = 21

	served := make([]string, 0, len(removals()))
	coming := 0
	for _, one := range removals() {
		served = append(served, one.groupVersion)
		if one.minor > running {
			coming++
		}
	}

	report := withFacts(t, Facts{ServerVersion: "v1.21.0", ServedVersions: served})

	if got := groupNamed(t, report, "serves-a-removed-api").Total; got != coming {
		t.Fatalf("reported %d of the %d removals ahead of 1.%d", got, coming, running)
	}
}

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

func TestTheRemovalTableComesFromTheApiThisRepoBuildsAgainst(t *testing.T) {
	newest := 0
	for _, one := range removals() {
		if one.minor > newest {
			newest = one.minor
		}
	}

	client := clientMinor(t)
	if newest < client {
		t.Fatalf("this repo builds against 1.%d and the newest removal it knows is 1.%d; "+
			"the table is no longer being read from the k8s.io/api markers", client, newest)
	}
	if newest-client > aheadAtMost {
		t.Fatalf("the newest removal is 1.%d against a 1.%d client, which is further ahead than "+
			"k8s.io/api has ever marked; check how the markers are being read", newest, client)
	}
}

const aheadAtMost = 12

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
