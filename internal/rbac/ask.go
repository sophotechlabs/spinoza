package rbac

import (
	"slices"
	"strings"
)

// Ask is one question about a permission: may this verb be used on this
// resource, in this namespace. An empty namespace asks about everywhere.
type Ask struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
}

// Reaches answers whether a grant lets the verb through. A grant bound in one
// namespace never reaches another, whatever its rules say.
func (g Grant) Reaches(ask Ask) bool {
	if !g.covers(ask.Namespace) {
		return false
	}
	for _, rule := range g.Rules {
		if rule.Allows(ask) {
			return true
		}
	}
	return false
}

// A cluster-wide grant covers every namespace. A namespaced one covers its own,
// and a question about everywhere is answered by any namespace that says yes.
func (g Grant) covers(namespace string) bool {
	if g.Everywhere() {
		return true
	}
	if namespace == "" {
		return true
	}
	return g.Namespace == namespace
}

// Allows is the rule half of the answer. A rule that names particular objects
// still allows the verb — on those objects — so it counts here and the names
// are carried out so the reader can see the limit.
func (r Rule) Allows(ask Ask) bool {
	if len(r.Resources) == 0 {
		return false
	}
	if !covers(r.Verbs, ask.Verb) {
		return false
	}
	if !covers(r.Groups, ask.Group) {
		return false
	}
	return coversResource(r.Resources, ask.Resource)
}

// A subresource is written as resource/subresource in a rule, so pods/exec is
// matched whole. A bare * covers everything; pods alone does not cover
// pods/exec, which is exactly the distinction the question is asked for.
func coversResource(held []string, wanted string) bool {
	if slices.Contains(held, Anything) {
		return true
	}
	if slices.Contains(held, wanted) {
		return true
	}
	base, sub, split := strings.Cut(wanted, "/")
	if !split {
		return false
	}
	return slices.Contains(held, base+"/"+Anything) || slices.Contains(held, Anything+"/"+sub)
}

func covers(held []string, wanted string) bool {
	return slices.Contains(held, Anything) || slices.Contains(held, wanted)
}

// Who answers the question the apiserver will not: every subject that may do
// this, and where.
func (i Index) Who(ask Ask) []Holder {
	out := []Holder{}
	for _, holder := range i.Holders {
		kept := []Grant{}
		for _, grant := range holder.Grants {
			if grant.Reaches(ask) {
				kept = append(kept, grant)
			}
		}
		if len(kept) == 0 {
			continue
		}
		holder.Grants = kept
		out = append(out, holder)
	}
	return out
}

// The powers worth naming on sight. Each is a question a person actually asks
// about a subject, in the order they would ask it.
var powers = []struct {
	label string
	ask   Ask
}{
	{"cluster-admin", Ask{Verb: Anything, Group: Anything, Resource: Anything}},
	{"reads secrets", Ask{Verb: "get", Resource: "secrets"}},
	{"lists secrets", Ask{Verb: "list", Resource: "secrets"}},
	{"execs into pods", Ask{Verb: "create", Resource: "pods/exec"}},
	{"creates pods", Ask{Verb: "create", Resource: "pods"}},
	{"impersonates", Ask{Verb: "impersonate", Resource: "users"}},
	{"writes RBAC", Ask{Verb: "create", Group: Group, Resource: "clusterrolebindings"}},
	{"escalates", Ask{Verb: "escalate", Group: Group, Resource: "clusterroles"}},
	{"reads every kind", Ask{Verb: "get", Group: Anything, Resource: Anything}},
}

// powersOf names what a subject can do that a reader would want to know without
// opening it. cluster-admin says everything, so nothing else is worth listing
// beside it.
func powersOf(holder Holder) []string {
	out := []string{}
	for _, power := range powers {
		if !reaches(holder, power.ask) {
			continue
		}
		if power.label == "cluster-admin" {
			return []string{"cluster-admin"}
		}
		out = append(out, power.label)
	}
	return out
}

func reaches(holder Holder, ask Ask) bool {
	for _, grant := range holder.Grants {
		if grant.Reaches(ask) {
			return true
		}
	}
	return false
}

// Namespaces is where a subject's grants apply, so a reader can tell a
// cluster-wide grant from one confined to a corner.
func (h Holder) Namespaces() []string {
	held := map[string]struct{}{}
	for _, grant := range h.Grants {
		if grant.Everywhere() {
			return nil
		}
		held[grant.Namespace] = struct{}{}
	}
	return sortedKeys(held)
}
