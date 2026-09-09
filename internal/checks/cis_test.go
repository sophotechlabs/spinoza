package checks

import (
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func controlNamed(t *testing.T, posture api.FrameworkPosture, id string) api.FrameworkControl {
	t.Helper()
	for _, one := range posture.Controls {
		if one.Control == id {
			return one
		}
	}
	t.Fatalf("no control named %q among the %d the posture carried", id, len(posture.Controls))
	return api.FrameworkControl{}
}

func builtInIDs() map[string]bool {
	out := map[string]bool{}
	for _, entry := range registry() {
		out[entry.id] = true
	}
	return out
}

func cisNamed() map[string]bool {
	out := map[string]bool{}
	for _, one := range catalog() {
		if one.framework != cisBenchmark {
			continue
		}
		for _, id := range one.checks {
			out[id] = true
		}
	}
	return out
}

func TestNoTwoControlsInTheCatalogShareAnID(t *testing.T) {
	seen := map[string]bool{}
	for _, one := range catalog() {
		key := one.framework + "\x00" + one.id
		if seen[key] {
			t.Fatalf("%s carries %s twice", one.framework, one.id)
		}
		seen[key] = true
	}
}

func TestEveryCheckAControlNamesIsARuleTheAuditShips(t *testing.T) {
	built := builtInIDs()
	for _, one := range catalog() {
		named := map[string]bool{}
		for _, id := range one.checks {
			if !built[id] {
				t.Fatalf("%s %s names %q, which is not a check the audit ships", one.framework, one.id, id)
			}
			if named[id] {
				t.Fatalf("%s %s names %q twice", one.framework, one.id, id)
			}
			named[id] = true
		}
	}
}

func TestTheCatalogHoldsAllThreeStatesOfCoverage(t *testing.T) {
	counted := map[string]int{}
	for _, one := range catalog() {
		counted[one.state()]++
	}
	for _, state := range []string{controlCovered, controlUncovered, controlOutOfScope} {
		if counted[state] == 0 {
			t.Fatalf("the catalog holds no %s control, so that state is never shown", state)
		}
	}
}

func TestAControlWithNoCheckBehindItIsReportedAsUncovered(t *testing.T) {
	uncovered := []string{}
	for _, one := range catalog() {
		if one.state() == controlUncovered {
			uncovered = append(uncovered, one.id)
		}
	}
	posture := Posture(report(t), cisBenchmark)
	for _, id := range uncovered {
		one := controlNamed(t, posture, id)
		if one.Covered {
			t.Fatalf("%s has no check behind it and still reports as covered", id)
		}
		if len(one.Checks) != 0 {
			t.Fatalf("%s has no check behind it and still names %v", id, one.Checks)
		}
		if one.Title == "" {
			t.Fatalf("%s came back without a title, so nothing says what is missing", id)
		}
	}
	if len(uncovered) == 0 {
		t.Fatal("no control is uncovered, so the uncovered state is never exercised")
	}
}

func TestAnOutOfScopeControlCarriesTheReasonItCannotBeDecided(t *testing.T) {
	posture := Posture(report(t), cisBenchmark)
	seen := 0
	for _, one := range catalog() {
		if one.state() != controlOutOfScope {
			continue
		}
		seen++
		if len(one.checks) != 0 {
			t.Fatalf("%s is out of scope and still names %v", one.id, one.checks)
		}
		carried := controlNamed(t, posture, one.id)
		if carried.Scope != controlOutOfScope {
			t.Fatalf("%s came back in scope %q", one.id, carried.Scope)
		}
		if carried.Reason != one.outOfScope {
			t.Fatalf("%s reads %q, which drops the reason %q", one.id, carried.Reason, one.outOfScope)
		}
		if carried.Title != one.title {
			t.Fatalf("%s title reads %q, want %q", one.id, carried.Title, one.title)
		}
	}
	if seen == 0 {
		t.Fatal("no control is out of scope, so the reason is never shown")
	}
}

func TestTheCISLabelAndTheCatalogNameTheSameRules(t *testing.T) {
	named := cisNamed()
	labeled := map[string]bool{}
	for _, entry := range registry() {
		if !slices.Contains(entry.frameworks, cisBenchmark) {
			continue
		}
		labeled[entry.id] = true
		if !named[entry.id] {
			t.Fatalf("%s carries the CIS label and no control in the catalog names it", entry.id)
		}
	}
	for id := range named {
		if !labeled[id] {
			t.Fatalf("a CIS control names %s and the rule does not carry the CIS label", id)
		}
	}
	if len(labeled) == 0 {
		t.Fatal("no rule carries the CIS label at all")
	}
}

func privilegedPod() *unstructured.Unstructured {
	return deployment("api", podSpec(container("app", withSecurity(map[string]any{"privileged": true}))))
}

func TestAControlCountsWhatTheChecksBehindItFound(t *testing.T) {
	found := report(t, privilegedPod())
	posture := Posture(found, cisBenchmark)

	tests := []struct {
		control string
		failing int
		objects int
	}{
		{control: "5.2.2", failing: 1, objects: 1},
		{control: "5.2.6", failing: 1, objects: 1},
		{control: "5.2.4", failing: 0, objects: 0},
	}
	for _, one := range tests {
		t.Run(one.control, func(t *testing.T) {
			got := controlNamed(t, posture, one.control)
			if got.Failing != one.failing {
				t.Fatalf("failing = %d, want %d", got.Failing, one.failing)
			}
			if got.Objects != one.objects {
				t.Fatalf("objects = %d, want %d", got.Objects, one.objects)
			}
			if !got.Covered {
				t.Fatal("the control reports as uncovered")
			}
		})
	}
}

func TestAControlAddsUpTheChecksBehindItRatherThanOneOfThem(t *testing.T) {
	found := report(t, privilegedPod())
	posture := Posture(found, cisBenchmark)

	one := controlNamed(t, posture, "5.2.9")
	behind := 0
	for _, id := range one.Checks {
		behind += groupNamed(t, found, id).Total
	}
	if one.Failing != behind {
		t.Fatalf("5.2.9 counted %d against %d across %v", one.Failing, behind, one.Checks)
	}
	if behind == 0 {
		t.Fatal("nothing failed behind 5.2.9, so the sum proves nothing")
	}
}

func TestAControlWhoseChecksDidNotRunSaysItsCountsArePartial(t *testing.T) {
	stood := api.CheckReport{Groups: []api.CheckGroup{{
		ID:      "privileged-containers",
		Skipped: "not audited: the cluster would not let this read pods",
	}}}

	posture := Posture(stood, cisBenchmark)

	if !strings.Contains(posture.Reason, "5.2.2") {
		t.Fatalf("reason = %q, which does not name the control nothing decided", posture.Reason)
	}
	if got := controlNamed(t, posture, "5.2.2").Failing; got != 0 {
		t.Fatalf("failing = %d on a check that never ran", got)
	}
}

func TestAReportThatRanEveryCheckIsNotMarkedPartial(t *testing.T) {
	posture := Posture(report(t), cisBenchmark)

	if strings.Contains(posture.Reason, "counts are partial") {
		t.Fatalf("reason = %q on a run where every check reported", posture.Reason)
	}
}

func TestThePostureListsEveryFrameworkTheCatalogKnows(t *testing.T) {
	posture := Posture(report(t), "")

	for _, want := range []string{pssBaseline, pssRestricted, nsaCisa, cisBenchmark} {
		if !slices.Contains(posture.Frameworks, want) {
			t.Fatalf("frameworks = %v, which leaves out %s", posture.Frameworks, want)
		}
	}
}

func TestNarrowingToOneFrameworkLeavesTheOthersOut(t *testing.T) {
	tests := []struct {
		name  string
		asked string
	}{
		{name: "cis", asked: cisBenchmark},
		{name: "baseline", asked: pssBaseline},
		{name: "restricted", asked: pssRestricted},
	}
	for _, one := range tests {
		t.Run(one.name, func(t *testing.T) {
			posture := Posture(report(t), one.asked)
			if len(posture.Controls) == 0 {
				t.Fatalf("%s narrowed to nothing", one.asked)
			}
			for _, control := range posture.Controls {
				if control.Framework != one.asked {
					t.Fatalf("asked for %s and got a control of %s", one.asked, control.Framework)
				}
			}
			if !slices.Contains(posture.Frameworks, nsaCisa) {
				t.Fatalf("narrowing dropped %s from the framework list", nsaCisa)
			}
		})
	}
}

func TestAFrameworkWithNoNumberedControlsSaysSoRatherThanAnsweringEmpty(t *testing.T) {
	posture := Posture(report(t), nsaCisa)

	if len(posture.Controls) != 0 {
		t.Fatalf("%s carried %d controls", nsaCisa, len(posture.Controls))
	}
	if !strings.Contains(posture.Reason, nsaCisa) {
		t.Fatalf("reason = %q, which does not say why the list is empty", posture.Reason)
	}
}

func TestAFrameworkNobodyKnowsIsNamedRatherThanAnsweredEmpty(t *testing.T) {
	posture := Posture(report(t), "SOC 2")

	if len(posture.Controls) != 0 {
		t.Fatalf("an unknown framework carried %d controls", len(posture.Controls))
	}
	if !strings.Contains(posture.Reason, "SOC 2") {
		t.Fatalf("reason = %q, which does not name what was asked for", posture.Reason)
	}
	if len(posture.Frameworks) == 0 {
		t.Fatal("an unknown framework hid the frameworks that are known")
	}
}

func TestEveryPSSControlUpstreamShipsIsInTheCatalog(t *testing.T) {
	held := map[string]bool{}
	for _, one := range catalog() {
		if one.framework == pssBaseline || one.framework == pssRestricted {
			held[one.id] = true
		}
	}
	for _, entry := range registry() {
		if entry.upstream == "" {
			continue
		}
		if !held[string(entry.upstream)] {
			t.Fatalf("%s answers upstream control %s and the catalog does not list it",
				entry.id, entry.upstream)
		}
	}
}
