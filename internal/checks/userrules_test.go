package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func withRules(t *testing.T, raw string, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Rules = ParseRules(raw)
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

const betaRule = `[{
  "id": "no-beta-images",
  "title": "Image tagged as a beta",
  "category": "reliability",
  "severity": "high",
  "match": "Deployment",
  "expr": "object.spec.template.spec.containers.exists(c, c.image.contains(\"-beta\"))",
  "wrong": "A beta build is running where a release should be.",
  "remedy": "Move it to a release tag."
}]`

func TestARuleOfYourOwnFiresOnWhatItMatches(t *testing.T) {
	report := withRules(t, betaRule,
		deployment("api", podSpec(container("app", map[string]any{"image": "ghcr.io/x/api:2.0-beta"}))))

	group := groupNamed(t, report, "no-beta-images")
	if group.Title != "Image tagged as a beta" {
		t.Fatalf("title was %q", group.Title)
	}
	if group.Severity != severityHigh || group.Category != categoryReliability {
		t.Fatalf("group was %s/%s", group.Category, group.Severity)
	}
	if group.Total != 1 {
		t.Fatalf("reported %d findings, want the one deployment it matches", group.Total)
	}
	if !strings.Contains(group.Wrong, "beta build") {
		t.Fatalf("wrong was %q, want the words the rule gave", group.Wrong)
	}
}

func TestARuleOfYourOwnLeavesWhatItDoesNotMatch(t *testing.T) {
	report := withRules(t, betaRule,
		deployment("api", podSpec(container("app", map[string]any{"image": "ghcr.io/x/api:2.0"}))))

	if findingCount(t, report, "no-beta-images") != 0 {
		t.Fatal("a release image was reported by the beta rule")
	}
}

func TestARuleOnlyJudgesTheKindItNames(t *testing.T) {
	report := withRules(t, betaRule,
		workload("StatefulSet", "db", podSpec(container("app", map[string]any{"image": "x:2.0-beta"}))))

	if findingCount(t, report, "no-beta-images") != 0 {
		t.Fatal("a rule matching Deployment judged a StatefulSet")
	}
}

func TestARuleNamingNoKindJudgesEverything(t *testing.T) {
	raw := `[{"id":"any-beta","expr":"object.kind == \"StatefulSet\""}]`

	report := withRules(t, raw, workload("StatefulSet", "db", podSpec(container("app", nil))))

	if findingCount(t, report, "any-beta") != 1 {
		t.Fatal("a rule naming no kind did not judge a StatefulSet")
	}
}

func TestARuleThatDoesNotCompileSaysSoInTheView(t *testing.T) {
	raw := `[{"id":"broken","expr":"object.spec.nope("}]`

	report := withRules(t, raw, deployment("api", podSpec(container("app", nil))))

	finding := onlyFinding(t, report, "broken")
	if !strings.Contains(finding.Detail, "did not compile") {
		t.Fatalf("detail was %q, want the compile error", finding.Detail)
	}
}

func TestARuleThatErrorsOnAnObjectIsQuietAboutIt(t *testing.T) {
	raw := `[{"id":"reaches-too-far","expr":"object.spec.template.spec.nothingIsHere == 1"}]`

	report := withRules(t, raw, deployment("api", podSpec(container("app", nil))))

	if findingCount(t, report, "reaches-too-far") != 0 {
		t.Fatal("a rule that errored on the object reported a finding anyway")
	}
}

func TestARuleReturningSomethingOtherThanTrueOrFalseReportsItsFault(t *testing.T) {
	raw := `[{"id":"counts","expr":"size(object.spec.template.spec.containers)"}]`

	report := withRules(t, raw, deployment("api", podSpec(container("app", nil))))

	finding := onlyFinding(t, report, "counts")
	if !strings.Contains(finding.Detail, "return true or false") {
		t.Fatalf("detail was %q, want the rule's output type fault", finding.Detail)
	}
}

func TestRulesAreReadFromWhatTheStoreHolds(t *testing.T) {
	rules := ParseRules(betaRule)

	if len(rules) != 1 || rules[0].ID != "no-beta-images" {
		t.Fatalf("read %v", rules)
	}
}

func TestAStoreHoldingNothingUsefulYieldsNoRules(t *testing.T) {
	for _, raw := range []string{
		"", "   ", "not json", `{"id":"not-a-list"}`, `[{"title":"no id"}]`,
		`[{"id":"no expression"}]`,
	} {
		if rules := ParseRules(raw); len(rules) != 0 {
			t.Fatalf("%q yielded %v", raw, rules)
		}
	}
}

func TestARuleWithNoTitleOrWordsStillReads(t *testing.T) {
	raw := `[{"id":"bare","expr":"true"}]`

	report := withRules(t, raw, deployment("api", podSpec(container("app", nil))))

	group := groupNamed(t, report, "bare")
	if group.Title != "bare" {
		t.Fatalf("title was %q, want the id standing in", group.Title)
	}
	if group.Severity != severityMedium {
		t.Fatalf("severity was %q, want medium as the default", group.Severity)
	}
	if group.Category != categoryReliability {
		t.Fatalf("category was %q, want reliability as the default", group.Category)
	}
	if group.Wrong == "" || group.Remedy == "" {
		t.Fatal("a rule with no words of its own was left with none")
	}
}

func TestYourOwnRuleCanBeTurnedOffLikeAnyOther(t *testing.T) {
	keep := wholeCluster()
	keep.Rules = ParseRules(betaRule)
	keep.Disabled = []string{"no-beta-images"}

	report := Run(t.Context(), newLister(
		deployment("api", podSpec(container("app", map[string]any{"image": "x:2.0-beta"}))),
	), descriptors(), api.Metrics{}, keep, 0)

	for _, group := range report.Groups {
		if group.ID == "no-beta-images" {
			t.Fatal("a rule of your own ignored being turned off")
		}
	}
}

func TestYourOwnRuleCanBePagedLikeAnyOther(t *testing.T) {
	keep := wholeCluster()
	keep.Rules = ParseRules(betaRule)

	page, err := Page(t.Context(), newLister(
		deployment("api", podSpec(container("app", map[string]any{"image": "x:2.0-beta"}))),
	), descriptors(), api.Metrics{}, "no-beta-images", "", keep, 0)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page.Findings) != 1 {
		t.Fatalf("paged %d findings", len(page.Findings))
	}
}
