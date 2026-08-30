package checks

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func subjectWith(origin string, replicas int64) Subject {
	return Subject{
		Ref:      api.ObjectRef{Namespace: "prod", Name: "api"},
		Kind:     "Deployment",
		Origin:   origin,
		Replicas: replicas,
	}
}

func TestAFindingOnYourOwnWorkloadKeepsTheChecksSeverity(t *testing.T) {
	got := severityFor(severityHigh, subjectWith("", 1))

	if got != severityHigh {
		t.Fatalf("severity = %q, want %q; nothing about this subject softens it", got, severityHigh)
	}
}

func TestAFindingOnASystemComponentDropsBelowYourOwn(t *testing.T) {
	mine := severityFor(severityHigh, subjectWith("", 1))
	theirs := severityFor(severityHigh, subjectWith(originSystem, 1))

	if baseWeight(theirs) >= baseWeight(mine) {
		t.Fatalf(
			"a kube-system component ranked %q against %q on your own workload; every "+
				"high-severity finding on p-mk1 was cilium, falco or an eBPF agent",
			theirs, mine,
		)
	}
}

func TestAPackagedWorkloadSitsBetweenYoursAndTheSystems(t *testing.T) {
	mine := baseWeight(severityFor(severityHigh, subjectWith("", 1)))
	packaged := baseWeight(severityFor(severityHigh, subjectWith(originPackaged, 1)))
	system := baseWeight(severityFor(severityHigh, subjectWith(originSystem, 1)))

	if mine <= packaged || packaged <= system {
		t.Fatalf(
			"ordering was yours=%d packaged=%d system=%d; a chart you install is one step "+
				"removed, a system component two", mine, packaged, system,
		)
	}
}

func TestRunningMoreThanOneReplicaWidensTheBlast(t *testing.T) {
	one := baseWeight(severityFor(severityMedium, subjectWith("", 1)))
	many := baseWeight(severityFor(severityMedium, subjectWith("", 3)))

	if many <= one {
		t.Fatalf("one replica ranked %d and three ranked %d, want the wider blast higher", one, many)
	}
}

func TestSeverityNeverLeavesTheThreeLevelsTheWireKnows(t *testing.T) {
	levels := map[string]bool{severityHigh: true, severityMedium: true, severityLow: true}
	for _, base := range []string{severityHigh, severityMedium, severityLow} {
		for _, origin := range []string{"", originPackaged, originSystem} {
			for _, replicas := range []int64{0, 1, 9} {
				got := severityFor(base, subjectWith(origin, replicas))
				if !levels[got] {
					t.Fatalf(
						"base %q, origin %q, %d replicas produced %q, which is not a level "+
							"the wire carries", base, origin, replicas, got,
					)
				}
			}
		}
	}
}

func TestTheWorstFindingsComeFirst(t *testing.T) {
	rule := check{severity: severityHigh}
	all := []found{
		{subject: subjectWith(originSystem, 1), container: "a"},
		{subject: subjectWith("", 1), container: "b"},
		{subject: subjectWith(originPackaged, 1), container: "c"},
	}

	ordered := rule.ranked(all)

	if ordered[0].container != "b" {
		t.Fatalf(
			"first finding was on %q; your own workload should lead, not a %s one",
			ordered[0].container, ordered[0].subject.Origin,
		)
	}
	if ordered[len(ordered)-1].container != "a" {
		t.Fatalf("last finding was %q, want the system component last", ordered[len(ordered)-1].container)
	}
}

func TestTheCursorKeyOrdersTheSameWayTheFindingsDo(t *testing.T) {
	rule := check{severity: severityHigh}
	all := rule.ranked([]found{
		{subject: subjectWith(originSystem, 1), container: "a"},
		{subject: subjectWith("", 1), container: "b"},
		{subject: subjectWith(originPackaged, 1), container: "c"},
	})

	for at := 1; at < len(all); at++ {
		if findingKey(all[at-1]) >= findingKey(all[at]) {
			t.Fatalf(
				"key %q did not sort before %q; paging skips with key <= after, so the key "+
					"has to carry the same order the findings are in",
				findingKey(all[at-1]), findingKey(all[at]),
			)
		}
	}
}
