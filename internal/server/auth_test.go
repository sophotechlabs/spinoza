package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/settings"
)

const testProxySecret = "a-proxy-authentication-secret-that-is-long-enough"

type servedBackend struct {
	notStubbed

	scope api.Scope
}

func (s *servedBackend) Scope(context.Context) api.Scope {
	return s.scope
}

func (s *servedBackend) Overview(context.Context) api.ClusterOverview {
	return api.ClusterOverview{Version: "v1.36.0"}
}

func (s *servedBackend) Namespaces(context.Context) api.Namespaces {
	return api.Namespaces{Names: []string{"payments"}}
}

func (s *servedBackend) Ping(context.Context) error {
	return nil
}

func servedServer(t *testing.T, backend Backend, cfg auth.Config) *httptest.Server {
	t.Helper()
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), "")
	authn, err := auth.New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func proxyServer(t *testing.T, backend Backend, cfg auth.Config) *httptest.Server {
	t.Helper()
	cfg.Mode = auth.ModeProxy
	cfg.Proxy.SharedSecret = []byte(testProxySecret)
	return servedServer(t, backend, cfg)
}

func proxyFleetServer(t *testing.T, first, second Backend) *httptest.Server {
	t.Helper()
	held := &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active:   mk1,
		backends: map[string]Backend{mk1: first, mk2: second},
	}
	cfg := auth.Config{
		Mode:        auth.ModeProxy,
		DefaultRole: auth.RoleAdmin,
		Proxy: auth.ProxyConfig{
			SharedSecret: []byte(testProxySecret),
		},
	}
	srv := New(held, testAssets(), "")
	authn, err := auth.New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func asUser(t *testing.T, ts *httptest.Server, method, path, user, groups string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, ts.URL+path, http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	if user != "" {
		req.Header.Set(auth.DefaultUserHeader, user)
		req.Header.Set(auth.DefaultGroupsHeader, groups)
		req.Header.Set(auth.DefaultProxyAuthHeader, testProxySecret)
	}
	resp, doErr := ts.Client().Do(req)
	if doErr != nil {
		t.Fatalf("%s %s: %v", method, path, doErr)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func everyNamespace() *servedBackend {
	return &servedBackend{scope: api.Scope{Everywhere: true}}
}

func TestAServedSpinozaTurnsAwayAnyoneWhoHasNotSignedIn(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{})

	resp, body := asUser(t, ts, http.MethodGet, "/api/overview", "", "")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if !strings.Contains(body, "sign in") {
		t.Fatalf("body = %q, want it to say to sign in", body)
	}
}

func TestTheSignInPageAndItsAssetsAreReachableWithoutSigningIn(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{})

	for _, path := range []string{"/", "/index.html", "/assets/chunk.js", "/favicon.svg", "/healthz", pathSession} {
		t.Run(path, func(t *testing.T) {
			resp, _ := asUser(t, ts, http.MethodGet, path, "", "")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d so the sign-in page can load", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestIdentityDependentResponsesCannotBeSharedByCaches(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{})
	for _, tc := range []struct {
		path   string
		user   string
		groups string
		want   string
	}{
		{path: pathSession, user: "alice", groups: "platform", want: "no-store"},
		{path: "/api/overview", want: "no-store"},
		{path: "/assets/chunk.js", want: "no-store"},
		{path: "/assets/chunk-Ab12cd34.js", want: "public, max-age=31536000, immutable"},
	} {
		resp, _ := asUser(t, ts, http.MethodGet, tc.path, tc.user, tc.groups)
		if got := resp.Header.Get("Cache-Control"); got != tc.want {
			t.Fatalf("%s cache control = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestTheSessionEndpointSaysWhoIsSignedInAndWhatTheyMayRead(t *testing.T) {
	ts := proxyServer(t, &servedBackend{scope: api.Scope{Namespaces: []string{"payments"}}}, auth.Config{
		EditorGroups: []string{"platform"},
	})

	_, body := asUser(t, ts, http.MethodGet, pathSession, "alice@example.com", "platform")

	var found api.Session
	if err := json.Unmarshal([]byte(body), &found); err != nil {
		t.Fatalf("decoding the session: %v", err)
	}
	if !found.Authenticated || found.User != "alice@example.com" {
		t.Fatalf("session = %+v, want alice signed in", found)
	}
	if found.Role != auth.RoleEditor {
		t.Fatalf("role = %q, want %q", found.Role, auth.RoleEditor)
	}
	if !found.Cluster || found.Mode != auth.ModeProxy {
		t.Fatalf("session = %+v, want it to say spinoza is serving a cluster over a proxy", found)
	}
	if found.Scope.Everywhere || strings.Join(found.Scope.Namespaces, ",") != "payments" {
		t.Fatalf("scope = %+v, want the one namespace", found.Scope)
	}
}

func TestTheSessionEndpointSaysNobodyIsSignedInWithoutFailing(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{})

	_, body := asUser(t, ts, http.MethodGet, pathSession+"?authError=call+555+for+support", "", "")

	var found api.Session
	if err := json.Unmarshal([]byte(body), &found); err != nil {
		t.Fatalf("decoding the session: %v", err)
	}
	if found.Authenticated {
		t.Fatal("nobody signed in and the session said otherwise")
	}
	if found.Error != "" {
		t.Fatalf("error = %q, which came from the address bar rather than from a login", found.Error)
	}
}

func TestALocalSpinozaSaysItIsNotServingACluster(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	_, body := asUser(t, ts, http.MethodGet, pathSession, "", "")

	var found api.Session
	if err := json.Unmarshal([]byte(body), &found); err != nil {
		t.Fatalf("decoding the session: %v", err)
	}
	if found.Cluster || !found.Authenticated || found.Role != auth.RoleAdmin {
		t.Fatalf("session = %+v, want your own window with nothing held back", found)
	}
}

func TestAServedSpinozaWithoutAnAuthenticatorKeepsAdminAccess(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	srv.UseClusterAuth(ClusterAuth{})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, body := asUser(t, ts, http.MethodGet, "/api/overview", "", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, http.StatusOK, body)
	}
}

func TestASignedInSessionWithoutAnActiveClusterHasEverywhereScope(t *testing.T) {
	ts := proxyServer(t, nil, auth.Config{DefaultRole: auth.RoleAdmin})

	_, body := asUser(t, ts, http.MethodGet, pathSession, "alice@example.com", "")

	var found api.Session
	if err := json.Unmarshal([]byte(body), &found); err != nil {
		t.Fatalf("decoding the session: %v", err)
	}
	if !found.Authenticated {
		t.Fatalf("session = %+v, want alice signed in", found)
	}
	if !found.Scope.Everywhere {
		t.Fatalf("scope = %+v, want no active cluster to impose no namespace limit", found.Scope)
	}
}

func TestAViewerMayLookAndMayNotChangeAnything(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{DefaultRole: auth.RoleViewer})

	looked, _ := asUser(t, ts, http.MethodGet, "/api/overview", "alice", "")
	if looked.StatusCode != http.StatusOK {
		t.Fatalf("reading gave %d, want %d", looked.StatusCode, http.StatusOK)
	}

	changed, body := asUser(t, ts, http.MethodPut, "/api/object?version=v1&resource=pods&name=web", "alice", "")
	if changed.StatusCode != http.StatusForbidden {
		t.Fatalf("writing gave %d, want %d", changed.StatusCode, http.StatusForbidden)
	}
	if !strings.Contains(body, "your role here is viewer") {
		t.Fatalf("body = %q, want it to name the role", body)
	}
}

func TestOnlyAnAdministratorMayRebuildSharedDiscovery(t *testing.T) {
	backend := &stubCatalog{}
	ts := proxyServer(t, backend, auth.Config{
		DefaultRole: auth.RoleViewer,
		AdminGroups: []string{"platform-admins"},
	})
	refused, body := asUser(t, ts, http.MethodPost, "/api/resources", "alice", "")
	if refused.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want %d", refused.StatusCode, http.StatusForbidden)
	}
	if !strings.Contains(body, "this needs admin") {
		t.Fatalf("viewer body = %q", body)
	}
	if backend.refreshed != 0 {
		t.Fatalf("viewer triggered %d refreshes", backend.refreshed)
	}
	accepted, body := asUser(t, ts, http.MethodPost, "/api/resources", "ana", "platform-admins")
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("admin status = %d, want %d: %s", accepted.StatusCode, http.StatusOK, body)
	}
	if backend.refreshed != 1 {
		t.Fatalf("admin triggered %d refreshes, want 1", backend.refreshed)
	}
}

func TestAnEditorMayChangeObjectsAndMayNotOpenAShell(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{
		DefaultRole:  auth.RoleViewer,
		EditorGroups: []string{"platform"},
	})

	shell, body := asUser(t, ts, http.MethodGet, "/api/exec?namespace=prod&pod=web", "alice", "platform")
	if shell.StatusCode != http.StatusForbidden {
		t.Fatalf("exec gave %d, want %d", shell.StatusCode, http.StatusForbidden)
	}
	if !strings.Contains(body, "this needs admin") {
		t.Fatalf("body = %q, want it to name the role it needs", body)
	}
}

func TestDrainingANodeNeedsMoreThanEditing(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{
		DefaultRole:  auth.RoleViewer,
		EditorGroups: []string{"platform"},
		AdminGroups:  []string{"platform-admins"},
	})
	path := "/api/action?action=drain&version=v1&resource=nodes&name=node-1"

	refused, body := asUser(t, ts, http.MethodPost, path, "alice", "platform")
	if refused.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d for an editor", refused.StatusCode, http.StatusForbidden)
	}
	if !strings.Contains(body, "this needs admin") {
		t.Fatalf("body = %q, want it to name the role it needs", body)
	}
}

func TestWhatOnlyWorksWhenYouRunSpinozaYourselfSaysSo(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{DefaultRole: auth.RoleAdmin})

	for _, path := range []string{"/api/shell", "/api/update", "/api/kubeconfigs/picker"} {
		t.Run(path, func(t *testing.T) {
			resp, body := asUser(t, ts, http.MethodGet, path, "alice", "")
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
			}
			if !strings.Contains(body, "run it yourself") {
				t.Fatalf("body = %q, want it to say why", body)
			}
		})
	}
}

func TestAViewThatReadsTheWholeClusterIsRefusedToAScopedAccount(t *testing.T) {
	ts := proxyServer(t, &servedBackend{scope: api.Scope{Namespaces: []string{"payments"}}}, auth.Config{
		DefaultRole: auth.RoleAdmin,
	})

	resp, body := asUser(t, ts, http.MethodGet, "/api/overview", "alice", "")

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if !strings.Contains(body, "reads the whole cluster") {
		t.Fatalf("body = %q, want it to say why", body)
	}
	if !strings.Contains(body, "reads named namespaces only") {
		t.Fatalf("body = %q, want it to say the cluster answered", body)
	}
}

func TestAFleetViewChecksScopeOnEveryCluster(t *testing.T) {
	ts := proxyFleetServer(
		t,
		&servedBackend{scope: api.Scope{Everywhere: true}},
		&servedBackend{scope: api.Scope{Namespaces: []string{"payments"}}},
	)

	resp, body := asUser(t, ts, http.MethodGet, "/api/search/fleet?q=api", "alice@example.com", "")

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, http.StatusForbidden, body)
	}
	if !strings.Contains(body, "p-mk2") {
		t.Fatalf("body = %q, want the restricted cluster named", body)
	}
}

func TestAFleetViewRefusesWhenAnOpenClusterHasNoBackend(t *testing.T) {
	ts := proxyFleetServer(t, everyNamespace(), nil)

	resp, body := asUser(t, ts, http.MethodGet, "/api/search/fleet?q=api", "alice@example.com", "")

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, http.StatusServiceUnavailable, body)
	}
	if !strings.Contains(body, "p-mk2") || !strings.Contains(body, "not available") {
		t.Fatalf("body = %q, want the unavailable cluster named", body)
	}
}

func TestAFleetViewRefusesWhenAnyClusterCannotDetermineScope(t *testing.T) {
	ts := proxyFleetServer(
		t,
		everyNamespace(),
		&servedBackend{scope: api.Scope{Undecided: []string{"payments"}}},
	)

	resp, body := asUser(t, ts, http.MethodGet, "/api/search/fleet?q=api", "alice@example.com", "")

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, http.StatusServiceUnavailable, body)
	}
	if !strings.Contains(body, "p-mk2") || !strings.Contains(body, "would not say") {
		t.Fatalf("body = %q, want the undecided cluster named", body)
	}
}

func TestAFleetScopeRefusalUsesTheClustersDisplayName(t *testing.T) {
	held := &fleet{
		held: []api.OpenCluster{{ID: mk1, Context: "p-mk1", Label: "Production"}},
		backends: map[string]Backend{
			mk1: &servedBackend{scope: api.Scope{Namespaces: []string{"payments"}}},
		},
	}
	srv := New(held, testAssets(), "")
	req := httptest.NewRequest(http.MethodGet, "/api/search/fleet?q=api", http.NoBody)

	status, why := srv.wholeFleetRefusal(req)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
	}
	if !strings.Contains(why, "Production") || strings.Contains(why, "p-mk1") {
		t.Fatalf("refusal = %q, want the display name", why)
	}
}

func TestAScopedAccountCannotReadOrChangeSharedDeploymentState(t *testing.T) {
	ts := proxyServer(t, &servedBackend{scope: api.Scope{Namespaces: []string{"payments"}}}, auth.Config{
		DefaultRole: auth.RoleAdmin,
	})
	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/history"},
		{method: http.MethodDelete, path: "/api/history"},
		{method: http.MethodGet, path: "/api/checks/mutes"},
		{method: http.MethodPost, path: "/api/checks/mutes"},
		{method: http.MethodDelete, path: "/api/checks/mutes"},
		{method: http.MethodPost, path: "/api/checks/baseline"},
		{method: http.MethodDelete, path: "/api/checks/baseline"},
		{method: http.MethodGet, path: "/api/checks/baseline/file"},
		{method: http.MethodPut, path: "/api/checks/baseline/file"},
		{method: http.MethodPost, path: "/api/clusters/timeline"},
	}
	for _, test := range cases {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			resp, body := asUser(t, ts, test.method, test.path, "carol@example.com", "")

			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
			}
			if !strings.Contains(body, "reads the whole cluster") {
				t.Fatalf("body = %q, want it to explain the shared cluster scope", body)
			}
		})
	}
}

func TestAViewThatReadsTheWholeClusterIsNotRefusedForAnAnswerNobodyGave(t *testing.T) {
	ts := proxyServer(t, &servedBackend{scope: api.Scope{Undecided: []string{"payments"}}}, auth.Config{
		DefaultRole: auth.RoleAdmin,
	})

	resp, body := asUser(t, ts, http.MethodGet, "/api/overview", "alice", "")

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d when the cluster gave no answer", resp.StatusCode, http.StatusServiceUnavailable)
	}
	if strings.Contains(body, "reads named namespaces only") {
		t.Fatalf("body = %q, want it not to blame the account for a check nobody answered", body)
	}
	if !strings.Contains(body, "would not say") {
		t.Fatalf("body = %q, want it to say the cluster did not answer", body)
	}
}

func TestAPageServedFromAnotherSiteCannotCallThisOne(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	authn, err := auth.New(t.Context(), auth.Config{Mode: auth.ModeNone})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn, PublicURL: "https://spinoza.example.com"})
	handler := srv.Handler()

	cases := []struct {
		name       string
		host       string
		origin     string
		fetch      string
		navigation bool
		want       int
	}{
		{name: "a kubelet probing the pod on its own address", host: "10.244.1.7:8080", want: http.StatusOK},
		{name: "the page spinoza served", host: "spinoza.example.com", origin: "https://spinoza.example.com", want: http.StatusOK},
		{name: "the same host over plain http", host: "spinoza.example.com", origin: "http://spinoza.example.com", want: http.StatusForbidden},
		{name: "another site", host: "spinoza.example.com", origin: "https://evil.example.com", want: http.StatusForbidden},
		{name: "a call from another site with no origin", host: "spinoza.example.com", fetch: "cross-site", want: http.StatusForbidden},
		{name: "a link to spinoza from another site", host: "spinoza.example.com", fetch: "cross-site", navigation: true, want: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetch)
			}
			if tc.navigation {
				req.Header.Set("Sec-Fetch-Mode", "navigate")
				req.Header.Set("Sec-Fetch-Dest", "document")
			}
			recorded := httptest.NewRecorder()

			handler.ServeHTTP(recorded, req)

			if recorded.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorded.Code, tc.want)
			}
		})
	}
}

func TestWithNoPublicUrlSpinozaStillRefusesAnotherSitesOrigin(t *testing.T) {
	ts := proxyServer(t, everyNamespace(), auth.Config{})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/healthz", http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example.com")

	resp, doErr := ts.Client().Do(req)
	if doErr != nil {
		t.Fatalf("request: %v", doErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestALoginIsOnlyRoutedWhenSpinozaIsServingACluster(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, _ := asUser(t, ts, http.MethodGet, pathLogin, "", "")

	if resp.StatusCode == http.StatusFound {
		t.Fatal("a local spinoza offered a login flow")
	}
}

func TestAClusterServerMountsEveryAuthenticationRoute(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	authn, err := auth.New(t.Context(), auth.Config{
		Mode:  auth.ModeProxy,
		Proxy: auth.ProxyConfig{SharedSecret: []byte(testProxySecret)},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	handler := srv.Handler()

	cases := []struct {
		method   string
		path     string
		status   int
		location string
	}{
		{method: http.MethodGet, path: pathLogin, status: http.StatusNotFound},
		{method: http.MethodGet, path: pathCallback, status: http.StatusNotFound},
		{method: http.MethodPost, path: pathLogout, status: http.StatusFound, location: "/"},
		{method: http.MethodPost, path: pathBackchannel, status: http.StatusNotFound},
	}
	for _, test := range cases {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			recorded := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, http.NoBody)

			handler.ServeHTTP(recorded, request)

			if recorded.Code != test.status {
				t.Fatalf("status = %d, want %d", recorded.Code, test.status)
			}
			if recorded.Header().Get("Location") != test.location {
				t.Fatalf("location = %q, want %q", recorded.Header().Get("Location"), test.location)
			}
		})
	}
}

func TestACrossSiteNavigationCannotSignSomeoneOut(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	authn, err := auth.New(t.Context(), auth.Config{
		Mode: auth.ModeProxy,
		Proxy: auth.ProxyConfig{
			SharedSecret: []byte(testProxySecret),
			LogoutURL:    "https://proxy.example/logout",
		},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{
		Authenticator: authn,
		PublicURL:     "https://spinoza.example.com",
	})
	for _, test := range []struct {
		name      string
		method    string
		fetchSite string
		want      int
	}{
		{name: "GET cross site", method: http.MethodGet, fetchSite: "cross-site", want: http.StatusForbidden},
		{name: "POST cross site", method: http.MethodPost, fetchSite: "cross-site", want: http.StatusForbidden},
		{name: "POST without metadata", method: http.MethodPost, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, pathLogout, http.NoBody)
			request.Host = "spinoza.example.com"
			request.Header.Set("Origin", "null")
			if test.fetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			}
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
			recorded := httptest.NewRecorder()

			srv.Handler().ServeHTTP(recorded, request)

			if recorded.Code != test.want {
				t.Fatalf("status = %d, want %d", recorded.Code, test.want)
			}
			if recorded.Header().Get("Set-Cookie") != "" {
				t.Fatal("a cross-site navigation cleared the session cookie")
			}
			if recorded.Header().Get("Location") != "" {
				t.Fatalf("location = %q, want no provider logout redirect", recorded.Header().Get("Location"))
			}
		})
	}
}

func TestASameSiteFormCanSignSomeoneOutWhenTheBrowserWithholdsItsOrigin(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	authn, err := auth.New(t.Context(), auth.Config{
		Mode: auth.ModeProxy,
		Proxy: auth.ProxyConfig{
			SharedSecret: []byte(testProxySecret),
			LogoutURL:    "https://proxy.example/logout",
		},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{
		Authenticator: authn,
		PublicURL:     "https://spinoza.example.com",
	})
	request := httptest.NewRequest(http.MethodPost, pathLogout, http.NoBody)
	request.Host = "spinoza.example.com"
	request.Header.Set("Origin", "null")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.Header.Set("Sec-Fetch-Dest", "document")
	recorded := httptest.NewRecorder()

	srv.Handler().ServeHTTP(recorded, request)

	if recorded.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusFound)
	}
	if recorded.Header().Get("Location") != "https://proxy.example/logout" {
		t.Fatalf("location = %q, want the provider logout", recorded.Header().Get("Location"))
	}
}

func TestWhereALoginLandsIsWhereSpinozaIsPublished(t *testing.T) {
	cases := map[string]string{
		"https://spinoza.example.com":       "https://spinoza.example.com",
		"https://spinoza.example.com:8443/": "https://spinoza.example.com:8443",
		"":                                  "",
		"://nope":                           "",
		"spinoza.example.com":               "",
	}
	for given, want := range cases {
		t.Run(given, func(t *testing.T) {
			if got := originOfURL(given); got != want {
				t.Fatalf("origin = %q, want %q", got, want)
			}
		})
	}
}

func TestTheSignInPageCarriesNothingOfTheCluster(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	held := settings.Memory()
	privateObject := "/v1/pods/team-a/storefront"
	mergeErr := held.Merge(map[string]string{
		checks.MutesKey: checks.EncodeMutes(map[string][]checks.Mute{
			"cluster": {{Check: "privileged", Ref: privateObject}},
		}),
	})
	if mergeErr != nil {
		t.Fatalf("holding a mute: %v", mergeErr)
	}
	srv.UseSettings(held)
	authn, err := auth.New(t.Context(), auth.Config{
		Mode:  auth.ModeProxy,
		Proxy: auth.ProxyConfig{SharedSecret: []byte(testProxySecret)},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	_, anonymousPage := asUser(t, ts, http.MethodGet, "/", "", "")
	if strings.Contains(anonymousPage, privateObject) {
		t.Fatalf("the sign-in page handed a stranger what this cluster mutes:\n%s", anonymousPage)
	}
	if strings.Contains(anonymousPage, "__SPINOZA_SETTINGS__") {
		t.Fatal("the sign-in page carried the settings of a cluster nobody has signed in to")
	}

	_, signedInPage := asUser(t, ts, http.MethodGet, "/", "alice", "")
	if strings.Contains(signedInPage, privateObject) {
		t.Fatalf("a signed-in reader received mute metadata through the page:\n%s", signedInPage)
	}
}

func TestAProfileAnswersToAdminsOnly(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	srv.UseProfiler(true)
	authn, err := auth.New(t.Context(), auth.Config{
		Mode:        auth.ModeProxy,
		DefaultRole: auth.RoleViewer,
		AdminGroups: []string{"platform-admins"},
		Proxy:       auth.ProxyConfig{SharedSecret: []byte(testProxySecret)},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	refused, message := asUser(t, ts, http.MethodGet, "/debug/pprof/heap", "alice", "")
	if refused.StatusCode != http.StatusForbidden {
		t.Fatalf("a viewer got %d from the heap profile, which holds everything the caches do", refused.StatusCode)
	}
	if !strings.Contains(message, "this needs admin") {
		t.Fatalf("body = %q, want it to name the role", message)
	}

	allowed, _ := asUser(t, ts, http.MethodGet, "/debug/pprof/symbol", "alice", "platform-admins")
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("an admin got %d from a profile", allowed.StatusCode)
	}
}

func TestAProfileIsOpenOnYourOwnMachine(t *testing.T) {
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), testToken)
	srv.UseProfiler(true)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, _ := asUser(t, ts, http.MethodGet, "/debug/pprof/symbol", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d when you are the only one here", resp.StatusCode, http.StatusOK)
	}
}
