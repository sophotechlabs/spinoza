package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testProxySecret = []byte(strings.Repeat("p", minimumSecretBytes))

func modeless(t *testing.T, cfg Config) *Authenticator {
	t.Helper()
	if cfg.Mode == ModeProxy && len(cfg.Proxy.SharedSecret) == 0 {
		cfg.Proxy.SharedSecret = testProxySecret
	}
	built, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	return built
}

func TestBuildingAuthFailsClosedWhenASecretCannotBeGenerated(t *testing.T) {
	built, err := newAuthenticator(t.Context(), Config{Mode: ModeNone}, failedRandom{})

	if built != nil {
		t.Fatalf("authenticator = %+v, want none without a signing secret", built)
	}
	if err == nil || !strings.Contains(err.Error(), "entropy source failed") {
		t.Fatalf("error = %v, want the entropy failure", err)
	}
}

func TestWithNoAuthEverybodyWhoReachesSpinozaIsAnAdmin(t *testing.T) {
	held := modeless(t, Config{Mode: ModeNone})

	who, ok := held.Identify(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody))
	if !ok {
		t.Fatal("a request was turned away even though nothing asks people to sign in")
	}
	if who.Role != RoleAdmin {
		t.Fatalf("role = %q, want %q", who.Role, RoleAdmin)
	}
	if !who.Anonymous() {
		t.Fatal("spinoza would impersonate somebody when nobody signed in")
	}
	if held.Enabled() || held.SignsIn() {
		t.Fatal("a spinoza with no auth claimed it could sign people in")
	}
	if held.Mode() != ModeNone {
		t.Fatalf("mode = %q, want %q, which is what the browser is told", held.Mode(), ModeNone)
	}
}

func TestProxyModeReadsWhoTheProxySaysYouAre(t *testing.T) {
	held := modeless(t, Config{
		Mode:        ModeProxy,
		AdminGroups: []string{"platform-admins"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
	req.Header.Set(DefaultUserHeader, "alice@example.com")
	req.Header.Set(DefaultGroupsHeader, "platform-admins, sre")
	req.Header.Set(DefaultProxyAuthHeader, string(testProxySecret))

	who, ok := held.Identify(httptest.NewRecorder(), req)
	if !ok {
		t.Fatal("the identity the proxy set was not read")
	}
	if who.User != "alice@example.com" {
		t.Fatalf("user = %q, want the header", who.User)
	}
	if who.Role != RoleAdmin {
		t.Fatalf("role = %q, want %q from the group list", who.Role, RoleAdmin)
	}
	if strings.Join(who.Groups, ",") != "platform-admins,sre" {
		t.Fatalf("groups = %v, want both", who.Groups)
	}
}

func TestProxyModeRejectsForgedIdentityHeadersWithoutProxyAuthentication(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	for _, token := range []string{"", strings.Repeat("x", minimumSecretBytes)} {
		req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
		req.Header.Set(DefaultUserHeader, "mallory")
		req.Header.Set(DefaultGroupsHeader, "platform-admins")
		req.Header.Set(DefaultProxyAuthHeader, token)

		if _, ok := held.Identify(httptest.NewRecorder(), req); ok {
			t.Fatalf("identity headers with proxy token %q were trusted", token)
		}
	}
}

func TestProxyModeTurnsAwayARequestWithNoIdentityOnIt(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})

	if _, ok := held.Identify(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)); ok {
		t.Fatal("a request that bypassed the proxy was let in")
	}
}

func TestAProxyIdentityStopsBeingValidWhenItsHeadersChange(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.Header.Set(DefaultUserHeader, "alice")
	req.Header.Set(DefaultGroupsHeader, "platform")
	req.Header.Set(DefaultProxyAuthHeader, string(testProxySecret))
	who, ok := held.Identify(httptest.NewRecorder(), req)
	if !ok || !held.StillValid(req, who) {
		t.Fatal("the proxy identity was not initially valid")
	}

	req.Header.Set(DefaultGroupsHeader, "guests")
	if held.StillValid(req, who) {
		t.Fatal("a proxy identity stayed valid after its groups changed")
	}
}

func TestAProxyIdentityStopsBeingValidWhenProxyAuthenticationDisappears(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.Header.Set(DefaultUserHeader, "alice")
	req.Header.Set(DefaultGroupsHeader, "platform")
	req.Header.Set(DefaultProxyAuthHeader, string(testProxySecret))
	who, ok := held.Identify(httptest.NewRecorder(), req)
	if !ok {
		t.Fatal("the authenticated proxy identity was not read")
	}

	req.Header.Del(DefaultProxyAuthHeader)

	if held.StillValid(req, who) {
		t.Fatal("a proxy identity stayed valid after proxy authentication disappeared")
	}
}

func TestProxyModeRejectsAnAuthenticatedBlankUser(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
	req.Header.Set(DefaultUserHeader, " \t ")
	req.Header.Set(DefaultProxyAuthHeader, string(testProxySecret))

	if _, ok := held.Identify(httptest.NewRecorder(), req); ok {
		t.Fatal("a whitespace-only proxy user became an identity")
	}
}

func TestNoAuthModeRevalidatesOnlyItsAnonymousAdminIdentity(t *testing.T) {
	held := modeless(t, Config{Mode: ModeNone})
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)

	if !held.StillValid(req, Identity{Role: RoleAdmin}) {
		t.Fatal("the anonymous admin stopped being valid in no-auth mode")
	}
	if held.StillValid(req, Identity{User: "alice", Role: RoleAdmin}) {
		t.Fatal("a named identity was accepted in no-auth mode")
	}
}

func TestEveryIdentityFieldRemainsBoundToTheSession(t *testing.T) {
	original := Identity{
		User:    "alice",
		Groups:  []string{"platform", "sre"},
		Role:    RoleEditor,
		Session: "session-7",
	}
	if !sameIdentity(original, original) {
		t.Fatal("an unchanged identity did not match itself")
	}
	cases := map[string]Identity{
		"user":    {User: "mallory", Groups: original.Groups, Role: original.Role, Session: original.Session},
		"role":    {User: original.User, Groups: original.Groups, Role: RoleAdmin, Session: original.Session},
		"session": {User: original.User, Groups: original.Groups, Role: original.Role, Session: "session-8"},
		"groups":  {User: original.User, Groups: []string{"platform"}, Role: original.Role, Session: original.Session},
		"order":   {User: original.User, Groups: []string{"sre", "platform"}, Role: original.Role, Session: original.Session},
	}
	for name, changed := range cases {
		t.Run(name, func(t *testing.T) {
			if sameIdentity(original, changed) {
				t.Fatalf("identity still matched after %s changed", name)
			}
		})
	}
}

func TestARevokedSessionIsNotStillValid(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	held.cfg.Mode = ModeOIDC
	who := Identity{User: "alice", Groups: []string{"platform"}, Role: RoleEditor, Session: "session-7"}
	recorded := httptest.NewRecorder()
	if err := held.sessions.issue(recorded, who, held.sessions.now()); err != nil {
		t.Fatalf("issuing a session: %v", err)
	}
	response := recorded.Result()
	defer func() { _ = response.Body.Close() }()
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.AddCookie(response.Cookies()[0])
	if !held.StillValid(req, who) {
		t.Fatal("a current session was not valid")
	}

	held.revoked.revoke(who.Session)
	if held.StillValid(req, who) {
		t.Fatal("a revoked session stayed valid")
	}
}

func TestAnExpiredSessionIsNotStillValid(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	held.cfg.Mode = ModeOIDC
	now := time.Now().Truncate(time.Second)
	held.sessions.now = func() time.Time { return now }
	who := Identity{User: "alice", Role: RoleViewer, Session: "session-8"}
	recorded := httptest.NewRecorder()
	if err := held.sessions.issue(recorded, who, now); err != nil {
		t.Fatalf("issuing a session: %v", err)
	}
	response := recorded.Result()
	defer func() { _ = response.Body.Close() }()
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.AddCookie(response.Cookies()[0])
	if !held.StillValid(req, who) {
		t.Fatal("a current session was not valid")
	}

	held.sessions.now = func() time.Time { return now.Add(DefaultSessionTTL) }
	if held.StillValid(req, who) {
		t.Fatal("an expired session stayed valid")
	}
}

func TestProxyModeSignsOutThroughTheProxy(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy, Proxy: ProxyConfig{LogoutURL: "https://proxy/oauth2/sign_out"}})
	recorded := httptest.NewRecorder()

	held.Logout(recorded, httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody))

	if recorded.Header().Get("Location") != "https://proxy/oauth2/sign_out" {
		t.Fatalf("location = %q, want the proxy's sign-out", recorded.Header().Get("Location"))
	}
}

func TestSigningOutWithNothingToSignOutOfLandsBackOnTheApp(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	recorded := httptest.NewRecorder()

	held.Logout(recorded, httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody))

	if recorded.Header().Get("Location") != "/" {
		t.Fatalf("location = %q, want the app", recorded.Header().Get("Location"))
	}
}

func TestThereIsNoLoginToStartWithoutAProvider(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})

	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"login":    held.Login,
		"callback": held.Callback,
	} {
		t.Run(name, func(t *testing.T) {
			recorded := httptest.NewRecorder()
			call(recorded, httptest.NewRequest(http.MethodGet, "/auth/"+name, http.NoBody))
			if recorded.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorded.Code, http.StatusNotFound)
			}
			var body failure
			_ = json.Unmarshal(recorded.Body.Bytes(), &body)
			if !strings.Contains(body.Message, "identity provider") {
				t.Fatalf("message = %q, want it to say there is no provider", body.Message)
			}
		})
	}
}

func TestAConfigThatCannotWorkStopsSpinozaStarting(t *testing.T) {
	_, err := New(t.Context(), Config{Mode: "ldap"})
	if err == nil {
		t.Fatal("an unusable auth config was accepted")
	}
}

func TestASecretIsGeneratedWhenNobodyGaveOne(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})

	if len(held.sessions.secret) == 0 {
		t.Fatal("sessions would be signed with an empty key")
	}
}

func TestCookiesAreOnlySecureWhenThePublicUrlIsHttps(t *testing.T) {
	cases := map[string]bool{
		"https://spinoza.example.com": true,
		"http://spinoza.example.com":  false,
		"":                            false,
		"://nope":                     false,
	}
	for url, want := range cases {
		t.Run(url, func(t *testing.T) {
			if got := secureFor(Config{PublicURL: url}); got != want {
				t.Fatalf("secure = %v, want %v for %q", got, want, url)
			}
		})
	}
}

func TestALandingPageOffSpinozaIsNotFollowed(t *testing.T) {
	cases := map[string]string{
		"/checks":              "/checks",
		"":                     "/",
		"https://elsewhere/":   "/",
		"//elsewhere/":         "/",
		`/\elsewhere/`:         "/",
		"/?view=checks#docked": "/?view=checks#docked",
	}
	for given, want := range cases {
		t.Run(given, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)
			query := req.URL.Query()
			query.Set(returnParam, given)
			req.URL.RawQuery = query.Encode()

			if got := landingFrom(req); got != want {
				t.Fatalf("landing = %q, want %q", got, want)
			}
		})
	}
}
