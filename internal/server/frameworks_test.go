package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func frameworkPosture(t *testing.T, at, framework string) api.FrameworkPosture {
	t.Helper()
	asked := at + "/api/checks/frameworks"
	if framework != "" {
		asked += "?framework=" + url.QueryEscape(framework)
	}
	var posture api.FrameworkPosture
	resp := getJSON(t, asked, &posture)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	return posture
}

func frameworkControl(t *testing.T, posture api.FrameworkPosture, id string) api.FrameworkControl {
	t.Helper()
	for _, one := range posture.Controls {
		if one.Control == id {
			return one
		}
	}
	t.Fatalf("no control named %q among the %d the posture carried", id, len(posture.Controls))
	return api.FrameworkControl{}
}

func frameworkExport(t *testing.T, at string) api.CheckReport {
	t.Helper()
	var report api.CheckReport
	exportBody(t, getRaw(t, at+"/api/checks/export?format=json"), &report)
	return report
}

func frameworkTotal(t *testing.T, report api.CheckReport, ids []string) (failing, muted int) {
	t.Helper()
	for _, id := range ids {
		group := groupIn(t, report, id)
		failing += group.Total
		muted += group.Muted
	}
	return failing, muted
}

func TestThePostureEndpointListsEveryFrameworkTheCatalogKnows(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	posture := frameworkPosture(t, ts.URL, "")

	want := []string{"PSS baseline", "PSS restricted", "NSA/CISA", "CIS Kubernetes Benchmark"}
	for _, one := range want {
		if !slices.Contains(posture.Frameworks, one) {
			t.Fatalf("frameworks = %v, which leaves out %s", posture.Frameworks, one)
		}
	}
	if len(posture.Controls) == 0 {
		t.Fatal("the posture carried no controls at all")
	}
}

func TestThePostureEndpointNarrowsToTheFrameworkAsked(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	whole := frameworkPosture(t, ts.URL, "")

	narrowed := frameworkPosture(t, ts.URL, "CIS Kubernetes Benchmark")

	if len(narrowed.Controls) == 0 {
		t.Fatal("narrowing to CIS left no controls")
	}
	if len(narrowed.Controls) >= len(whole.Controls) {
		t.Fatalf("narrowed to %d controls out of %d", len(narrowed.Controls), len(whole.Controls))
	}
	for _, one := range narrowed.Controls {
		if one.Framework != "CIS Kubernetes Benchmark" {
			t.Fatalf("asked for CIS and got a control of %s", one.Framework)
		}
	}
}

func TestThePostureCountsAgreeWithTheReportTheChecksViewShows(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	report := frameworkExport(t, ts.URL)
	posture := frameworkPosture(t, ts.URL, "CIS Kubernetes Benchmark")

	tests := []struct {
		control string
		checks  []string
	}{
		{control: "5.2.6", checks: []string{"privilege-escalation"}},
		{control: "5.2.9", checks: []string{"dangerous-capabilities", "capabilities-not-dropped"}},
		{control: "5.6.2", checks: []string{"seccomp-unset", "seccomp-unconfined"}},
	}
	for _, one := range tests {
		t.Run(one.control, func(t *testing.T) {
			failing, muted := frameworkTotal(t, report, one.checks)
			got := frameworkControl(t, posture, one.control)
			if got.Failing != failing {
				t.Fatalf("failing = %d, want the %d the report holds for %v", got.Failing, failing, one.checks)
			}
			if got.Muted != muted {
				t.Fatalf("muted = %d, want the %d the report holds for %v", got.Muted, muted, one.checks)
			}
			if failing == 0 {
				t.Fatalf("%v found nothing, so the agreement proves nothing", one.checks)
			}
		})
	}
}

func TestThePostureCountsAMutedFindingTheWayTheChecksViewDoes(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	before := frameworkControl(t, frameworkPosture(t, ts.URL, "CIS Kubernetes Benchmark"), "5.2.6")

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", api.Mute{
		Check: "privilege-escalation", Ref: "/v1/pods/prod/web-0", Reason: "it is a one-off",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	report := frameworkExport(t, ts.URL)
	after := frameworkControl(t, frameworkPosture(t, ts.URL, "CIS Kubernetes Benchmark"), "5.2.6")
	failing, muted := frameworkTotal(t, report, []string{"privilege-escalation"})
	if after.Failing != failing || after.Muted != muted {
		t.Fatalf("the posture said %d failing and %d muted against the report's %d and %d",
			after.Failing, after.Muted, failing, muted)
	}
	if after.Failing != before.Failing-1 || after.Muted != 1 {
		t.Fatalf("muting one finding moved 5.2.6 from %d failing to %d failing and %d muted",
			before.Failing, after.Failing, after.Muted)
	}
}

func TestThePostureNamesAFrameworkNobodyKnowsRatherThanAnsweringEmpty(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	posture := frameworkPosture(t, ts.URL, "SOC 2")

	if len(posture.Controls) != 0 {
		t.Fatalf("an unknown framework carried %d controls", len(posture.Controls))
	}
	if !strings.Contains(posture.Reason, "SOC 2") {
		t.Fatalf("reason = %q, which does not name what was asked for", posture.Reason)
	}
}

func TestThePostureSaysWhichControlsNothingCouldDecide(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	posture := frameworkPosture(t, ts.URL, "CIS Kubernetes Benchmark")

	if !strings.Contains(posture.Reason, "counts are partial") {
		t.Fatalf("reason = %q on a cluster where only pods were read", posture.Reason)
	}
	if !strings.Contains(posture.Reason, "5.1.1") {
		t.Fatalf("reason = %q, which does not name a control whose check stood down", posture.Reason)
	}
}
