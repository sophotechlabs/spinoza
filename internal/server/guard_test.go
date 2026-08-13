package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

func TestAllowedOrigin(t *testing.T) {
	cases := []struct {
		name      string
		origin    string
		authority string
		want      bool
	}{
		{"absent", "", "127.0.0.1:34115", true},
		{"served page", "http://127.0.0.1:34115", "127.0.0.1:34115", true},
		{"localhost name", "http://localhost:34115", "localhost:34115", true},
		{"vite dev server proxying to itself", "http://localhost:5173", "localhost:5173", true},
		{"ipv6 loopback", "http://[::1]:34115", "[::1]:34115", true},
		{"wails webview", "wails://wails", "127.0.0.1:51234", true},
		{"wails dev webview", "wails://wails.localhost:34115", "127.0.0.1:51234", true},
		{"wails windows webview", "http://wails.localhost", "127.0.0.1:51234", true},
		{"another loopback port", "http://localhost:5173", "127.0.0.1:34115", false},
		{"the same host on another port", "http://127.0.0.1:9999", "127.0.0.1:34115", false},
		{"loopback by another name", "http://localhost:34115", "127.0.0.1:34115", false},
		{"a hostile wails origin", "wails://evil.example", "127.0.0.1:34115", false},
		{"a hostile page", "https://evil.example", "127.0.0.1:34115", false},
		{"a rebound name", "http://evil.example:34115", "127.0.0.1:34115", false},
		{"sandboxed iframe", "null", "127.0.0.1:34115", false},
		{"file url", "file://", "127.0.0.1:34115", false},
		{"unparseable", "http://[::1", "127.0.0.1:34115", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allowedOrigin(tc.origin, tc.authority)
			if got != tc.want {
				t.Fatalf("allowedOrigin(%q, %q) = %v", tc.origin, tc.authority, got)
			}
		})
	}
}

func TestLoopbackAuthority(t *testing.T) {
	cases := []struct {
		authority string
		want      bool
	}{
		{"127.0.0.1:34115", true},
		{"127.0.0.1", true},
		{"localhost:5173", true},
		{"[::1]:34115", true},
		{"[::1]", true},
		{"wails.localhost", true},
		{"wails.localhost:34115", true},
		{"notwails.localhost:34115", false},
		{"127.1.2.3:34115", true},
		{"evil.example:34115", false},
		{"192.168.1.10:34115", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.authority, func(t *testing.T) {
			got := loopbackAuthority(tc.authority)
			if got != tc.want {
				t.Fatalf("loopbackAuthority(%q) = %v", tc.authority, got)
			}
		})
	}
}

func guardServer(t *testing.T) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := httptest.NewServer(New(fixed(mgr), testAssets(), testToken).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func crossSiteNavigation(dest string) map[string]string {
	return map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": dest,
	}
}

func TestGuardAdmissionMatrix(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		path    string
		token   string
		host    string
		headers map[string]string
		want    int
	}{
		{
			name:    "an api fetch from another site",
			path:    "/api/resources",
			token:   "header",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			name:    "an api read from a foreign origin",
			path:    "/api/resources",
			token:   "header",
			headers: map[string]string{"Origin": "https://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:    "a rebottled host",
			path:    "/api/resources",
			token:   "header",
			host:    "evil.example:34115",
			headers: map[string]string{"Origin": "http://evil.example:34115"},
			want:    http.StatusForbidden,
		},
		{
			name:    "the desktop webview",
			path:    "/api/resources",
			token:   "header",
			headers: map[string]string{"Origin": "wails://wails"},
			want:    http.StatusOK,
		},
		{
			name:    "the windows desktop webview",
			path:    "/api/resources",
			token:   "header",
			host:    "wails.localhost",
			headers: map[string]string{"Origin": "http://wails.localhost"},
			want:    http.StatusOK,
		},
		{
			name:    "the index from a foreign origin",
			path:    "/index.html",
			token:   "header",
			headers: map[string]string{"Origin": "https://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:  "healthz with no origin",
			path:  "/healthz",
			token: "header",
			want:  http.StatusOK,
		},
		{
			name:    "healthz from a foreign origin",
			path:    "/healthz",
			token:   "header",
			headers: map[string]string{"Origin": "https://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:    "a cross-site top-level navigation carrying the token",
			path:    "/",
			token:   "query",
			headers: crossSiteNavigation("document"),
			want:    http.StatusOK,
		},
		{
			name:    "a cross-site navigation without the token",
			path:    "/",
			headers: crossSiteNavigation("document"),
			want:    http.StatusUnauthorized,
		},
		{
			name:    "a framed cross-site navigation",
			path:    "/",
			token:   "query",
			headers: crossSiteNavigation("iframe"),
			want:    http.StatusForbidden,
		},
		{
			name: "an asset chunk without a token",
			path: "/assets/chunk.js",
			want: http.StatusOK,
		},
		{
			name:   "a head probe of an asset chunk",
			method: http.MethodHead,
			path:   "/assets/chunk.js",
			want:   http.StatusOK,
		},
		{
			name: "the assets directory without a token",
			path: "/assets/",
			want: http.StatusUnauthorized,
		},
		{
			name: "the favicon without a token",
			path: "/favicon.svg",
			want: http.StatusOK,
		},
		{
			name: "the index without a token",
			path: "/",
			want: http.StatusUnauthorized,
		},
		{
			name: "a root file without a token",
			path: "/app.js",
			want: http.StatusUnauthorized,
		},
		{
			name:    "a cross-site pull of an asset chunk",
			path:    "/assets/chunk.js",
			headers: map[string]string{"Origin": "https://evil.example"},
			want:    http.StatusForbidden,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := guardServer(t)
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			target := srv.URL + tc.path
			if tc.token == "query" {
				target += "?token=" + testToken
			}
			req, err := http.NewRequest(method, target, http.NoBody)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if tc.token == "header" {
				req.Header.Set(AuthHeader, testToken)
			}
			if tc.host != "" {
				req.Host = tc.host
			}
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestGuardRefusesACrossOriginWebsocket(t *testing.T) {
	srv := guardServer(t)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	_, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("a cross-origin page reached the socket")
	}
}
