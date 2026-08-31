package rbac

import (
	"slices"
	"strings"
)

type Ask struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
}

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

func (g Grant) covers(namespace string) bool {
	if g.Everywhere() {
		return true
	}
	if namespace == "" {
		return true
	}
	return g.Namespace == namespace
}

func (r Rule) Allows(ask Ask) bool {
	if len(r.Resources) == 0 {
		return false
	}
	if len(r.Names) > 0 {
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
