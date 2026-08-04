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

func TestGuardRefusesACrossSiteFetch(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; a page on another site may not reach the api", res.StatusCode)
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

func TestGuardRefusesACrossOriginRead(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestGuardRefusesARebottledHost(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Host = "evil.example:34115"
	req.Header.Set("Origin", "http://evil.example:34115")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestGuardRefusesACrossOriginWebsocket(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	_, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("a cross-origin page reached the socket")
	}
}

func TestGuardAdmitsTheDesktopWebview(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", "wails://wails")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestGuardRefusesCrossOriginAssets(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/index.html", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestHealthzAnswersALocalProbe(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want a probe with no Origin to pass", res.StatusCode)
	}
}

func TestHealthzRefusesAForeignOrigin(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; no route should answer a foreign page", res.StatusCode)
	}
}

func TestGuardAdmitsTheWindowsDesktopWebview(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Host = "wails.localhost"
	req.Header.Set("Origin", "http://wails.localhost")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; wails on Windows serves the app from http://wails.localhost", res.StatusCode)
	}
}
