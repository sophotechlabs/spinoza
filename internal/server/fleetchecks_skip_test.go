package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

type panicking struct {
	notStubbed
}

func (p *panicking) Checks(context.Context, checks.Filter) api.CheckReport {
	panic("the informer cache was swapped underneath")
}

func (p *panicking) Issues(context.Context) api.IssueQueue {
	panic("the informer cache was swapped underneath")
}

func skippedGroup(id, why string) api.CheckGroup {
	one := group(id)
	one.Skipped = why
	return one
}

func baselined(id string, refs ...int) api.CheckGroup {
	one := group(id, refs...)
	one.Baselined = true
	one.Ran = true
	one.Was = 1
	one.NewCount = 1
	return one
}

func reportWith(groups ...api.CheckGroup) api.CheckReport {
	return api.CheckReport{
		Groups:  groups,
		Objects: []api.CheckObject{object("api")},
		Scanned: 1,
	}
}

const noMetrics = "metrics-server did not answer, so usage is unknown"

func TestACheckOneClusterCouldNotRunIsNamedRatherThanFoldedIntoTheNumber(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportWith(group("requests-above-usage", 0))},
		&listing{report: reportWith(skippedGroup("requests-above-usage", noMetrics))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Groups[0].Total != 1 {
		t.Fatalf("total = %d, want what the cluster that ran it found", got.Groups[0].Total)
	}
	if got.Groups[0].Skipped != "" {
		t.Fatalf("skipped = %q, want no skip mark that would grey out a real count", got.Groups[0].Skipped)
	}
	want := []string{"p-mk2: " + noMetrics}
	if !slices.Equal(got.Groups[0].PartialOn, want) {
		t.Fatalf("partialOn = %v, want %v", got.Groups[0].PartialOn, want)
	}
	if contains(got.Error, "did not run there") {
		t.Fatalf("error = %q, want the stand-down on the row rather than the banner", got.Error)
	}
}

func TestAGroupAnotherClusterRanIsNotMarkedSkipped(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportWith(skippedGroup("requests-above-usage", noMetrics))},
		&listing{report: reportWith(group("requests-above-usage", 0))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Groups[0].Skipped != "" {
		t.Fatalf("skipped = %q, want no skip mark on a check another cluster ran", got.Groups[0].Skipped)
	}
	if got.Groups[0].Total != 1 {
		t.Fatalf("total = %d, want the finding from the cluster that ran it", got.Groups[0].Total)
	}
	if len(got.Groups[0].Findings) != 1 {
		t.Fatalf("findings = %d, want the one the cluster that ran it reported", len(got.Groups[0].Findings))
	}
	want := []string{"p-mk1: " + noMetrics}
	if !slices.Equal(got.Groups[0].PartialOn, want) {
		t.Fatalf("partialOn = %v, want %v", got.Groups[0].PartialOn, want)
	}
}

func TestACheckNoClusterCouldRunKeepsSayingSo(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportWith(skippedGroup("requests-above-usage", noMetrics))},
		&listing{report: reportWith(skippedGroup("requests-above-usage", noMetrics))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Groups[0].Skipped != noMetrics {
		t.Fatalf("skipped = %q, want the reason kept when nobody ran it", got.Groups[0].Skipped)
	}
	if len(got.Groups[0].PartialOn) != 0 {
		t.Fatalf("partialOn = %v, want nothing partial about a check nobody ran", got.Groups[0].PartialOn)
	}
}

func TestTheFleetSaysWhichBaselineItIsComparingAgainst(t *testing.T) {
	first := reportWith(baselined("limits-missing", 0))
	first.Baseline = "2026-08-31T09:00:00Z"
	first.WasScanned = 3
	second := reportWith(baselined("limits-missing", 0))
	second.Baseline = "2026-08-30T09:00:00Z"
	second.WasScanned = 4
	ts := listServer(t, &listing{report: first}, &listing{report: second})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Baseline != "2026-08-30T09:00:00Z" {
		t.Fatalf("baseline = %q, want the oldest one the fleet is held against", got.Baseline)
	}
	if got.WasScanned != 7 {
		t.Fatalf("wasScanned = %d, want what both baselines held", got.WasScanned)
	}
	if !got.Groups[0].Baselined {
		t.Fatalf("baselined = false, want the flag kept when every cluster has one")
	}
	if got.Groups[0].NewCount != 2 {
		t.Fatalf("new = %d, want both clusters counted", got.Groups[0].NewCount)
	}
	if got.Groups[0].Was != 2 {
		t.Fatalf("was = %d, want both baselines counted", got.Groups[0].Was)
	}
}

func TestAClusterWithNoBaselineIsNamedRatherThanCountedAsCompared(t *testing.T) {
	first := reportWith(baselined("limits-missing", 0))
	first.Baseline = "2026-08-31T09:00:00Z"
	ts := listServer(t, &listing{report: first}, &listing{report: reportWith(group("limits-missing", 0))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Baseline != "2026-08-31T09:00:00Z" {
		t.Fatalf("baseline = %q, want the one that was taken", got.Baseline)
	}
	if got.Groups[0].Baselined {
		t.Fatalf("baselined = true, want no claim to compare a cluster that has no baseline")
	}
	if !contains(got.Error, "p-mk2: no baseline taken there") {
		t.Fatalf("error = %q, want the cluster with no baseline named", got.Error)
	}
}

func TestNoBaselineAnywhereSaysNothingAboutBaselines(t *testing.T) {
	ts := listServer(t,
		&listing{report: reportWith(group("limits-missing", 0))},
		&listing{report: reportWith(group("limits-missing", 0))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Baseline != "" {
		t.Fatalf("baseline = %q, want none claimed", got.Baseline)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want nothing said about baselines nobody took", got.Error)
	}
}

func TestAPageCursorFromOneClusterIsNotHandedOutForTheFleet(t *testing.T) {
	paged := reportWith(group("limits-missing", 0))
	paged.Groups[0].Next = "cursor-on-p-mk1"
	ts := listServer(t, &listing{report: paged}, &listing{report: reportWith(group("limits-missing", 0))})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if got.Groups[0].Next != "" {
		t.Fatalf("next = %q, want no per-cluster cursor on a merged group", got.Groups[0].Next)
	}
}

func TestAClusterThatPanickedIsNamedAndTheOthersStillAnswer(t *testing.T) {
	ts := listServer(t, &listing{report: reportWith(group("limits-missing", 0))}, &panicking{})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if len(got.Groups) != 1 || got.Groups[0].Total != 1 {
		t.Fatalf("groups = %+v, want the cluster that answered still counted", got.Groups)
	}
	if !contains(got.Error, "p-mk2") || !contains(got.Error, "panicked") {
		t.Fatalf("error = %q, want the cluster that panicked named", got.Error)
	}
}

func TestAPanicInOneClustersIssuesDoesNotTakeTheProcessDown(t *testing.T) {
	srv, _ := twoClusters(t, &queueBackend{queue: queueOf(issue("a", api.SeverityWarning))}, &panicking{})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/issues/fleet", nil)

	merged := fleetFrom(t, body)
	if len(merged.Rows) != 1 {
		t.Fatalf("rows = %d, want the cluster that answered still in the queue", len(merged.Rows))
	}
	if !contains(merged.Error, "p-mk2") || !contains(merged.Error, "panicked") {
		t.Fatalf("error = %q, want the cluster that panicked named", merged.Error)
	}
}
