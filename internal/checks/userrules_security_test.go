package checks

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func encodedRules(t *testing.T, count int, expression string) string {
	t.Helper()
	rules := make([]UserRule, count)
	for at := range rules {
		rules[at] = UserRule{ID: "rule", Expr: expression}
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("encode rules: %v", err)
	}
	return string(encoded)
}

func TestRuleCountAcceptsTheLimitAndRejectsTheNextRule(t *testing.T) {
	if faults := Faults(encodedRules(t, maxUserRules, "true")); len(faults) != 0 {
		t.Fatalf("the maximum normal rule list was refused: %v", faults)
	}
	faults := Faults(encodedRules(t, maxUserRules+1, "true"))
	if len(faults) != 1 || faults[0].Reason != errTooManyUserRules.Error() {
		t.Fatalf("one extra rule returned %v", faults)
	}
}

func TestRuleExpressionAcceptsTheByteLimitAndRejectsTheNextByte(t *testing.T) {
	accepted := "true" + strings.Repeat(" ", maxUserRuleExpression-len("true"))
	if faults := Faults(encodedRules(t, 1, accepted)); len(faults) != 0 {
		t.Fatalf("the maximum expression was refused: %v", faults)
	}
	rejected := accepted + " "
	faults := Faults(encodedRules(t, 1, rejected))
	want := "an expression cannot be longer than 4096 bytes"
	if len(faults) != 1 || faults[0].Reason != want {
		t.Fatalf("one extra expression byte returned %v", faults)
	}
}

func TestSavedRulesCannotBypassRuleShapeLimits(t *testing.T) {
	if rules := ParseRules(encodedRules(t, maxUserRules+1, "true")); rules != nil {
		t.Fatalf("too many saved rules parsed as %d rules", len(rules))
	}
	overlong := strings.Repeat(" ", maxUserRuleExpression+1)
	if rules := ParseRules(encodedRules(t, 1, overlong)); len(rules) != 0 {
		t.Fatalf("an overlong saved rule parsed as %v", rules)
	}
}

func TestRuleListsRejectATrailingValue(t *testing.T) {
	faults := Faults(`[{"id":"safe","expr":"true"}] {}`)

	if len(faults) != 1 || !strings.Contains(faults[0].Reason, "trailing value") {
		t.Fatalf("faults = %v, want the trailing value refused", faults)
	}
}

func TestUserRuleEvaluationStopsAtItsCostLimit(t *testing.T) {
	items := make([]any, maxUserRuleCost*2)
	for at := range items {
		items[at] = "absent"
	}
	rule := UserRule{Expr: `object.items.exists(item, item == "found")`}
	subject := Subject{Object: &unstructured.Unstructured{Object: map[string]any{"items": items}}}
	_, err := rule.holds(subject)
	if err == nil || !strings.Contains(err.Error(), "actual cost limit exceeded") {
		t.Fatalf("cost-limited evaluation error = %v", err)
	}
}
