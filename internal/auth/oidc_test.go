package auth

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type startedLogin struct {
	cookies []*http.Cookie
	state   string
	nonce   string
	away    *url.URL
}

func startLogin(t *testing.T, held *Authenticator, next string) startedLogin {
	t.Helper()
	recorded := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/login?"+url.Values{returnParam: []string{next}}.Encode(), http.NoBody)

	held.Login(recorded, req)

	if recorded.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d with the location of the provider", recorded.Code, http.StatusFound)
	}
	away, err := url.Parse(recorded.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the login redirect was not a url: %v", err)
	}
	return startedLogin{
		cookies: recorded.Result().Cookies(),
		state:   away.Query().Get("state"),
		nonce:   away.Query().Get("nonce"),
		away:    away,
	}
}

func callback(t *testing.T, held *Authenticator, login startedLogin, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	recorded := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+query.Encode(), http.NoBody)
	for _, cookie := range login.cookies {
		req.AddCookie(cookie)
	}
	held.Callback(recorded, req)
	return recorded
}

func signedIn(t *testing.T, held *Authenticator, idp *fakeIDP) *http.Request {
	t.Helper()
	login := startLogin(t, held, "/checks")
	idp.claims = idp.standardClaims(login.nonce)
	recorded := callback(t, held, login, url.Values{"state": {login.state}, "code": {"abc"}})
	if recorded.Header().Get("Location") != "/checks" {
		t.Fatalf("landed on %q, want the page the login started from", recorded.Header().Get("Location"))
	}
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
	for _, cookie := range recorded.Result().Cookies() {
		req.AddCookie(cookie)
	}
	return req
}

func errorFrom(t *testing.T, held *Authenticator, recorded *httptest.ResponseRecorder) string {
	t.Helper()
	if got := recorded.Header().Get("Location"); got != "/" {
		t.Fatalf("landed on %q, want the sign-in page with nothing in the url", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", http.NoBody)
	for _, cookie := range recorded.Result().Cookies() {
		req.AddCookie(cookie)
	}
	return held.WhyNot(httptest.NewRecorder(), req)
}

func TestALoginSendsTheBrowserToTheProviderWithEverythingTheSpecWants(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)

	login := startLogin(t, held, "/checks")

	if login.away.Path != "/authorize" {
		t.Fatalf("path = %q, want the provider's authorize endpoint", login.away.Path)
	}
	query := login.away.Query()
	if query.Get("client_id") != "spinoza" {
		t.Fatalf("client_id = %q, want spinoza", query.Get("client_id"))
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("pkce = %q/%q, want an S256 challenge", query.Get("code_challenge_method"), query.Get("code_challenge"))
	}
	if login.state == "" || login.nonce == "" {
		t.Fatal("the login carried no state or no nonce")
	}
	if query.Get("scope") != "openid profile email groups" {
		t.Fatalf("scope = %q, want the four spinoza asks for", query.Get("scope"))
	}
	if query.Get("redirect_uri") != "https://spinoza.example.com/auth/callback" {
		t.Fatalf("redirect_uri = %q, want the configured one", query.Get("redirect_uri"))
	}
	if len(login.cookies) != 1 || login.cookies[0].Name != flowCookie {
		t.Fatalf("cookies = %v, want only the login state", login.cookies)
	}
}

func TestALoginAfterSigningOutAsksTheProviderToShowItsForm(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)
	recorded := httptest.NewRecorder()

	held.Login(recorded, httptest.NewRequest(http.MethodGet, "/auth/login?prompt=login", http.NoBody))

	away, _ := url.Parse(recorded.Header().Get("Location"))
	if away.Query().Get("prompt") != "login" {
		t.Fatal("the provider was not asked to show its login form, so it would sign the same person straight back in")
	}
}

func TestAFinishedLoginBecomesASessionWithTheProvidersClaims(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.EditorGroups = []string{"platform"}
	})

	who, ok := held.Identify(httptest.NewRecorder(), signedIn(t, held, idp))
	if !ok {
		t.Fatal("the session spinoza issued did not read back")
	}
	if who.User != "alice" {
		t.Fatalf("user = %q, want the preferred_username claim", who.User)
	}
	if strings.Join(who.Groups, ",") != "platform" {
		t.Fatalf("groups = %v, want the groups claim", who.Groups)
	}
	if who.Role != RoleEditor {
		t.Fatalf("role = %q, want %q from the group list", who.Role, RoleEditor)
	}
	if who.Session != "session-7" {
		t.Fatalf("session = %q, want the sid claim", who.Session)
	}
	if idp.received("code_verifier") == "" {
		t.Fatal("the token exchange sent no pkce verifier")
	}
}

func TestPrefixesGoOnToMatchWhatTheApiserverBinds(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.UsernamePrefix = "oidc:"
		cfg.OIDC.GroupsPrefix = "oidc:"
	})

	who, _ := held.Identify(httptest.NewRecorder(), signedIn(t, held, idp))

	if who.User != "oidc:alice" {
		t.Fatalf("user = %q, want the prefix in front", who.User)
	}
	if who.Groups[0] != "oidc:platform" {
		t.Fatalf("group = %q, want the prefix in front", who.Groups[0])
	}
}

func TestALoginThatComesBackWrongEndsOnTheSignInPage(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)

	cases := []struct {
		name  string
		build func(t *testing.T) *httptest.ResponseRecorder
		want  string
	}{
		{
			name: "with a state spinoza never sent",
			build: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()
				login := startLogin(t, held, "/")
				return callback(t, held, login, url.Values{"state": {"forged"}, "code": {"abc"}})
			},
			want: "did not come back with the state",
		},
		{
			name: "with no login in progress at all",
			build: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()
				return callback(t, held, startedLogin{}, url.Values{"state": {"forged"}, "code": {"abc"}})
			},
			want: "did not come back with the state",
		},
		{
			name: "with the provider refusing",
			build: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()
				login := startLogin(t, held, "/")
				return callback(t, held, login, url.Values{
					"state":             {login.state},
					"error":             {"access_denied"},
					"error_description": {"the user said no"},
				})
			},
			want: "the user said no",
		},
		{
			name: "with the provider refusing and saying nothing useful",
			build: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()
				login := startLogin(t, held, "/")
				return callback(t, held, login, url.Values{"state": {login.state}, "error": {"access_denied"}})
			},
			want: "access_denied",
		},
		{
			name: "with no code and no reason",
			build: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()
				login := startLogin(t, held, "/")
				return callback(t, held, login, url.Values{"state": {login.state}})
			},
			want: "without an authorization code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errorFrom(t, held, tc.build(t))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("message = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

func TestATokenSpinozaCannotTrustNeverBecomesASession(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(idp *fakeIDP, nonce string)
		want    string
	}{
		{
			name:    "the exchange failed",
			arrange: func(idp *fakeIDP, _ string) { idp.tokenCode = http.StatusBadRequest },
			want:    "refused the login",
		},
		{
			name:    "there was no id token",
			arrange: func(idp *fakeIDP, _ string) { idp.noIDToken = true },
			want:    "returned no id token",
		},
		{
			name: "the nonce was somebody else's",
			arrange: func(idp *fakeIDP, _ string) {
				idp.claims = idp.standardClaims("another-login")
			},
			want: "nonce spinoza did not send",
		},
		{
			name: "it was signed for another client",
			arrange: func(idp *fakeIDP, nonce string) {
				claims := idp.standardClaims(nonce)
				claims["aud"] = "grafana"
				idp.claims = claims
			},
			want: "did not verify",
		},
		{
			name: "it carried no username spinoza reads",
			arrange: func(idp *fakeIDP, nonce string) {
				claims := idp.standardClaims(nonce)
				delete(claims, "preferred_username")
				delete(claims, "email")
				delete(claims, "sub")
				idp.claims = claims
			},
			want: "none of the claims spinoza reads a username from",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp := newIDP(t)
			held := authFor(t, idp, nil)
			login := startLogin(t, held, "/")
			tc.arrange(idp, login.nonce)

			got := errorFrom(t, held, callback(t, held, login, url.Values{"state": {login.state}, "code": {"abc"}}))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("message = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

func TestAValidLoginTooLargeForASessionCookieIsRefused(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)
	login := startLogin(t, held, "/")
	claims := idp.standardClaims(login.nonce)
	claims["groups"] = []string{strings.Repeat("g", maxCookieBytes)}
	idp.claims = claims

	recorded := callback(t, held, login, url.Values{"state": {login.state}, "code": {"abc"}})

	if recorded.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusInternalServerError)
	}
	for _, cookie := range recorded.Result().Cookies() {
		if cookie.Name == SessionCookie && cookie.Value != "" {
			t.Fatal("an identity too large to retain still became a session")
		}
	}
	if !strings.Contains(recorded.Body.String(), "too large") {
		t.Fatalf("body = %q, want the cookie limit named", recorded.Body.String())
	}
}

func TestSigningOutEndsTheProvidersSessionToo(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.PostLogoutURL = "https://spinoza.example.com/"
	})
	req := signedIn(t, held, idp)
	recorded := httptest.NewRecorder()

	held.Logout(recorded, req)

	away, err := url.Parse(recorded.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the logout redirect was not a url: %v", err)
	}
	if away.Path != "/logout" {
		t.Fatalf("path = %q, want the provider's end session endpoint", away.Path)
	}
	if away.Query().Get("client_id") != "spinoza" {
		t.Fatal("the provider was not told which client is signing out")
	}
	if away.Query().Get("post_logout_redirect_uri") != "https://spinoza.example.com/" {
		t.Fatal("the provider was not told where to send the browser back")
	}
	if _, still := held.Identify(httptest.NewRecorder(), req); still {
		t.Fatal("the session was still good after signing out")
	}
}

func TestSigningOutOfAProviderWithNoLogoutEndpointForcesTheFormNextTime(t *testing.T) {
	idp := newIDP(t)
	idp.noLogout = true
	held := authFor(t, idp, nil)
	recorded := httptest.NewRecorder()

	held.Logout(recorded, httptest.NewRequest(http.MethodGet, "/auth/logout", http.NoBody))

	if recorded.Header().Get("Location") != "/?prompt=login" {
		t.Fatalf("location = %q, want the sign-in page asking for the form", recorded.Header().Get("Location"))
	}
}

func TestAProviderCanEndASessionOverTheBackChannel(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.BackchannelLogout = true
	})
	req := signedIn(t, held, idp)
	token := signJWT(map[string]any{
		"iss":    idp.issuer,
		"aud":    "spinoza",
		"iat":    idp.standardClaims("")["iat"],
		"exp":    idp.standardClaims("")["exp"],
		"sid":    "session-7",
		"events": map[string]any{backchannelEvent: map[string]any{}},
	})
	recorded := httptest.NewRecorder()

	held.BackchannelLogout(recorded, logoutRequest(token))

	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusOK)
	}
	if _, still := held.Identify(httptest.NewRecorder(), req); still {
		t.Fatal("the session the provider ended still worked")
	}
}

func logoutRequest(token string) *http.Request {
	body := strings.NewReader(url.Values{"logout_token": {token}}.Encode())
	req := httptest.NewRequest(http.MethodPost, "/auth/backchannel-logout", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestABackChannelLogoutIsRefusedWhenItCannotBeTrusted(t *testing.T) {
	idp := newIDP(t)
	base := idp.standardClaims("")

	cases := []struct {
		name  string
		on    bool
		token string
		want  int
	}{
		{name: "the feature is off", on: false, token: "anything", want: http.StatusNotFound},
		{name: "no token at all", on: true, token: "", want: http.StatusBadRequest},
		{name: "a token that is not a token", on: true, token: "not.a.jwt", want: http.StatusBadRequest},
		{
			name: "a token that is not a logout",
			on:   true,
			token: signJWT(map[string]any{
				"iss": idp.issuer, "aud": "spinoza", "iat": base["iat"], "exp": base["exp"], "sid": "session-7",
			}),
			want: http.StatusBadRequest,
		},
		{
			name: "a logout naming nobody",
			on:   true,
			token: signJWT(map[string]any{
				"iss": idp.issuer, "aud": "spinoza", "iat": base["iat"], "exp": base["exp"],
				"events": map[string]any{backchannelEvent: map[string]any{}},
			}),
			want: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			held := authFor(t, idp, func(cfg *Config) {
				cfg.OIDC.BackchannelLogout = tc.on
			})
			recorded := httptest.NewRecorder()

			held.BackchannelLogout(recorded, logoutRequest(tc.token))

			if recorded.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorded.Code, tc.want)
			}
		})
	}
}

func TestAnOversizedBackChannelLogoutIsRefusedBeforeItIsParsed(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.BackchannelLogout = true
	})
	body := strings.NewReader("logout_token=" + strings.Repeat("x", maxBackchannelLogoutBytes))
	req := httptest.NewRequest(http.MethodPost, "/auth/backchannel-logout", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorded := httptest.NewRecorder()

	held.BackchannelLogout(recorded, req)

	if recorded.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestALogoutNamingOnlyASubjectIsAcceptedAndSaidOutLoud(t *testing.T) {
	idp := newIDP(t)
	idp.noSIDs = true
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.BackchannelLogout = true
	})
	base := idp.standardClaims("")
	token := signJWT(map[string]any{
		"iss": idp.issuer, "aud": "spinoza", "iat": base["iat"], "exp": base["exp"], "sub": "1a2b",
		"events": map[string]any{backchannelEvent: map[string]any{}},
	})
	recorded := httptest.NewRecorder()

	held.BackchannelLogout(recorded, logoutRequest(token))

	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; the provider retries anything else", recorded.Code, http.StatusOK)
	}
}

func TestAnUnreachableProviderStopsSpinozaStarting(t *testing.T) {
	cfg := Config{
		Mode:      ModeOIDC,
		PublicURL: "https://spinoza.example.com",
		OIDC: OIDCConfig{
			IssuerURL:   "http://127.0.0.1:1/realms/main",
			ClientID:    "spinoza",
			RedirectURL: "https://spinoza.example.com/auth/callback",
		},
	}

	_, err := New(t.Context(), cfg)
	if err == nil {
		t.Fatal("spinoza started against a provider it could not reach")
	}
	if !strings.Contains(err.Error(), "oidc discovery") {
		t.Fatalf("error = %q, want it to name discovery", err.Error())
	}
}

func TestTheBrowserGetsThePublicIssuerWhileThePodUsesTheInternalOne(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.OIDC.IssuerURL = "https://keycloak.example.com/realms/main"
		cfg.OIDC.InternalIssuerURL = idp.issuer
	})

	login := startLogin(t, held, "/")

	if login.away.Host != "keycloak.example.com" {
		t.Fatalf("the browser was sent to %q, which it cannot reach", login.away.Host)
	}
	idp.claims = map[string]any{
		"iss": "https://keycloak.example.com/realms/main", "aud": "spinoza",
		"exp": idp.standardClaims("")["exp"], "iat": idp.standardClaims("")["iat"],
		"nonce": login.nonce, "sub": "1a2b", "preferred_username": "alice",
	}

	recorded := callback(t, held, login, url.Values{"state": {login.state}, "code": {"abc"}})
	if errorFrom(t, held, recorded) != "" {
		t.Fatalf("the login failed: %s", errorFrom(t, held, recorded))
	}
}

func TestAnEndpointOnAHostSpinozaWasNotToldAboutIsLeftAlone(t *testing.T) {
	if got := swapBase("https://elsewhere/authorize", "https://inner", "https://outer"); got != "https://elsewhere/authorize" {
		t.Fatalf("endpoint = %q, want it untouched", got)
	}
	if got := swapBase("", "https://inner", "https://outer"); got != "" {
		t.Fatalf("endpoint = %q, want it empty", got)
	}
}

func TestACertificateAuthorityHasToBeReadable(t *testing.T) {
	_, err := httpClientFor(OIDCConfig{CACertFile: "/no/such/file"})
	if err == nil {
		t.Fatal("a missing ca certificate was accepted")
	}
	if !strings.Contains(err.Error(), "oidc ca certificate") {
		t.Fatalf("error = %q, want it to name the certificate", err.Error())
	}

	path := writeFile(t, "not a certificate")
	_, badErr := httpClientFor(OIDCConfig{CACertFile: path})
	if badErr == nil {
		t.Fatal("a file with no certificate in it was accepted")
	}
}

func TestACustomCertificateAuthorityTrustsItsProvider(t *testing.T) {
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(provider.Close)

	certificate := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: provider.Certificate().Raw,
	})
	path := writeFile(t, string(certificate))
	client, err := httpClientFor(OIDCConfig{CACertFile: path})
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}
	response, err := client.Get(provider.URL)
	if err != nil {
		t.Fatalf("calling the provider: %v", err)
	}
	t.Cleanup(func() {
		response.Body.Close()
	})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestTheDefaultTransportIsUsedWhenTlsNeedsNothingSpecial(t *testing.T) {
	client, err := httpClientFor(OIDCConfig{})
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}
	if client.Transport != http.DefaultTransport {
		t.Fatal("a plain provider got a transport of its own")
	}
}

func TestSkippingVerificationIsPossibleForALab(t *testing.T) {
	client, err := httpClientFor(OIDCConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}
	if client.Transport == http.DefaultTransport {
		t.Fatal("skipping verification left the default transport in place")
	}
}

func TestAProviderWithNoLogoutEndpointOffersNoLogoutUrl(t *testing.T) {
	held := &provider{}

	if got := held.logoutURL(); got != "" {
		t.Fatalf("url = %q, want none", got)
	}
	broken := &provider{endSession: "://nope"}
	if got := broken.logoutURL(); got != "" {
		t.Fatalf("url = %q, want none from an endpoint that is not a url", got)
	}
}

func TestAScopeTheProviderDoesNotDefineIsNotAskedFor(t *testing.T) {
	cases := []struct {
		name      string
		wanted    []string
		supported []string
		want      string
	}{
		{
			name:      "stock keycloak, which has no groups scope",
			wanted:    DefaultScopes,
			supported: []string{"openid", "profile", "email", "roles", "address"},
			want:      "openid profile email",
		},
		{
			name:      "dex, which needs it",
			wanted:    DefaultScopes,
			supported: []string{"openid", "profile", "email", "groups"},
			want:      "openid profile email groups",
		},
		{
			name:      "a provider that lists none",
			wanted:    DefaultScopes,
			supported: nil,
			want:      "openid profile email groups",
		},
		{
			name:      "a provider that forgot to list openid",
			wanted:    []string{"openid", "email"},
			supported: []string{"email"},
			want:      "openid email",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(askableScopes(tc.wanted, tc.supported), " "); got != tc.want {
				t.Fatalf("scopes = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTheLoginAsksOnlyForScopesTheProviderAdvertises(t *testing.T) {
	idp := newIDP(t)
	idp.scopes = []string{"openid", "profile", "email"}
	held := authFor(t, idp, nil)

	login := startLogin(t, held, "/")

	if got := login.away.Query().Get("scope"); got != "openid profile email" {
		t.Fatalf("scope = %q, want the groups scope left out", got)
	}
}

func TestWorkingAllDayNeverBouncesYouBackToTheProvider(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.SessionTTL = time.Hour
	})
	req := signedIn(t, held, idp)
	held.sessions.now = func() time.Time { return time.Now().Add(50 * time.Minute) }
	recorded := httptest.NewRecorder()

	who, ok := held.Identify(recorded, req)
	if !ok {
		t.Fatal("a session still inside its life was turned away")
	}
	if who.User != "alice" {
		t.Fatalf("user = %q, want alice", who.User)
	}
	renewed := recorded.Result().Cookies()
	if len(renewed) != 1 || renewed[0].Name != SessionCookie {
		t.Fatalf("cookies = %v, want the session renewed", renewed)
	}
}

func TestASessionWellInsideItsLifeIsLeftAlone(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)
	req := signedIn(t, held, idp)
	recorded := httptest.NewRecorder()

	held.Identify(recorded, req)

	if len(recorded.Result().Cookies()) != 0 {
		t.Fatal("a fresh session was written back on every request")
	}
}

func TestABrowserLeftOpenAllWeekGoesBackToTheProvider(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.SessionTTL = time.Hour
		cfg.SessionMaxAge = 2 * time.Hour
	})
	req := signedIn(t, held, idp)

	held.sessions.now = func() time.Time { return time.Now().Add(50 * time.Minute) }
	renewed := httptest.NewRecorder()
	if _, ok := held.Identify(renewed, req); !ok {
		t.Fatal("a session inside the cap was turned away")
	}
	if len(renewed.Result().Cookies()) != 1 {
		t.Fatal("a session inside the cap was not renewed")
	}

	held.sessions.now = func() time.Time { return time.Now().Add(3 * time.Hour) }
	past := httptest.NewRecorder()
	if _, ok := held.Identify(past, req); ok {
		t.Fatal("a session past the cap still worked, so groups lost at the provider would too")
	}
	if len(past.Result().Cookies()) != 0 {
		t.Fatal("a session past the cap was renewed anyway")
	}
}

func TestASessionPastTheCapIsNotRenewedWhileItStillReads(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, func(cfg *Config) {
		cfg.SessionTTL = time.Hour
		cfg.SessionMaxAge = 90 * time.Minute
	})
	req := signedIn(t, held, idp)
	held.sessions.now = func() time.Time { return time.Now().Add(95 * time.Minute) }
	recorded := httptest.NewRecorder()

	if _, ok := held.Identify(recorded, req); ok {
		t.Fatal("the cookie outlived both its own life and the cap")
	}
	if len(recorded.Result().Cookies()) != 0 {
		t.Fatal("a session past the cap was renewed")
	}
}

func TestWhyALoginFailedIsNotSomethingAnyoneCanPutOnThatPage(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)

	planted := httptest.NewRequest(http.MethodGet, "/api/auth/me?authError=call+555+for+support", http.NoBody)
	if got := held.WhyNot(httptest.NewRecorder(), planted); got != "" {
		t.Fatalf("the sign-in page would have shown %q, which came from the address bar", got)
	}

	login := startLogin(t, held, "/")
	refused := callback(t, held, login, url.Values{"state": {login.state}, "error": {"access_denied"}})
	if got := errorFrom(t, held, refused); !strings.Contains(got, "access_denied") {
		t.Fatalf("the provider's own reason was lost: %q", got)
	}
}

func TestTheReasonALoginFailedIsShownOnceAndThenForgotten(t *testing.T) {
	idp := newIDP(t)
	held := authFor(t, idp, nil)
	login := startLogin(t, held, "/")
	refused := callback(t, held, login, url.Values{"state": {login.state}, "error": {"access_denied"}})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", http.NoBody)
	for _, cookie := range refused.Result().Cookies() {
		req.AddCookie(cookie)
	}
	cleared := httptest.NewRecorder()
	if held.WhyNot(cleared, req) == "" {
		t.Fatal("the reason was not kept for the sign-in page at all")
	}

	again := httptest.NewRequest(http.MethodGet, "/api/auth/me", http.NoBody)
	for _, cookie := range cleared.Result().Cookies() {
		again.AddCookie(cookie)
	}
	if got := held.WhyNot(httptest.NewRecorder(), again); got != "" {
		t.Fatalf("the reason came back a second time as %q, so a reload would keep showing it", got)
	}
}
