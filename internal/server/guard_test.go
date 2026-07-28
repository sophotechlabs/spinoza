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
		name   string
		origin string
		want   bool
	}{
		{"absent", "", true},
		{"served page", "http://127.0.0.1:34115", true},
		{"localhost name", "http://localhost:34115", true},
		{"vite dev server", "http://localhost:5173", true},
		{"ipv6 loopback", "http://[::1]:34115", true},
		{"wails webview", "wails://wails", true},
		{"wails dev webview", "wails://wails.localhost:34115", true},
		{"a hostile page", "https://evil.example", false},
		{"a rebound name", "http://evil.example:34115", false},
		{"sandboxed iframe", "null", false},
		{"file url", "file://", false},
		{"unparseable", "http://[::1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allowedOrigin(tc.origin)
			if got != tc.want {
				t.Fatalf("allowedOrigin(%q) = %v", tc.origin, got)
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
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", nil)
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
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", nil)
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
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
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
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/resources", nil)
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
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/index.html", nil)
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

func TestHealthzStaysOpen(t *testing.T) {
	mgr, _ := testManager(t)
	srv := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
}
