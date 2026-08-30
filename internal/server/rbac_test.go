package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/rbac"
)

type indexed struct {
	notStubbed

	index rbac.Index
}

func (i *indexed) RBACIndex(context.Context) rbac.Index {
	return i.index
}

func holding(powers []string, grants ...rbac.Grant) rbac.Holder {
	return rbac.Holder{
		Subject: rbac.Subject{Kind: rbac.KindUser, Name: "ana"},
		Powers:  powers,
		Grants:  grants,
	}
}

func granted(role, namespace string, rules ...rbac.Rule) rbac.Grant {
	return rbac.Grant{
		Binding:     role + "-binding",
		BindingKind: "ClusterRoleBinding",
		Role:        role,
		RoleKind:    rbac.ClusterRoleKind,
		Namespace:   namespace,
		Rules:       rules,
	}
}

func canDo(verb, resource string) rbac.Rule {
	return rbac.Rule{Verbs: []string{verb}, Groups: []string{""}, Resources: []string{resource}}
}

func rbacServer(t *testing.T, index rbac.Index) *httptest.Server {
	t.Helper()
	return stubbedServer(t, &indexed{index: index})
}

func TestTheIndexNamesEverySubjectAndWhatItHolds(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{holding([]string{"reads secrets"}, granted("reader", "", canDo("get", "secrets")))},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	if len(got.Subjects) != 1 {
		t.Fatalf("subjects = %+v", got.Subjects)
	}
	one := got.Subjects[0]
	if one.Label != "ana" || len(one.Powers) != 1 {
		t.Fatalf("the subject came back as %+v", one)
	}
	if len(one.Grants) != 1 || one.Grants[0].Role != "reader" {
		t.Fatalf("grants = %+v", one.Grants)
	}
}

func TestASubjectSaysWhereItsGrantsApply(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{holding(nil, granted("reader", "web", canDo("get", "pods")))},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	if len(got.Subjects[0].Namespaces) != 1 || got.Subjects[0].Namespaces[0] != "web" {
		t.Fatalf("namespaces = %+v", got.Subjects[0].Namespaces)
	}
}

func TestTheIndexCarriesTheRulesAsWritten(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{holding(nil, granted("wide", "",
			rbac.Rule{Verbs: []string{"*"}, Groups: []string{"*"}, Resources: []string{"*"}}))},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	rule := got.Subjects[0].Grants[0].Rules[0]
	if len(rule.Verbs) != 1 || rule.Verbs[0] != "*" {
		t.Fatalf("the rule came back as %+v", rule)
	}
}

func TestTheIndexSaysWhatItCouldNotSee(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Absent: []string{"RoleBinding web/read wants Role gone"},
		Error:  "not discovered: clusterroles",
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	if len(got.Absent) != 1 || got.Error != "not discovered: clusterroles" {
		t.Fatalf("index = %+v", got)
	}
}

func TestAnEmptyIndexComesBackAsAList(t *testing.T) {
	ts := rbacServer(t, rbac.Index{})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	if got.Subjects == nil {
		t.Fatal("subjects came back as null rather than an empty list")
	}
}

func TestTheIndexStopsAtItsCapAndSaysSo(t *testing.T) {
	held := rbac.Index{}
	for range rbacSubjectCap + 20 {
		held.Holders = append(held.Holders, holding(nil, granted("reader", "web", canDo("get", "pods"))))
	}
	ts := rbacServer(t, held)

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac", &got)

	if len(got.Subjects) != rbacSubjectCap {
		t.Fatalf("subjects = %d", len(got.Subjects))
	}
	if got.Dropped != 20 {
		t.Fatalf("dropped = %d", got.Dropped)
	}
}

func TestWhoNamesTheSubjectsThatMay(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{
			holding(nil, granted("exec", "", canDo("create", "pods/exec"))),
			{
				Subject: rbac.Subject{Kind: rbac.KindUser, Name: "bo"},
				Grants:  []rbac.Grant{granted("look", "", canDo("get", "pods"))},
			},
		},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac/who?verb=create&resource=pods/exec", &got)

	if len(got.Subjects) != 1 || got.Subjects[0].Label != "ana" {
		t.Fatalf("who = %+v", got.Subjects)
	}
}

func TestWhoNeedsAVerbAndAResource(t *testing.T) {
	ts := rbacServer(t, rbac.Index{})

	for _, query := range []string{"", "?verb=get", "?resource=pods"} {
		resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/rbac/who"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %q = %d", query, resp.StatusCode)
		}
	}
}

func TestWhoNarrowsToANamespace(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{holding(nil, granted("reader", "web", canDo("get", "secrets")))},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac/who?verb=get&resource=secrets&namespace=other", &got)

	if len(got.Subjects) != 0 {
		t.Fatalf("who = %+v, want nobody outside the namespace", got.Subjects)
	}
}

func TestWhoCarriesTheApiGroup(t *testing.T) {
	ts := rbacServer(t, rbac.Index{
		Holders: []rbac.Holder{holding(nil, granted("deployer", "",
			rbac.Rule{Verbs: []string{"create"}, Groups: []string{"apps"}, Resources: []string{"deployments"}}))},
	})

	var got api.RBACIndex
	readFleet(t, ts, "/api/rbac/who?verb=create&group=apps&resource=deployments", &got)

	if len(got.Subjects) != 1 {
		t.Fatalf("who = %+v", got.Subjects)
	}
}
