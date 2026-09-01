package checks

import (
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const silencingRule = `[{
  "id": "the-node-agents",
  "silences": "privileged-containers",
  "match": "DaemonSet",
  "expr": "object.metadata.labels[\"tier\"] == \"system\"",
  "reason": "a node agent is meant to be privileged here"
}]`

func tagged(obj *unstructured.Unstructured, labels map[string]string) *unstructured.Unstructured {
	obj.SetLabels(labels)
	return obj
}

func privilegedDaemonSet(name string) *unstructured.Unstructured {
	return workload("DaemonSet", name, podSpec(container("app", map[string]any{
		"securityContext": map[string]any{"privileged": true},
	})))
}

func withSilencers(t *testing.T, raw string, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Rules = ParseRules(raw)
	keep.Silencers = Silencers(keep.Rules)
	keep.ShowMuted = true
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

func TestARuleOfYourOwnCanSilenceACheckWithAReason(t *testing.T) {
	report := withSilencers(t, silencingRule,
		tagged(privilegedDaemonSet("agent"), map[string]string{"tier": "system"}))

	finding := onlyFinding(t, report, privilegedCheck)
	if finding.MutedBy != ScopeRule {
		t.Fatalf("the finding was silenced as %q, want a rule", finding.MutedBy)
	}
	if !strings.Contains(finding.Reason, "node agent") {
		t.Fatalf("the reason was %q, want what the rule said", finding.Reason)
	}
}

func TestASilencingRuleLeavesWhatItDoesNotMatch(t *testing.T) {
	report := withSilencers(t, silencingRule,
		tagged(privilegedDaemonSet("agent"), map[string]string{"tier": "app"}))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatalf("a workload the rule does not match was silenced: %+v", finding)
	}
}

func TestASilencingRuleOnlyQuietensTheCheckItNames(t *testing.T) {
	report := withSilencers(t, silencingRule,
		tagged(privilegedDaemonSet("agent"), map[string]string{"tier": "system"}))

	group := groupNamed(t, report, "capabilities-not-dropped")
	if group.Total == 0 {
		t.Fatal("the control check stopped firing, so this proves nothing")
	}
	for _, finding := range group.Findings {
		if finding.Muted {
			t.Fatal("a rule silencing one check quietened another")
		}
	}
}

func TestASilencingRuleOnlyJudgesTheKindItNames(t *testing.T) {
	report := withSilencers(t, silencingRule,
		tagged(privilegedDeployment("api"), map[string]string{"tier": "system"}))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatal("a rule matching DaemonSet silenced a Deployment")
	}
}

func TestASilencingRuleIsNotACheckOfItsOwn(t *testing.T) {
	report := withSilencers(t, silencingRule, privilegedDaemonSet("agent"))

	for _, group := range report.Groups {
		if group.ID == "the-node-agents" {
			t.Fatal("a rule that silences a check was registered as a check")
		}
	}
}

func TestASilencingRuleThatDoesNotCompileQuietensNothing(t *testing.T) {
	raw := `[{"id":"broken","silences":"privileged-containers","expr":"object.nope(","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatal("a rule that does not compile silenced a finding")
	}
	if !strings.Contains(report.Error, `silencer "broken"`) ||
		!strings.Contains(report.Error, "DaemonSet "+testNamespace+"/agent") {
		t.Fatalf("error = %q, want the rule and object named", report.Error)
	}
}

func TestASilencingRuleThatErrorsOnAnObjectQuietensNothing(t *testing.T) {
	raw := `[{"id":"reaches","silences":"privileged-containers","expr":"object.spec.nothingHere == 1","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatal("a rule that errored on the object silenced the finding anyway")
	}
	if !strings.Contains(report.Error, `silencer "reaches"`) ||
		!strings.Contains(report.Error, "DaemonSet "+testNamespace+"/agent") ||
		!strings.Contains(report.Error, "nothingHere") {
		t.Fatalf("error = %q, want the rule, object, and evaluation fault", report.Error)
	}
}

func TestASilencingRuleThatEvaluatesFalseReportsNoFault(t *testing.T) {
	raw := `[{"id":"other","silences":"privileged-containers","expr":"object.metadata.name == 'elsewhere'","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"))

	if report.Error != "" {
		t.Fatalf("a false expression was reported as %q", report.Error)
	}
}

func TestRepeatedSilencerFailuresAreCountedWithoutRepeatingTheError(t *testing.T) {
	raw := `[{"id":"reaches","silences":"privileged-containers","expr":"object.spec.nothingHere == 1","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"), privilegedDaemonSet("other"))

	if strings.Count(report.Error, `silencer "reaches"`) != 1 {
		t.Fatalf("error = %q, want the rule named once", report.Error)
	}
	if !strings.Contains(report.Error, "and 1 other object") {
		t.Fatalf("error = %q, want the other affected object counted", report.Error)
	}
}

func TestSeveralFindingsOnOneObjectCountAsOneSilencerFailure(t *testing.T) {
	raw := `[{"id":"reaches","silences":"privileged-containers","expr":"object.spec.nothingHere == 1","reason":"x"}]`
	object := workload("DaemonSet", "agent", podSpec(
		container("one", map[string]any{"securityContext": map[string]any{"privileged": true}}),
		container("two", map[string]any{"securityContext": map[string]any{"privileged": true}}),
	))

	report := withSilencers(t, raw, object)

	if strings.Contains(report.Error, "other object") {
		t.Fatalf("one object with several findings was reported as several objects: %q", report.Error)
	}
}

func TestYourOwnMuteWinsOverASilencingRule(t *testing.T) {
	keep := wholeCluster()
	keep.Rules = ParseRules(silencingRule)
	keep.Silencers = Silencers(keep.Rules)
	keep.ShowMuted = true
	keep.Mutes = []Mute{{
		Check:  privilegedCheck,
		Ref:    RefKey(api.ObjectRef{Group: "apps", Version: "v1", Resource: "daemonsets", Namespace: testNamespace, Name: "agent"}),
		Reason: "I decided this one myself",
	}}

	report := Run(t.Context(), newLister(
		tagged(privilegedDaemonSet("agent"), map[string]string{"tier": "system"}),
	), descriptors(), api.Metrics{}, keep, 0)

	finding := onlyFinding(t, report, privilegedCheck)
	if finding.MutedBy != ScopeObject {
		t.Fatalf("the finding was silenced as %q, want the mute the operator made", finding.MutedBy)
	}
	if !strings.Contains(finding.Reason, "myself") {
		t.Fatalf("the reason was %q", finding.Reason)
	}
}

func TestASilencedFindingIsStillCountedAndStillInTheBaseline(t *testing.T) {
	objects := []*unstructured.Unstructured{
		tagged(privilegedDaemonSet("agent"), map[string]string{"tier": "system"}),
	}
	base := Fingerprint(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, wholeCluster())

	report := withSilencers(t, silencingRule, objects...)

	if group := groupNamed(t, report, privilegedCheck); group.Muted != 1 {
		t.Fatalf("counted %d silenced, want the one the rule quietened", group.Muted)
	}
	if base.Counts[privilegedCheck] != 1 {
		t.Fatalf("the baseline counted %d, want the finding a rule silences to still be recorded",
			base.Counts[privilegedCheck])
	}
}

func TestASilencingRuleNamingNoRealCheckIsNamedAsAFault(t *testing.T) {
	faults := Faults(`[{"id":"typo","silences":"no-such-check","expr":"true","reason":"x"}]`)

	if len(faults) != 1 || !strings.Contains(faults[0].Reason, "no check goes by the name") {
		t.Fatalf("reported %v", faults)
	}
}

func TestASilencingRuleWithNoReasonIsNamedAsAFault(t *testing.T) {
	faults := Faults(`[{"id":"quiet","silences":"privileged-containers","expr":"true"}]`)

	if len(faults) != 1 || !strings.Contains(faults[0].Reason, "has to say why") {
		t.Fatalf("reported %v", faults)
	}
}

func TestASilencerCannotReadFieldsTheFindingDoesNotCarry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check string
		expr  string
	}{
		{"a ConfigMap's data", "orphaned-config-map", `object.data.size() > 0`},
		{"a Secret's labels", "orphaned-secret", `object.metadata.labels["tier"] == "system"`},
		{"a claim's spec", "claim-nothing-mounts", `object.spec.storageClassName == "local"`},
		{"a field through brackets", "orphaned-config-map", `object["data"].size() > 0`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `[{"id":"quiet","silences":"` + tc.check + `","expr":` + strconv.Quote(tc.expr) +
				`,"reason":"expected here"}]`

			faults := Faults(raw)

			if len(faults) != 1 || !strings.Contains(faults[0].Reason, "only exposes") {
				t.Fatalf("reported %v", faults)
			}
		})
	}
}

func TestASilencerCanReadEveryFieldAMetadataOnlyFindingCarries(t *testing.T) {
	for _, expr := range []string{
		`object.apiVersion == 'v1' && object.kind == 'ConfigMap' && object.metadata.name == 'settings' && object.metadata.namespace == 'prod'`,
		`object["metadata"]["name"] == 'settings'`,
	} {
		raw := `[{"id":"quiet","silences":"orphaned-config-map","expr":` + strconv.Quote(expr) +
			`,"reason":"expected here"}]`

		if faults := Faults(raw); len(faults) != 0 {
			t.Fatalf("%q reported %v", expr, faults)
		}
	}
}

func TestASilencingRuleThatReadsHasNoFaults(t *testing.T) {
	if faults := Faults(silencingRule); len(faults) != 0 {
		t.Fatalf("a rule that reads was reported as %v", faults)
	}
}
