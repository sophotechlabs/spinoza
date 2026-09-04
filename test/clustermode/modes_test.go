//go:build clustermode

package clustermode

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestThroughARealAuthenticationProxy(t *testing.T) {
	deployRealProxy(t)

	t.Run("the proxy signs a keycloak identity before spinoza sees it", func(t *testing.T) {
		alice := signIn(t, "alice")
		session := whoami(t, alice)
		if session.User != "alice" {
			t.Fatalf("user = %q, want alice", session.User)
		}
		if session.Role != "admin" {
			t.Fatalf("role = %q, want admin", session.Role)
		}
		if session.Mode != "proxy" {
			t.Fatalf("mode = %q, want proxy", session.Mode)
		}
	})

	t.Run("incoming identity headers are replaced", func(t *testing.T) {
		alice := signIn(t, "alice")
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+pathSession, http.NoBody)
		if err != nil {
			t.Fatalf("building the forged request: %v", err)
		}
		req.Header.Set("X-Forwarded-User", "mallory")
		req.Header.Set("X-Forwarded-Groups", "guests")
		req.Header.Set("X-Spinoza-Proxy-Secret", "not-the-shared-secret")
		resp, requestErr := alice.Do(req)
		if requestErr != nil {
			t.Fatalf("sending the forged request: %v", requestErr)
		}
		defer func() { _ = resp.Body.Close() }()
		var session api.Session
		if decodeErr := json.NewDecoder(resp.Body).Decode(&session); decodeErr != nil {
			t.Fatalf("decoding the session: %v", decodeErr)
		}
		if session.User != "alice" || session.Role != "admin" {
			t.Fatalf("session = %+v, want alice's identity", session)
		}
	})

	t.Run("a direct request without the proxy secret is refused", func(t *testing.T) {
		out, err := maybeKubectl(t, "-n", "payments", "exec", "deployment/web", "--",
			"wget", "-S", "-O", "/dev/null", "http://spinoza.spinoza.svc.cluster.local:8080/api/overview")
		if err == nil {
			t.Fatalf("an unsigned direct request succeeded:\n%s", out)
		}
		if !strings.Contains(out, "401 Unauthorized") {
			t.Fatalf("direct request said %q, want 401 Unauthorized", truncate(out))
		}
	})

	t.Run("websockets cross the proxy with the signed identity", func(t *testing.T) {
		reply := subscribeUntilReady(t, signIn(t, "carol"), api.ClientMsg{Version: "v1", Resource: "pods"})
		if reply.Type != "snapshot" {
			t.Fatalf("feed said %q: %s", reply.Type, reply.Message)
		}
		if got := strings.Join(namespacesIn(reply.Rows), ","); got != "payments" {
			t.Fatalf("rows came from %q, want payments alone", got)
		}
	})

	t.Run("the proxy identity is used for kubernetes writes", func(t *testing.T) {
		defer func() {
			_, _ = maybeKubectl(t, "-n", "payments", "scale", "deployment/web", "--replicas=1")
		}()
		bob := signIn(t, "bob")
		status, message := post(t, bob, scaleTo("payments", "web", 2))
		if status != http.StatusOK {
			t.Fatalf("scaling payments gave %d: %s", status, message)
		}
		status, message = post(t, bob, scaleTo("default", "other", 2))
		if status != http.StatusForbidden {
			t.Fatalf("scaling default gave %d: %s", status, message)
		}
		if !strings.Contains(messageOf(t, message), `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", message)
		}
	})

	t.Run("signing out clears the proxy session", func(t *testing.T) {
		alice := signIn(t, "alice")
		direct := *alice
		direct.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		resp := request(t, &direct, http.MethodPost, "/auth/logout")
		location := resp.Header.Get("Location")
		_ = resp.Body.Close()
		if location != base+"/oauth2/sign_out" {
			t.Fatalf("location = %q, want the proxy sign-out", location)
		}
		resp, err := direct.Get(location)
		if err != nil {
			t.Fatalf("following the proxy sign-out: %v", err)
		}
		_ = resp.Body.Close()
		resp = request(t, &direct, http.MethodGet, "/oauth2/auth")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("proxy auth status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}

func TestBehindAnAuthProxy(t *testing.T) {
	values := baseValues()
	values["auth.mode"] = "proxy"
	values["auth.proxy.sharedSecret"] = "a-cluster-mode-proxy-secret-that-is-long-enough"
	values["auth.adminGroups[0]"] = "platform-admins"
	values["auth.editorGroups[0]"] = "platform"
	values["auth.proxy.logoutURL"] = "https://spinoza.localtest.me:8443/oauth2/sign_out"
	deploy(t, values)

	t.Run("a request the proxy did not sign is turned away", func(t *testing.T) {
		status, _ := read(t, anonymous(t), "/api/overview")
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
		}
	})

	t.Run("the headers the proxy sets decide who you are", func(t *testing.T) {
		session := asProxied(t, "alice@example.com", "platform-admins")
		if session.User != "alice@example.com" {
			t.Fatalf("user = %q, want the header", session.User)
		}
		if session.Role != "admin" {
			t.Fatalf("role = %q, want admin from the group list", session.Role)
		}
		if session.Mode != "proxy" {
			t.Fatalf("mode = %q, want proxy", session.Mode)
		}
	})

	t.Run("the cluster acts as the person the proxy named", func(t *testing.T) {
		status, message := proxiedPost(t, "bob", "platform", scaleTo("default", "other", 2))
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
		}
		if !strings.Contains(messageOf(t, message), `User "bob"`) {
			t.Fatalf("the cluster refused %q, want it to name bob", message)
		}
	})

	t.Run("signing out hands the browser to the proxy", func(t *testing.T) {
		resp := proxiedRequest(t, http.MethodPost, "/auth/logout", "alice@example.com", "platform-admins", false)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
		}
		if resp.Header.Get("Location") != "https://spinoza.localtest.me:8443/oauth2/sign_out" {
			t.Fatalf("location = %q, want the proxy's sign-out", resp.Header.Get("Location"))
		}
	})
}

func TestWithNothingAskingPeopleToSignIn(t *testing.T) {
	values := baseValues()
	values["auth.mode"] = "none"
	values["auth.allowAnonymous"] = "true"
	deploy(t, values)

	t.Run("anybody who reaches it is an admin", func(t *testing.T) {
		session := whoami(t, anonymous(t))
		if !session.Authenticated || session.Role != "admin" {
			t.Fatalf("session = %+v, want an admin", session)
		}
		if session.SignIn {
			t.Fatal("a spinoza with no provider offered a login")
		}
	})

	t.Run("the cluster is read as spinoza's own account", func(t *testing.T) {
		status, overview := read(t, anonymous(t), "/api/overview")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", status, http.StatusOK, truncate(overview))
		}
	})
}

func TestWithImpersonationOff(t *testing.T) {
	values := baseValues()
	values["auth.mode"] = "proxy"
	values["auth.proxy.sharedSecret"] = "a-cluster-mode-proxy-secret-that-is-long-enough"
	values["impersonate"] = "false"
	values["rbac.write"] = "true"
	values["auth.adminGroups[0]"] = "platform"
	deploy(t, values)

	t.Run("a write nobody is bound for still goes through, as the pod", func(t *testing.T) {
		status, message := proxiedPost(t, "bob", "platform", scaleTo("default", "other", 3))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", status, http.StatusOK, message)
		}
	})

	t.Run("every account reads the whole cluster, because the pod does", func(t *testing.T) {
		session := asProxied(t, "carol", "guests")
		if !session.Scope.Everywhere {
			t.Fatalf("scope = %+v, want the pod's own reach", session.Scope)
		}
	})
}

func TestWithTheNarrowerReadRole(t *testing.T) {
	values := baseValues()
	values["auth.mode"] = "none"
	values["auth.allowAnonymous"] = "true"
	values["rbac.read"] = "workloads"
	deploy(t, values)

	t.Run("workloads still come back", func(t *testing.T) {
		reply := subscribeUntilReady(t, anonymous(t), api.ClientMsg{Version: "v1", Resource: "pods"})
		if reply.Type != "snapshot" {
			t.Fatalf("feed said %q: %s", reply.Type, reply.Message)
		}
		if len(reply.Rows) == 0 {
			t.Fatal("no pods came back")
		}
	})

	t.Run("secrets do not", func(t *testing.T) {
		out, _ := maybeKubectl(t, "auth", "can-i", "list", "secrets",
			"--as", "system:serviceaccount:spinoza:spinoza")
		if strings.TrimSpace(out) != "no" {
			t.Fatalf("the narrower role can list secrets: %q", strings.TrimSpace(out))
		}
	})
}

func TestWhenTheProviderPublishesInternalEndpoints(t *testing.T) {
	values := oidcValues()
	values["auth.oidc.internalIssuerURL"] = shimRealm
	deploy(t, values)

	t.Run("the browser is still sent somewhere it can reach", func(t *testing.T) {
		away := loginRedirect(t)
		if !strings.HasPrefix(away, "https://keycloak.localtest.me:8443/") {
			t.Fatalf("the browser was sent to %q, which it cannot reach", away)
		}
	})

	t.Run("and the login still completes", func(t *testing.T) {
		session := whoami(t, signIn(t, "alice"))
		if !session.Authenticated || session.User != "alice" {
			t.Fatalf("session = %+v, want alice signed in", session)
		}
	})
}
