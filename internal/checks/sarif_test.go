package checks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func sarifSample() api.CheckReport {
	return api.CheckReport{
		Objects: []api.CheckObject{
			{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespace: "prod", Name: "web"},
			{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespace: "prod", Name: "api"},
			{Version: "v1", Resource: "clusterroles", Kind: "ClusterRole", Name: "spinoza-reader"},
		},
		Groups: []api.CheckGroup{
			{
				ID:         "privileged-container",
				Title:      "Runs a privileged container",
				Category:   "security",
				Severity:   severityHigh,
				Frameworks: []string{"PSS baseline", "NSA/CISA"},
				Wrong:      "a privileged container holds every capability the node has",
				Remedy:     "drop privileged from the container's security context",
				Total:      2,
				Findings: []api.CheckFinding{
					{Ref: 0, Container: "app", Detail: "runs privileged", Severity: severityHigh},
					{
						Ref: 1, Container: "sidecar", Detail: "runs privileged", Severity: severityHigh,
						Muted: true, MutedBy: ScopeObject, Reason: "the node agent needs it",
					},
				},
			},
			{
				ID:       "requests-missing",
				Title:    "Sets no CPU or memory request",
				Category: "efficiency",
				Severity: severityLow,
				Wrong:    "the scheduler cannot place what does not say what it needs",
				Remedy:   "set requests on every container",
				Total:    1,
				Findings: []api.CheckFinding{
					{Ref: 2, Detail: "sets no requests", Severity: severityLow},
				},
			},
		},
	}
}

func sarifRuleNamed(t *testing.T, doc SARIFLog, id string) SARIFRule {
	t.Helper()
	for _, rule := range doc.Runs[0].Tool.Driver.Rules {
		if rule.ID == id {
			return rule
		}
	}
	t.Fatalf("no rule for %q among %d rules", id, len(doc.Runs[0].Tool.Driver.Rules))
	return SARIFRule{}
}

func sarifResultNamed(t *testing.T, doc SARIFLog, id, container string) SARIFResult {
	t.Helper()
	for _, result := range doc.Runs[0].Results {
		if result.RuleID != id {
			continue
		}
		if container == "" || strings.Contains(result.Message.Text, "container "+container) {
			return result
		}
	}
	t.Fatalf("no result for %q container %q among %d results", id, container, len(doc.Runs[0].Results))
	return SARIFResult{}
}

func TestTheSARIFRunSaysWhoWroteItAndWhatItIs(t *testing.T) {
	doc := SARIF(sarifSample())

	if doc.Version != "2.1.0" {
		t.Fatalf("version = %q, want 2.1.0", doc.Version)
	}
	if doc.Schema == "" {
		t.Fatal("the document named no schema")
	}
	if len(doc.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(doc.Runs))
	}
	driver := doc.Runs[0].Tool.Driver
	if driver.Name != "spinoza" {
		t.Fatalf("driver name = %q", driver.Name)
	}
	if driver.InformationURI != "https://spinoza.tech" {
		t.Fatalf("informationUri = %q", driver.InformationURI)
	}
	if driver.Version == "" {
		t.Fatal("the driver did not say which build wrote the file")
	}
}

func TestTheSARIFExportCarriesOneRulePerCheckAndOneResultPerFinding(t *testing.T) {
	report := sarifSample()

	doc := SARIF(report)

	if len(doc.Runs[0].Tool.Driver.Rules) != len(report.Groups) {
		t.Fatalf("rules = %d, want %d", len(doc.Runs[0].Tool.Driver.Rules), len(report.Groups))
	}
	want := 0
	for _, group := range report.Groups {
		want += len(group.Findings)
	}
	if len(doc.Runs[0].Results) != want {
		t.Fatalf("results = %d, want %d", len(doc.Runs[0].Results), want)
	}
	for _, result := range doc.Runs[0].Results {
		sarifRuleNamed(t, doc, result.RuleID)
	}
}

func TestASARIFRuleCarriesWhatIsWrongAndTheRemedy(t *testing.T) {
	doc := SARIF(sarifSample())

	rule := sarifRuleNamed(t, doc, "privileged-container")
	if rule.Name != "privileged-container" {
		t.Fatalf("name = %q", rule.Name)
	}
	if rule.ShortDescription.Text != "Runs a privileged container" {
		t.Fatalf("shortDescription = %q", rule.ShortDescription.Text)
	}
	if !strings.Contains(rule.FullDescription.Text, "every capability") {
		t.Fatalf("fullDescription = %q", rule.FullDescription.Text)
	}
	if !strings.Contains(rule.Help.Text, "security context") {
		t.Fatalf("help = %q", rule.Help.Text)
	}
}

func TestASARIFRuleTagsTheCategoryAndEveryFramework(t *testing.T) {
	doc := SARIF(sarifSample())

	cases := []struct {
		name string
		id   string
		want []string
	}{
		{
			name: "a check carrying two frameworks",
			id:   "privileged-container",
			want: []string{"security", "PSS baseline", "NSA/CISA"},
		},
		{
			name: "a check carrying none",
			id:   "requests-missing",
			want: []string{"efficiency"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sarifRuleNamed(t, doc, tc.id).Properties.Tags
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tags = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSARIFLevelsFollowTheSeverityTable(t *testing.T) {
	cases := []struct {
		name     string
		severity string
		level    string
		problem  string
	}{
		{name: "high is an error", severity: severityHigh, level: "error", problem: "error"},
		{name: "medium is a warning", severity: severityMedium, level: "warning", problem: "warning"},
		{name: "low is a note", severity: severityLow, level: "note", problem: "recommendation"},
		{name: "a severity nobody declared is none", severity: "moderate", level: "none", problem: "recommendation"},
		{name: "no severity at all is none", severity: "", level: "none", problem: "recommendation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := api.CheckReport{
				Objects: []api.CheckObject{{Kind: "Pod", Namespace: "prod", Name: "web-0"}},
				Groups: []api.CheckGroup{{
					ID:       "some-check",
					Severity: tc.severity,
					Findings: []api.CheckFinding{{Ref: 0, Detail: "something is wrong"}},
				}},
			}

			doc := SARIF(report)

			rule := sarifRuleNamed(t, doc, "some-check")
			if rule.DefaultConfiguration.Level != tc.level {
				t.Fatalf("level = %q, want %q", rule.DefaultConfiguration.Level, tc.level)
			}
			if rule.Properties.Problem.Severity != tc.problem {
				t.Fatalf("problem.severity = %q, want %q", rule.Properties.Problem.Severity, tc.problem)
			}
			if doc.Runs[0].Results[0].Level != tc.level {
				t.Fatalf("result level = %q, want %q", doc.Runs[0].Results[0].Level, tc.level)
			}
		})
	}
}

func TestASARIFResultNamesTheObjectAndWhatIsWrong(t *testing.T) {
	doc := SARIF(sarifSample())

	result := sarifResultNamed(t, doc, "privileged-container", "app")
	if result.Message.Text != "Deployment prod/web container app: runs privileged" {
		t.Fatalf("message = %q", result.Message.Text)
	}
	if len(result.Locations) != 1 {
		t.Fatalf("locations = %d, want 1", len(result.Locations))
	}
	where := result.Locations[0].LogicalLocations[0]
	if where.FullyQualifiedName != "Deployment/prod/web" {
		t.Fatalf("fullyQualifiedName = %q", where.FullyQualifiedName)
	}
	if where.Name != "web" {
		t.Fatalf("logical location name = %q", where.Name)
	}
}

func TestASARIFResultOnAClusterScopedObjectStillSaysWhereItIs(t *testing.T) {
	doc := SARIF(sarifSample())

	result := sarifResultNamed(t, doc, "requests-missing", "")
	if result.Message.Text != "ClusterRole spinoza-reader: sets no requests" {
		t.Fatalf("message = %q", result.Message.Text)
	}
	if got := result.Locations[0].LogicalLocations[0].FullyQualifiedName; got != "ClusterRole/spinoza-reader" {
		t.Fatalf("fullyQualifiedName = %q", got)
	}
}

func TestTheSARIFExportInventsNoFileOrLine(t *testing.T) {
	body, err := json.Marshal(SARIF(sarifSample()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(body), "physicalLocation") {
		t.Fatal("the export made up a file and a line for a kubernetes object")
	}
	if !strings.Contains(string(body), "logicalLocations") {
		t.Fatal("the export said nothing about where the object is")
	}
}

func TestASARIFFingerprintIsTheSameOnASecondConversion(t *testing.T) {
	report := sarifSample()

	first := SARIF(report)
	second := SARIF(report)

	for at, result := range first.Runs[0].Results {
		got := second.Runs[0].Results[at].PartialFingerprints
		if !reflect.DeepEqual(result.PartialFingerprints, got) {
			t.Fatalf("result %d moved from %v to %v between runs", at, result.PartialFingerprints, got)
		}
		if len(result.PartialFingerprints) != 1 {
			t.Fatalf("result %d carried %d fingerprints", at, len(result.PartialFingerprints))
		}
	}
}

func TestASARIFFingerprintIsBlindToWhatChangesBetweenRuns(t *testing.T) {
	report := sarifSample()
	moved := sarifSample()
	moved.Groups[0].Findings[0].New = true
	moved.Groups[0].Total = 97
	moved.Groups[0].Findings[0].Detail = "runs privileged, seen again"
	moved.Baseline = "2026-09-09T10:00:00Z"

	before := SARIF(report).Runs[0].Results[0].PartialFingerprints
	after := SARIF(moved).Runs[0].Results[0].PartialFingerprints

	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the fingerprint moved from %v to %v when only the run changed", before, after)
	}
}

func TestASARIFFingerprintDiffersForADifferentFinding(t *testing.T) {
	doc := SARIF(sarifSample())
	seen := map[string]string{}

	for _, result := range doc.Runs[0].Results {
		mark := result.PartialFingerprints[sarifPrintKey]
		if was, twice := seen[mark]; twice {
			t.Fatalf("%s and %s share the fingerprint %s", was, result.Message.Text, mark)
		}
		seen[mark] = result.Message.Text
	}
	if len(seen) != len(doc.Runs[0].Results) {
		t.Fatalf("%d fingerprints for %d results", len(seen), len(doc.Runs[0].Results))
	}
}

func TestAMutedFindingIsCarriedAsASARIFSuppression(t *testing.T) {
	doc := SARIF(sarifSample())

	muted := sarifResultNamed(t, doc, "privileged-container", "sidecar")
	if len(muted.Suppressions) != 1 {
		t.Fatalf("suppressions = %d, want 1", len(muted.Suppressions))
	}
	if muted.Suppressions[0].Kind != "external" {
		t.Fatalf("suppression kind = %q", muted.Suppressions[0].Kind)
	}
	if muted.Suppressions[0].Justification != "the node agent needs it" {
		t.Fatalf("justification = %q", muted.Suppressions[0].Justification)
	}
	live := sarifResultNamed(t, doc, "privileged-container", "app")
	if len(live.Suppressions) != 0 {
		t.Fatalf("a live finding was suppressed: %v", live.Suppressions)
	}
}

func TestASARIFResultWhoseObjectIsOutOfRangeDoesNotPanic(t *testing.T) {
	cases := []struct {
		name string
		ref  int
	}{
		{name: "a reference below the first object", ref: -1},
		{name: "a reference one past the last object", ref: 1},
		{name: "a reference far past the last object", ref: 4096},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := api.CheckReport{
				Objects: []api.CheckObject{{Kind: "Pod", Namespace: "prod", Name: "web-0"}},
				Groups: []api.CheckGroup{{
					ID:       "some-check",
					Severity: severityHigh,
					Findings: []api.CheckFinding{{Ref: tc.ref, Detail: "something is wrong"}},
				}},
			}

			doc := SARIF(report)

			if len(doc.Runs[0].Results) != 1 {
				t.Fatalf("results = %d, want 1", len(doc.Runs[0].Results))
			}
			result := doc.Runs[0].Results[0]
			if len(result.Locations) != 0 {
				t.Fatalf("locations = %v, want none for an object nobody can point at", result.Locations)
			}
			if !strings.Contains(result.Message.Text, "did not carry") {
				t.Fatalf("message = %q", result.Message.Text)
			}
			if result.PartialFingerprints[sarifPrintKey] == "" {
				t.Fatal("the result carried no fingerprint")
			}
		})
	}
}

func TestTheSARIFDocumentRoundTripsThroughJSON(t *testing.T) {
	doc := SARIF(sarifSample())

	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back SARIFLog
	if decodeErr := json.Unmarshal(body, &back); decodeErr != nil {
		t.Fatalf("unmarshal: %v", decodeErr)
	}

	if !reflect.DeepEqual(doc, back) {
		t.Fatalf("the document came back different:\n%+v\n%+v", doc, back)
	}
	var loose map[string]any
	if decodeErr := json.Unmarshal(body, &loose); decodeErr != nil {
		t.Fatalf("the document is not plain json: %v", decodeErr)
	}
	if loose["$schema"] == "" || loose["version"] != "2.1.0" {
		t.Fatalf("the encoded document read %v", loose["version"])
	}
}

func TestAnEmptySARIFReportIsStillAValidDocument(t *testing.T) {
	doc := SARIF(api.CheckReport{})

	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"results":[]`) {
		t.Fatalf("an empty run did not carry an empty result list: %s", body)
	}
	if !strings.Contains(string(body), `"rules":[]`) {
		t.Fatalf("an empty run did not carry an empty rule list: %s", body)
	}
}

func TestSpinozaCanReadBackTheSARIFItWrote(t *testing.T) {
	doc := SARIF(sarifSample())
	written, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tool, rules, readErr := readSARIF(written)
	if readErr != nil {
		t.Fatalf("the export did not read back: %v", readErr)
	}
	if tool != "spinoza" {
		t.Fatalf("the tool read back as %q", tool)
	}
	if len(rules) == 0 {
		t.Fatal("the export read back with no rules")
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		for _, finding := range rule.findings {
			if finding.object.kind == "" || finding.object.name == "" {
				t.Fatalf("%s read back without an object: %+v", rule.id, finding.object)
			}
			seen[finding.object.kind+"/"+finding.object.namespace+"/"+finding.object.name] = true
		}
	}
	if !seen["Deployment/prod/web"] {
		t.Fatalf("the namespaced object did not survive the round trip; read back %v", seen)
	}
}
