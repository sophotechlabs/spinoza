package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type UserRule struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Match    string `json:"match"`
	Expr     string `json:"expr"`
	Wrong    string `json:"wrong"`
	Remedy   string `json:"remedy"`
	Silences string `json:"silences,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func (r UserRule) silencer() bool {
	return r.Silences != ""
}

const userRuleObject = ScopeObject

const (
	maxUserRules          = 128
	maxUserRuleExpression = 4096
	maxUserRuleCost       = 10_000
)

var (
	errTooManyUserRules                         = fmt.Errorf("a rule list cannot contain more than %d rules", maxUserRules)
	userRuleEnvironment, userRuleEnvironmentErr = cel.NewEnv(cel.Variable(userRuleObject, cel.DynType))
)

const RulesKey = "spinoza.checks.rules.v1"

func ParseRules(raw string) []UserRule {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out, err := decodeRules(raw)
	if err != nil {
		return nil
	}
	kept := make([]UserRule, 0, len(out))
	for _, one := range out {
		if one.ID == "" || one.Expr == "" || len(one.Expr) > maxUserRuleExpression {
			continue
		}
		kept = append(kept, one)
	}
	return kept
}

type RuleFault = api.RuleFault

func Faults(raw string) []RuleFault {
	if strings.TrimSpace(raw) == "" {
		return []RuleFault{}
	}
	out, err := decodeRules(raw)
	if err != nil {
		if errors.Is(err, errTooManyUserRules) {
			return []RuleFault{{Reason: err.Error()}}
		}
		return []RuleFault{{Reason: "this is not a list of rules: " + err.Error()}}
	}
	faults := []RuleFault{}
	for at, one := range out {
		faults = append(faults, one.faults(at)...)
	}
	return faults
}

func decodeRules(raw string) ([]UserRule, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := opening.(json.Delim)
	if !isDelim || delim != '[' {
		return nil, errors.New("the top-level value is not a list")
	}
	out := make([]UserRule, 0, min(maxUserRules, 16))
	for decoder.More() {
		if len(out) == maxUserRules {
			return nil, errTooManyUserRules
		}
		var one UserRule
		if decodeErr := decoder.Decode(&one); decodeErr != nil {
			return nil, decodeErr
		}
		out = append(out, one)
	}
	if _, closingErr := decoder.Token(); closingErr != nil {
		return nil, closingErr
	}
	var trailing any
	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("the list has a trailing value")
		}
		return nil, err
	}
	return out, nil
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
	if len(r.Expr) > maxUserRuleExpression {
		return []RuleFault{{ID: name, Reason: fmt.Sprintf("an expression cannot be longer than %d bytes", maxUserRuleExpression)}}
	}
	_, parsed, err := parseRule(r.Expr)
	if err != nil {
		return []RuleFault{{ID: name, Reason: err.Error()}}
	}
	if r.silencer() && !isCheck(r.Silences) {
		return []RuleFault{{ID: name, Reason: "no check goes by the name " + r.Silences}}
	}
	if r.silencer() && r.Reason == "" {
		return []RuleFault{{ID: name, Reason: "a rule that silences a check has to say why"}}
	}
	if r.silencer() && metadataOnlyCheck(r.Silences) && readsPastMetadata(parsed) {
		return []RuleFault{{
			ID: name,
			Reason: "this check only exposes object.apiVersion, object.kind, " +
				"object.metadata.name, and object.metadata.namespace to a silencer",
		}}
	}
	return nil
}

func metadataOnlyCheck(id string) bool {
	return slices.Contains([]string{
		"orphaned-config-map",
		"orphaned-secret",
		"claim-nothing-mounts",
	}, id)
}

func readsPastMetadata(parsed *cel.Ast) bool {
	root := celast.NavigateAST(parsed.NativeRep())
	nodes := append(celast.MatchDescendants(root, celast.AllMatcher()), root)
	for _, node := range nodes {
		path, rooted := objectPath(node)
		if !rooted {
			continue
		}
		parent, hasParent := node.Parent()
		if hasParent && extendsObjectPath(parent, node.ID()) {
			continue
		}
		if slices.Equal(path, []string{"apiVersion"}) {
			continue
		}
		if slices.Equal(path, []string{"kind"}) {
			continue
		}
		if slices.Equal(path, []string{"metadata", "name"}) {
			continue
		}
		if slices.Equal(path, []string{"metadata", "namespace"}) {
			continue
		}
		return true
	}
	return false
}

func extendsObjectPath(parent celast.Expr, child int64) bool {
	if _, rooted := objectPath(parent); !rooted {
		return false
	}
	if parent.Kind() == celast.SelectKind {
		return parent.AsSelect().Operand().ID() == child
	}
	if parent.Kind() != celast.CallKind {
		return false
	}
	call := parent.AsCall()
	if call.FunctionName() != "_[_]" || len(call.Args()) != 2 {
		return false
	}
	return call.Args()[0].ID() == child
}

func objectPath(expr celast.Expr) ([]string, bool) {
	switch expr.Kind() {
	case celast.IdentKind:
		if expr.AsIdent() == userRuleObject {
			return []string{}, true
		}
	case celast.SelectKind:
		path, rooted := objectPath(expr.AsSelect().Operand())
		if rooted {
			return append(path, expr.AsSelect().FieldName()), true
		}
	case celast.CallKind:
		call := expr.AsCall()
		if call.FunctionName() != "_[_]" || len(call.Args()) != 2 {
			return nil, false
		}
		path, rooted := objectPath(call.Args()[0])
		if !rooted || call.Args()[1].Kind() != celast.LiteralKind {
			return nil, false
		}
		field, isString := call.Args()[1].AsLiteral().(types.String)
		if !isString {
			return nil, false
		}
		return append(path, string(field)), true
	default:
		return nil, false
	}
	return nil, false
}

func isCheck(id string) bool {
	for _, entry := range registry() {
		if entry.id == id {
			return true
		}
	}
	return false
}

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
	if len(expr) > maxUserRuleExpression {
		return nil, fmt.Errorf("an expression cannot be longer than %d bytes", maxUserRuleExpression)
	}
	env, parsed, err := parseRule(expr)
	if err != nil {
		return nil, err
	}
	return env.Program(parsed, cel.CostLimit(maxUserRuleCost), cel.InterruptCheckFrequency(100))
}

func parseRule(expr string) (*cel.Env, *cel.Ast, error) {
	if userRuleEnvironmentErr != nil {
		return nil, nil, userRuleEnvironmentErr
	}
	parsed, issues := userRuleEnvironment.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, nil, fmt.Errorf("the expression did not compile: %w", issues.Err())
	}
	if !parsed.OutputType().IsExactType(cel.BoolType) {
		return nil, nil, fmt.Errorf("the expression has to return true or false, not %s",
			cel.FormatCELType(parsed.OutputType()))
	}
	return userRuleEnvironment, parsed, nil
}

func refuses(reason string) finder {
	return func(scan) []found {
		return []found{{
			subject: Subject{
				Ref:  api.ObjectRef{Version: "v1", Resource: "rules", Name: "this rule"},
				Kind: "Rule",
			},
			detail: reason,
		}}
	}
}

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

func (r UserRule) holds(subject Subject) (bool, error) {
	program, err := compileRule(r.Expr)
	if err != nil {
		return false, err
	}
	value, _, evalErr := program.Eval(map[string]any{userRuleObject: subject.Object.Object})
	if evalErr != nil {
		return false, evalErr
	}
	return truthy(value), nil
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
