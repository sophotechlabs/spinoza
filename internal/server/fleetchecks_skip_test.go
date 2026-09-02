package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func (p *panicking) CheckPage(context.Context, string, string, checks.Filter) (api.CheckPage, error) {
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

func TestFleetCheckPagesContinueEveryClusterAndKeepProvenance(t *testing.T) {
	firstReport := reportWith(group("limits-missing", 0))
	firstReport.Groups[0].Next = "after-p-mk1"
	secondReport := reportWith(group("limits-missing", 0))
	secondReport.Groups[0].Next = "after-p-mk2"
	first := &listing{
		report: firstReport,
		page: api.CheckPage{
			Findings: []api.CheckFinding{{Ref: 0, Detail: "first"}},
			Objects:  []api.CheckObject{object("api-later")},
			Next:     "again-p-mk1",
		},
	}
	second := &listing{
		report: secondReport,
		page: api.CheckPage{
			Findings: []api.CheckFinding{{Ref: 0, Detail: "second"}},
			Objects:  []api.CheckObject{object("worker-later")},
		},
	}
	ts := listServer(t, first, second)

	var report api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &report)
	cursor := report.Groups[0].Next
	if cursor == "" || cursor == "after-p-mk1" || cursor == "after-p-mk2" {
		t.Fatalf("next = %q, want an opaque fleet cursor", cursor)
	}

	var page api.CheckPage
	path := "/api/checks/findings/fleet?check=limits-missing&skipNamespaces=kube-system&after=" + url.QueryEscape(cursor)
	readFleet(t, ts, path, &page)

	if first.pageID != "limits-missing" || first.pageAfter != "after-p-mk1" {
		t.Fatalf("first request = %q after %q", first.pageID, first.pageAfter)
	}
	if second.pageID != "limits-missing" || second.pageAfter != "after-p-mk2" {
		t.Fatalf("second request = %q after %q", second.pageID, second.pageAfter)
	}
	if !slices.Equal(first.pageKeep.SkipNamespaces, []string{"kube-system"}) {
		t.Fatalf("filter = %+v, want the displayed namespace exclusions", first.pageKeep)
	}
	if len(page.Objects) != 2 || page.Objects[0].Cluster != mk1 || page.Objects[1].Cluster != mk2 {
		t.Fatalf("objects = %+v, want both source clusters", page.Objects)
	}
	if len(page.Findings) != 2 || page.Findings[0].Ref != 0 || page.Findings[1].Ref != 1 {
		t.Fatalf("findings = %+v, want references shifted into the merged object list", page.Findings)
	}
	next, err := decodeFleetCheckCursor(page.Next)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	if len(next.After) != 1 || next.After[mk1] != "again-p-mk1" {
		t.Fatalf("next = %+v, want only the cluster with findings left", next.After)
	}
}

func TestFleetCheckPageDoesNotAskAClusterWithNothingLeft(t *testing.T) {
	firstReport := reportWith(group("limits-missing", 0))
	firstReport.Groups[0].Next = "after-p-mk1"
	first := &listing{report: firstReport, page: api.CheckPage{}}
	second := &listing{report: reportWith(group("limits-missing", 0))}
	ts := listServer(t, first, second)

	var report api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &report)
	var page api.CheckPage
	readFleet(t, ts, "/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(report.Groups[0].Next), &page)

	if first.pageCalls != 1 {
		t.Fatalf("first page calls = %d, want 1", first.pageCalls)
	}
	if second.pageCalls != 0 {
		t.Fatalf("second page calls = %d, want none", second.pageCalls)
	}
}

func TestFleetCheckPageRejectsInvalidCursors(t *testing.T) {
	ts := listServer(t, &listing{}, &listing{})
	otherCheck := encodeFleetCheckCursor("other", map[string]string{mk1: "after"})
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"missing", "check=limits-missing", "after is required"},
		{"malformed", "check=limits-missing&after=not-a-cursor", "cursor is invalid"},
		{"another check", "check=limits-missing&after=" + url.QueryEscape(otherCheck), "another check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/checks/findings/fleet?"+tt.query, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
			}
			if !contains(string(body), tt.want) {
				t.Fatalf("body = %s, want %q", body, tt.want)
			}
		})
	}
}

func TestFleetCheckPageRejectsIncompleteCursorPayloads(t *testing.T) {
	ts := listServer(t, &listing{}, &listing{})
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid JSON", raw: "{"},
		{name: "missing check", raw: `{"after":{"cluster":"position"}}`},
		{name: "missing positions", raw: `{"check":"limits-missing"}`},
		{name: "empty cluster", raw: `{"check":"limits-missing","after":{"":"position"}}`},
		{name: "empty position", raw: `{"check":"limits-missing","after":{"cluster":""}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor := base64.RawURLEncoding.EncodeToString([]byte(tt.raw))
			resp, body := doRequest(t, http.MethodGet,
				ts.URL+"/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(cursor), nil)

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
			}
			if !contains(string(body), "cursor is invalid") {
				t.Fatalf("body = %s, want the cursor rejected", body)
			}
		})
	}
}

func TestFleetCheckPageRejectsACursorForAClosedCluster(t *testing.T) {
	ts := listServer(t, &listing{}, &listing{})
	cursor := encodeFleetCheckCursor("limits-missing", map[string]string{"https://gone:6443": "after"})

	resp, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(cursor), nil)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", resp.StatusCode, body)
	}
	if !contains(string(body), "no longer open") {
		t.Fatalf("body = %s, want the stale fleet named", body)
	}
}

func TestFleetCheckPageFailsWithoutAdvancingWhenAClusterFails(t *testing.T) {
	first := &listing{page: api.CheckPage{Findings: []api.CheckFinding{{Ref: 0}}, Objects: []api.CheckObject{object("api")}}}
	second := &listing{pageErr: errors.New("the API stopped answering")}
	ts := listServer(t, first, second)
	cursor := encodeFleetCheckCursor("limits-missing", map[string]string{mk1: "one", mk2: "two"})

	resp, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(cursor), nil)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", resp.StatusCode, body)
	}
	if !contains(string(body), "p-mk2") || !contains(string(body), "stopped answering") {
		t.Fatalf("body = %s, want the failed cluster named", body)
	}
}

func TestFleetCheckPageContainsAClusterPanic(t *testing.T) {
	ts := listServer(t, &listing{}, &panicking{})
	cursor := encodeFleetCheckCursor("limits-missing", map[string]string{mk2: "after"})

	resp, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(cursor), nil)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", resp.StatusCode, body)
	}
	if !contains(string(body), "p-mk2") || !contains(string(body), "could not finish") {
		t.Fatalf("body = %s, want the panicking cluster named", body)
	}
}

func TestFleetCheckPageDoesNotHideAClusterThatClosedDuringTheRequest(t *testing.T) {
	after := map[string]string{mk1: "one", mk2: "two"}
	found := []clusterAnswer[fleetCheckPageResult]{
		{cluster: mk1, context: "p-mk1", answer: fleetCheckPageResult{}},
	}

	_, err := mergeFleetCheckPages("limits-missing", after, found)

	if !errors.Is(err, errFleetChecksChanged) {
		t.Fatalf("error = %v, want a stale fleet cursor rejected", err)
	}
	if !contains(err.Error(), mk2) {
		t.Fatalf("error = %v, want the cluster that closed named", err)
	}
}

func TestAClusterThatPanickedIsNamedAndTheOthersStillAnswer(t *testing.T) {
	ts := listServer(t, &listing{report: reportWith(group("limits-missing", 0))}, &panicking{})

	var got api.CheckReport
	readFleet(t, ts, "/api/checks/fleet", &got)

	if len(got.Groups) != 1 || got.Groups[0].Total != 1 {
		t.Fatalf("groups = %+v, want the cluster that answered still counted", got.Groups)
	}
	if !contains(got.Error, "p-mk2") || !contains(got.Error, "could not finish") {
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
	if !contains(merged.Error, "p-mk2") || !contains(merged.Error, "could not finish") {
		t.Fatalf("error = %q, want the cluster that panicked named", merged.Error)
	}
}
