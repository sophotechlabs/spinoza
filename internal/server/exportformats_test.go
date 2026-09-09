package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

func exportBody(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("the export was not parseable json: %v", err)
	}
}

func TestTheSARIFExportSaysItIsSARIFAndParses(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := getRaw(t, ts.URL+"/api/checks/export?format=sarif")

	if got := resp.Header.Get("Content-Type"); got != "application/sarif+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-checks.sarif") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	var doc checks.SARIFLog
	exportBody(t, resp, &doc)
	if doc.Version != "2.1.0" {
		t.Fatalf("version = %q, want 2.1.0", doc.Version)
	}
	if len(doc.Runs) != 1 || doc.Runs[0].Tool.Driver.Name != "spinoza" {
		t.Fatalf("the run named %+v", doc.Runs)
	}
	if len(doc.Runs[0].Tool.Driver.Rules) == 0 {
		t.Fatal("the export carried no rules at all")
	}
}

func TestEverySARIFExportResultPointsAtARuleTheFileCarries(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	var doc checks.SARIFLog
	exportBody(t, getRaw(t, ts.URL+"/api/checks/export?format=sarif"), &doc)

	rules := map[string]bool{}
	for _, rule := range doc.Runs[0].Tool.Driver.Rules {
		rules[rule.ID] = true
	}
	if len(doc.Runs[0].Results) == 0 {
		t.Fatal("the export carried no results at all")
	}
	for _, result := range doc.Runs[0].Results {
		if !rules[result.RuleID] {
			t.Fatalf("a result pointed at %q, which the file does not describe", result.RuleID)
		}
	}
}

func TestTheJSONExportSaysItIsJSONAndParses(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := getRaw(t, ts.URL+"/api/checks/export?format=json")

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-checks.json") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	var report api.CheckReport
	exportBody(t, resp, &report)
	if len(report.Groups) == 0 {
		t.Fatal("the export carried no check groups at all")
	}
}

func TestAnExportFormatNobodyOffersStillGetsTheSpreadsheet(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	cases := []struct {
		name  string
		query string
	}{
		{name: "no format at all", query: ""},
		{name: "the format the button asks for", query: "?format=csv"},
		{name: "a format nobody wrote", query: "?format=parquet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := getRaw(t, ts.URL+"/api/checks/export"+tc.query)

			if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
				t.Fatalf("Content-Type = %q", got)
			}
			if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-checks.csv") {
				t.Fatalf("Content-Disposition = %q", got)
			}
			if rows := readCSV(t, resp); rows[0][0] != "check" {
				t.Fatalf("the header row was %v", rows[0])
			}
		})
	}
}
