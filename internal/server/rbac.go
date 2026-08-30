package server

import (
	"errors"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/rbac"
)

const rbacSubjectCap = 500

var errNoVerb = errors.New("verb and resource are both required")

func (s *Server) handleRBAC(w http.ResponseWriter, r *http.Request) {
	held := s.managerFor(r).RBACIndex(r.Context())
	writeJSON(w, cappedIndex(api.RBACIndex{
		Subjects: subjectsOf(held.Holders),
		Absent:   held.Absent,
		Error:    held.Error,
	}))
}

// The reverse lookup: not "may I", which the apiserver answers, but who may.
func (s *Server) handleRBACWho(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ask := rbac.Ask{
		Verb:      query.Get("verb"),
		Group:     query.Get("group"),
		Resource:  query.Get("resource"),
		Namespace: query.Get("namespace"),
	}
	if ask.Verb == "" || ask.Resource == "" {
		writeError(w, http.StatusBadRequest, errNoVerb.Error())
		return
	}
	held := s.managerFor(r).RBACIndex(r.Context())
	writeJSON(w, cappedIndex(api.RBACIndex{
		Subjects: subjectsOf(held.Who(ask)),
		Absent:   held.Absent,
		Error:    held.Error,
	}))
}

func cappedIndex(index api.RBACIndex) api.RBACIndex {
	if index.Subjects == nil {
		index.Subjects = []api.RBACSubject{}
	}
	if len(index.Subjects) <= rbacSubjectCap {
		return index
	}
	index.Dropped = len(index.Subjects) - rbacSubjectCap
	index.Subjects = index.Subjects[:rbacSubjectCap]
	return index
}

func subjectsOf(held []rbac.Holder) []api.RBACSubject {
	out := make([]api.RBACSubject, 0, len(held))
	for _, one := range held {
		out = append(out, api.RBACSubject{
			Kind:       one.Subject.Kind,
			Name:       one.Subject.Name,
			Namespace:  one.Subject.Namespace,
			Label:      one.Subject.Label(),
			Powers:     one.Powers,
			Namespaces: one.Namespaces(),
			Grants:     grantsOf(one.Grants),
		})
	}
	return out
}

func grantsOf(held []rbac.Grant) []api.RBACGrant {
	out := make([]api.RBACGrant, 0, len(held))
	for _, one := range held {
		out = append(out, api.RBACGrant{
			Binding:     one.Binding,
			BindingKind: one.BindingKind,
			Role:        one.Role,
			RoleKind:    one.RoleKind,
			Namespace:   one.Namespace,
			Rules:       rulesOf(one.Rules),
			Missing:     one.Missing,
			Aggregated:  one.Aggregated,
		})
	}
	return out
}

func rulesOf(held []rbac.Rule) []api.RBACRule {
	out := make([]api.RBACRule, 0, len(held))
	for _, one := range held {
		out = append(out, api.RBACRule{
			Verbs:     one.Verbs,
			Groups:    one.Groups,
			Resources: one.Resources,
			Names:     one.Names,
			URLs:      one.URLs,
		})
	}
	return out
}
