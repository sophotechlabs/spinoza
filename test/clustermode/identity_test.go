//go:build clustermode

package clustermode

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestSigningInThroughKeycloak(t *testing.T) {
	deploy(t, oidcValues())

	t.Run("nobody signed in is turned away from the api", func(t *testing.T) {
		status, message := read(t, anonymous(t), "/api/overview")
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
		}
		if !strings.Contains(message, "sign in") {
			t.Fatalf("body = %q, want it to say to sign in", message)
		}
	})

	t.Run("the sign-in page loads without a session", func(t *testing.T) {
		status, page := read(t, anonymous(t), "/")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
		if !strings.Contains(page, "<div id=\"root\">") {
			t.Fatalf("the app did not come back: %s", truncate(page))
		}
	})

	t.Run("the session cookie is locked down over https", func(t *testing.T) {
		cookie := sessionCookieFrom(t, "alice")
		if !cookie.Secure {
			t.Fatal("the session cookie would travel over plain http")
		}
		if !cookie.HttpOnly {
			t.Fatal("the session cookie is readable from javascript")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("samesite = %v, want lax", cookie.SameSite)
		}
	})

	t.Run("each group gets the role it was given", func(t *testing.T) {
		cases := map[string]string{"alice": "admin", "bob": "editor", "carol": "viewer"}
		for user, want := range cases {
			t.Run(user, func(t *testing.T) {
				got := whoami(t, signIn(t, user))
				if got.User != user {
					t.Fatalf("user = %q, want %q", got.User, user)
				}
				if got.Role != want {
					t.Fatalf("role = %q, want %q", got.Role, want)
				}
			})
		}
	})

	t.Run("a scoped account reads only its own namespaces", func(t *testing.T) {
		carol := signIn(t, "carol")
		session := whoami(t, carol)
		if session.Scope.Everywhere {
			t.Fatal("an account bound in one namespace was given the whole cluster")
		}
		if strings.Join(session.Scope.Namespaces, ",") != "payments" {
			t.Fatalf("scope = %v, want payments alone", session.Scope.Namespaces)
		}
		if len(session.Scope.Undecided) != 0 {
			t.Fatalf("undecided = %v, want none from a cluster that answered every check", session.Scope.Undecided)
		}
		_, names := read(t, carol, "/api/namespaces")
		if !strings.Contains(names, "payments") || strings.Contains(names, "storefront") {
			t.Fatalf("namespaces = %s, want payments alone", names)
		}
		_, hits := read(t, carol, "/api/search?q=other")
		if strings.Contains(hits, "\"name\":\"other\"") {
			t.Fatalf("search reached another namespace: %s", truncate(hits))
		}
		_, mine := read(t, carol, "/api/search?q=web")
		if !strings.Contains(mine, "\"namespace\":\"payments\"") {
			t.Fatalf("search found nothing in its own namespace: %s", truncate(mine))
		}
	})

	t.Run("a view that reads the whole cluster refuses a scoped account", func(t *testing.T) {
		status, message := read(t, signIn(t, "carol"), "/api/overview")
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
		}
		if !strings.Contains(message, "reads the whole cluster") {
			t.Fatalf("body = %q, want it to say why", message)
		}
	})

	t.Run("a cluster-wide reader gets the whole cluster", func(t *testing.T) {
		status, overview := read(t, signIn(t, "bob"), "/api/overview")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
		if !strings.Contains(overview, "\"nodes\"") {
			t.Fatalf("overview = %s", truncate(overview))
		}
	})

	t.Run("the row feed carries only the namespaces the account may read", func(t *testing.T) {
		carol := subscribeUntilReady(t, signIn(t, "carol"), api.ClientMsg{Version: "v1", Resource: "pods"})
		if carol.Type != "snapshot" {
			t.Fatalf("feed said %q: %s", carol.Type, carol.Message)
		}
		if got := strings.Join(namespacesIn(carol.Rows), ","); got != "payments" {
			t.Fatalf("rows came from %q, want payments alone", got)
		}
		bob := subscribeUntilReady(t, signIn(t, "bob"), api.ClientMsg{Version: "v1", Resource: "pods"})
		if len(bob.Rows) <= len(carol.Rows) {
			t.Fatalf("a cluster-wide reader saw %d rows, a scoped one %d", len(bob.Rows), len(carol.Rows))
		}
	})

	t.Run("a kind in no namespace is refused to a scoped account", func(t *testing.T) {
		reply := subscribe(t, signIn(t, "carol"), api.ClientMsg{Version: "v1", Resource: "nodes"})
		if reply.Type != "error" {
			t.Fatalf("feed said %q, want an error rather than an empty table", reply.Type)
		}
		if !strings.Contains(reply.Message, "belongs to no namespace") {
			t.Fatalf("message = %q, want it to say why", reply.Message)
		}
	})

	t.Run("signing out ends the provider's session too", func(t *testing.T) {
		alice := signIn(t, "alice")
		resp := request(t, alice, http.MethodPost, "/auth/logout")
		defer func() { _ = resp.Body.Close() }()
		if signedIn(t, alice) {
			t.Fatal("the session survived signing out")
		}
		landed := resp.Request.URL.String()
		if !strings.Contains(landed, "keycloak.localtest.me") {
			t.Fatalf("signing out landed on %q, want the provider's logout", landed)
		}
	})
}
