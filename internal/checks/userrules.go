package checks

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// UserRule is a check written by whoever runs Spinoza rather than shipped with
// it. The expression is CEL, the language Kubernetes itself chose for
// admission policy, evaluated with the workload bound to `object`.
type UserRule struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Match    string `json:"match"`
	Expr     string `json:"expr"`
	Wrong    string `json:"wrong"`
	Remedy   string `json:"remedy"`
	// Silences names a check this rule quietens instead of adding one of its
	// own. Where the expression matches, that check's finding is reported as
	// silenced with Reason, the same way the audit silences what an operator
	// reads without naming it.
	Silences string `json:"silences,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func (r UserRule) silencer() bool {
	return r.Silences != ""
}

const userRuleObject = ScopeObject

// RulesKey is where the settings store holds the rules you wrote yourself.
const RulesKey = "spinoza.checks.rules.v1"

// ParseRules reads what the settings store holds. A store that holds nothing,
// or holds something that is not a rule list, yields no rules rather than an
// error: the audit is not the place to argue about the settings file.
func ParseRules(raw string) []UserRule {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []UserRule
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	kept := make([]UserRule, 0, len(out))
	for _, one := range out {
		if one.ID == "" || one.Expr == "" {
			continue
		}
		kept = append(kept, one)
	}
	return kept
}

// RuleFault is one rule the audit would refuse, named so the editor can say
// which line is wrong before the rule is saved rather than after the next audit.
type RuleFault = api.RuleFault

// Faults reads a rule list the way the audit will and reports what it would
// refuse. An unreadable list is one fault about the list itself: an editor that
// says nothing about a typo is worse than one that says the wrong thing.
func Faults(raw string) []RuleFault {
	if strings.TrimSpace(raw) == "" {
		return []RuleFault{}
	}
	var out []UserRule
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []RuleFault{{Reason: "this is not a list of rules: " + err.Error()}}
	}
	faults := []RuleFault{}
	for at, one := range out {
		faults = append(faults, one.faults(at)...)
	}
	return faults
}

func (r UserRule) faults(at int) []RuleFault {
	name := r.ID
	if name == "" {
		name = "rule " + strconv.Itoa(at+1)
	}
	if r.ID == "" {
		return []RuleFault{{ID: name, Reason: "a rule needs an id of its own"}}
	}
	if r.Expr == "" {
		return []RuleFault{{ID: name, Reason: "a rule needs an expression to judge by"}}
	}
	if _, err := compileRule(r.Expr); err != nil {
		return []RuleFault{{ID: name, Reason: "the expression did not compile: " + err.Error()}}
	}
	if r.silencer() && !isCheck(r.Silences) {
		return []RuleFault{{ID: name, Reason: "no check goes by the name " + r.Silences}}
	}
	if r.silencer() && r.Reason == "" {
		return []RuleFault{{ID: name, Reason: "a rule that silences a check has to say why"}}
	}
	return nil
}

func isCheck(id string) bool {
	for _, entry := range registry() {
		if entry.id == id {
			return true
		}
	}
	return false
}

// Silencers are the rules that quieten a check rather than add one. They are
// kept apart from the rest so the registry only ever holds checks.
func Silencers(rules []UserRule) []UserRule {
	out := []UserRule{}
	for _, one := range rules {
		if one.silencer() {
			out = append(out, one)
		}
	}
	return out
}

func userChecks(rules []UserRule) []check {
	out := make([]check, 0, len(rules))
	for _, one := range rules {
		if one.silencer() {
			continue
		}
		out = append(out, one.asCheck())
	}
	return out
}

func (r UserRule) asCheck() check {
	entry := check{
		id:       r.ID,
		title:    r.titleOrID(),
		category: knownCategory(r.Category),
		severity: knownSeverity(r.Severity),
		wrong:    r.wrongOrDefault(),
		remedy:   r.remedyOrDefault(),
	}
	if entry.severity == "" {
		entry.severity = severityMedium
	}
	program, err := compileRule(r.Expr)
	if err != nil {
		entry.find = refuses(err.Error())
		return entry
	}
	entry.find = overSubjects(judgeWith(r, program))
	return entry
}

func (r UserRule) titleOrID() string {
	if r.Title == "" {
		return r.ID
	}
	return r.Title
}

func (r UserRule) wrongOrDefault() string {
	if r.Wrong == "" {
		return "One of your own rules matched this workload."
	}
	return r.Wrong
}

func (r UserRule) remedyOrDefault() string {
	if r.Remedy == "" {
		return "Change what the rule looks for, or change the rule."
	}
	return r.Remedy
}

func knownCategory(name string) string {
	if slices.Contains([]string{categorySecurity, categoryReliability, categoryEfficiency}, name) {
		return name
	}
	return categoryReliability
}

func compileRule(expr string) (cel.Program, error) {
	env, err := cel.NewEnv(cel.Variable(userRuleObject, cel.DynType))
	if err != nil {
		return nil, err
	}
	parsed, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	return env.Program(parsed)
}

// A rule that does not compile becomes a check that reports the compile error
// instead of findings, so a typo is visible in the view rather than silent.
func refuses(reason string) finder {
	return func(scan) []found {
		return []found{{
			subject: Subject{
				Ref:  api.ObjectRef{Version: "v1", Resource: "rules", Name: "this rule"},
				Kind: "Rule",
			},
			detail: "the expression did not compile: " + reason,
		}}
	}
}

// A rule that matches is a finding; a rule that errors on one object is quiet
// about that object and keeps going. Neither may take the audit down with it.
func judgeWith(rule UserRule, program cel.Program) subjectRule {
	return func(subject Subject) (string, string) {
		if !rule.matches(subject) {
			return "", ""
		}
		value, _, err := program.Eval(map[string]any{userRuleObject: subject.Object.Object})
		if err != nil {
			return "", ""
		}
		if !truthy(value) {
			return "", ""
		}
		return "matches " + rule.ID, ""
	}
}

// holds runs the rule against one object. A rule that cannot be compiled, or
// that errors on this object, holds for nothing: a broken silencer must not
// quieten a check by accident.
func (r UserRule) holds(subject Subject) bool {
	program, err := compileRule(r.Expr)
	if err != nil {
		return false
	}
	value, _, evalErr := program.Eval(map[string]any{userRuleObject: subject.Object.Object})
	if evalErr != nil {
		return false
	}
	return truthy(value)
}

func (r UserRule) matches(subject Subject) bool {
	if r.Match == "" || r.Match == anything {
		return true
	}
	return r.Match == subject.Kind
}

func truthy(value ref.Val) bool {
	found, ok := value.(types.Bool)
	return ok && bool(found)
}
