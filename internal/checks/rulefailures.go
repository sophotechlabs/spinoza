package checks

import (
	"fmt"
	"strings"
	"sync"
)

type ruleFailure struct {
	rule   string
	object string
	reason string
	seen   map[string]struct{}
}

type ruleDiagnostics struct {
	mu     sync.Mutex
	byRule map[string]int
	list   []ruleFailure
}

func newRuleDiagnostics() *ruleDiagnostics {
	return &ruleDiagnostics{byRule: map[string]int{}}
}

func (f *ruleDiagnostics) record(rule UserRule, subject Subject, err error) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := subject.Kind + "\x00" + RefKey(subject.Ref)
	at, seen := f.byRule[rule.ID]
	if seen {
		f.list[at].seen[key] = struct{}{}
		return
	}
	f.byRule[rule.ID] = len(f.list)
	f.list = append(f.list, ruleFailure{
		rule:   rule.ID,
		object: subject.Kind + " " + refLabel(subject.Ref),
		reason: err.Error(),
		seen:   map[string]struct{}{key: {}},
	})
}

func (f *ruleDiagnostics) message() string {
	if f == nil {
		return ""
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.list))
	for _, failure := range f.list {
		object := failure.object
		others := len(failure.seen) - 1
		if others > 0 {
			object += fmt.Sprintf(" and %d other %s", others, objectWord(others))
		}
		out = append(out, fmt.Sprintf("silencer %q could not evaluate for %s: %s",
			failure.rule, object, failure.reason))
	}
	return strings.Join(out, "; ")
}

func objectWord(count int) string {
	if count == 1 {
		return "object"
	}
	return "objects"
}
