package checks

import (
	"strings"
	"testing"
)

func TestARuleListThatReadsHasNoFaults(t *testing.T) {
	if faults := Faults(betaRule); len(faults) != 0 {
		t.Fatalf("a rule that compiles was reported as %v", faults)
	}
}

func TestNoRulesAtAllHasNoFaults(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		if faults := Faults(raw); len(faults) != 0 {
			t.Fatalf("%q was reported as %v", raw, faults)
		}
	}
}

func TestARuleThatDoesNotCompileIsNamedBeforeItIsSaved(t *testing.T) {
	faults := Faults(`[{"id":"broken","expr":"object.spec.nope("}]`)

	if len(faults) != 1 {
		t.Fatalf("reported %v", faults)
	}
	if faults[0].ID != "broken" {
		t.Fatalf("the fault named %q", faults[0].ID)
	}
	if !strings.Contains(faults[0].Reason, "did not compile") {
		t.Fatalf("the reason was %q", faults[0].Reason)
	}
}

func TestARuleThatDoesNotReturnTrueOrFalseIsNamedBeforeItIsSaved(t *testing.T) {
	faults := Faults(`[{"id":"a-name","expr":"object.metadata.name"}]`)

	if len(faults) != 1 {
		t.Fatalf("reported %v", faults)
	}
	if faults[0].ID != "a-name" {
		t.Fatalf("the fault named %q", faults[0].ID)
	}
	if !strings.Contains(faults[0].Reason, "return true or false") {
		t.Fatalf("the reason was %q", faults[0].Reason)
	}
}

func TestARuleMissingWhatEveryRuleNeedsIsNamed(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"no id", `[{"expr":"true"}]`, "id of its own"},
		{"no expression", `[{"id":"bare"}]`, "expression to judge by"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			faults := Faults(tc.raw)

			if len(faults) != 1 || !strings.Contains(faults[0].Reason, tc.want) {
				t.Fatalf("reported %v", faults)
			}
		})
	}
}

func TestARuleWithNoIdIsStillPointedAt(t *testing.T) {
	faults := Faults(`[{"id":"fine","expr":"true"},{"expr":"true"}]`)

	if len(faults) != 1 || faults[0].ID != "rule 2" {
		t.Fatalf("reported %v, want the second rule named by its place", faults)
	}
}

func TestSomethingThatIsNotARuleListIsOneFaultAboutTheList(t *testing.T) {
	faults := Faults("not json at all")

	if len(faults) != 1 || faults[0].ID != "" {
		t.Fatalf("reported %v", faults)
	}
	if !strings.Contains(faults[0].Reason, "not a list of rules") {
		t.Fatalf("the reason was %q", faults[0].Reason)
	}
}

func TestEveryFaultInAListIsReportedNotJustTheFirst(t *testing.T) {
	faults := Faults(`[{"id":"one","expr":"object.spec.nope("},{"id":"two","expr":"also bad ("}]`)

	if len(faults) != 2 {
		t.Fatalf("reported %v, want both rules named", faults)
	}
}
