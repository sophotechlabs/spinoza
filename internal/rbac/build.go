package rbac

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

type Held struct {
	Roles        []*unstructured.Unstructured
	ClusterRoles []*unstructured.Unstructured
	Bindings     []*unstructured.Unstructured
	ClusterBinds []*unstructured.Unstructured
}

func Build(held Held) Index {
	roles := byName(held.Roles, true)
	clusterRoles := byName(held.ClusterRoles, false)
	holders := map[string]*Holder{}
	absent := map[string]struct{}{}
	for _, binding := range held.ClusterBinds {
		fold(holders, absent, roles, clusterRoles, binding, "ClusterRoleBinding", "")
	}
	for _, binding := range held.Bindings {
		fold(holders, absent, roles, clusterRoles, binding, "RoleBinding", binding.GetNamespace())
	}
	return Index{Holders: ordered(holders), Absent: sortedKeys(absent)}
}

func byName(held []*unstructured.Unstructured, namespaced bool) map[string]*unstructured.Unstructured {
	out := map[string]*unstructured.Unstructured{}
	for _, one := range held {
		out[roleKey(one.GetName(), one.GetNamespace(), namespaced)] = one
	}
	return out
}

func roleKey(name, namespace string, namespaced bool) string {
	if !namespaced {
		return name
	}
	return namespace + "/" + name
}

func fold(
	holders map[string]*Holder, absent map[string]struct{},
	roles, clusterRoles map[string]*unstructured.Unstructured,
	binding *unstructured.Unstructured, bindingKind, namespace string,
) {
	ref, found := unstr.Map(binding, "roleRef")
	if !found {
		return
	}
	roleName := unstr.At(ref, "name")
	roleKind := unstr.At(ref, "kind")
	role, present := lookup(roles, clusterRoles, roleKind, roleName, namespace)
	if !present {
		absent[bindingKind+" "+where(binding)+" wants "+roleKind+" "+roleName] = struct{}{}
	}
	grant := Grant{
		Binding:     binding.GetName(),
		BindingKind: bindingKind,
		Role:        roleName,
		RoleKind:    roleKind,
		Namespace:   namespace,
		Missing:     !present,
	}
	if present {
		grant.Rules = rulesOf(role)
		grant.Aggregated = aggregated(role)
	}
	grant.RuleCount = len(grant.Rules)
	for _, subject := range subjectsOf(binding, namespace) {
		into, seen := holders[subject.Key()]
		if !seen {
			into = &Holder{Subject: subject}
			holders[subject.Key()] = into
		}
		into.Grants = append(into.Grants, grant)
	}
}

func lookup(
	roles, clusterRoles map[string]*unstructured.Unstructured,
	kind, name, namespace string,
) (*unstructured.Unstructured, bool) {
	if kind == ClusterRoleKind {
		found, ok := clusterRoles[name]
		return found, ok
	}
	found, ok := roles[roleKey(name, namespace, true)]
	return found, ok
}

func where(binding *unstructured.Unstructured) string {
	if binding.GetNamespace() == "" {
		return binding.GetName()
	}
	return binding.GetNamespace() + "/" + binding.GetName()
}

func aggregated(role *unstructured.Unstructured) bool {
	_, found := unstr.Map(role, "aggregationRule")
	return found
}

func rulesOf(role *unstructured.Unstructured) []Rule {
	out := []Rule{}
	for _, raw := range unstr.Slice(role, "rules") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Rule{
			Verbs:     stringsAt(entry, "verbs"),
			Groups:    stringsAt(entry, "apiGroups"),
			Resources: stringsAt(entry, "resources"),
			Names:     stringsAt(entry, "resourceNames"),
			URLs:      stringsAt(entry, "nonResourceURLs"),
		})
	}
	return out
}

func subjectsOf(binding *unstructured.Unstructured, namespace string) []Subject {
	out := []Subject{}
	for _, raw := range unstr.Slice(binding, "subjects") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := unstr.At(entry, "name")
		if name == "" {
			continue
		}
		one := Subject{Kind: unstr.At(entry, "kind"), Name: name, Namespace: unstr.At(entry, "namespace")}
		if one.Kind == KindServiceAccount && one.Namespace == "" {
			one.Namespace = namespace
		}
		out = append(out, one)
	}
	return out
}

func stringsAt(entry map[string]any, field string) []string {
	listed, ok := entry[field].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(listed))
	for _, item := range listed {
		text, isString := item.(string)
		if !isString {
			continue
		}
		out = append(out, text)
	}
	return out
}

func ordered(holders map[string]*Holder) []Holder {
	out := make([]Holder, 0, len(holders))
	for _, one := range holders {
		one.Powers = powersOf(*one)
		out = append(out, *one)
	}
	slices.SortFunc(out, func(left, right Holder) int {
		if len(left.Powers) != len(right.Powers) {
			return len(right.Powers) - len(left.Powers)
		}
		if len(left.Grants) != len(right.Grants) {
			return len(right.Grants) - len(left.Grants)
		}
		return strings.Compare(left.Subject.Label(), right.Subject.Label())
	})
	return out
}

func sortedKeys(held map[string]struct{}) []string {
	out := make([]string, 0, len(held))
	for one := range held {
		out = append(out, one)
	}
	slices.Sort(out)
	return out
}
