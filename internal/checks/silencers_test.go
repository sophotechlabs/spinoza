package checks

import (
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

// what a rule of your own can quieten

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

// what a silencing rule is never allowed to do

func TestASilencingRuleThatDoesNotCompileQuietensNothing(t *testing.T) {
	raw := `[{"id":"broken","silences":"privileged-containers","expr":"object.nope(","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatal("a rule that does not compile silenced a finding")
	}
}

func TestASilencingRuleThatErrorsOnAnObjectQuietensNothing(t *testing.T) {
	raw := `[{"id":"reaches","silences":"privileged-containers","expr":"object.spec.nothingHere == 1","reason":"x"}]`

	report := withSilencers(t, raw, privilegedDaemonSet("agent"))

	if finding := onlyFinding(t, report, privilegedCheck); finding.Muted {
		t.Fatal("a rule that errored on the object silenced the finding anyway")
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

// what the editor is told about a silencing rule

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

func TestASilencingRuleThatReadsHasNoFaults(t *testing.T) {
	if faults := Faults(silencingRule); len(faults) != 0 {
		t.Fatalf("a rule that reads was reported as %v", faults)
	}
}
