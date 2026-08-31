package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func modeless(t *testing.T, cfg Config) *Authenticator {
	t.Helper()
	built, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	return built
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
}

func TestProxyModeReadsWhoTheProxySaysYouAre(t *testing.T) {
	held := modeless(t, Config{
		Mode:        ModeProxy,
		AdminGroups: []string{"platform-admins"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
	req.Header.Set(DefaultUserHeader, "alice@example.com")
	req.Header.Set(DefaultGroupsHeader, "platform-admins, sre")

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

func TestProxyModeTurnsAwayARequestWithNoIdentityOnIt(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})

	if _, ok := held.Identify(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)); ok {
		t.Fatal("a request that bypassed the proxy was let in")
	}
}

func TestProxyModeSignsOutThroughTheProxy(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy, Proxy: ProxyConfig{LogoutURL: "https://proxy/oauth2/sign_out"}})
	recorded := httptest.NewRecorder()

	held.Logout(recorded, httptest.NewRequest(http.MethodGet, "/auth/logout", http.NoBody))

	if recorded.Header().Get("Location") != "https://proxy/oauth2/sign_out" {
		t.Fatalf("location = %q, want the proxy's sign-out", recorded.Header().Get("Location"))
	}
}

func TestSigningOutWithNothingToSignOutOfLandsBackOnTheApp(t *testing.T) {
	held := modeless(t, Config{Mode: ModeProxy})
	recorded := httptest.NewRecorder()

	held.Logout(recorded, httptest.NewRequest(http.MethodGet, "/auth/logout", http.NoBody))

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
