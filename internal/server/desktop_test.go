package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func desktopFeedRequest(t *testing.T, ts *httptest.Server, origin, fetchSite string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/ws?"+AuthParam+"="+testToken, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Sec-Fetch-Site", fetchSite)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestTheDesktopWebviewReachesTheFeed(t *testing.T) {
	cases := map[string]struct {
		origin    string
		fetchSite string
	}{
		"macos webview":   {origin: "wails://wails", fetchSite: "cross-site"},
		"windows webview": {origin: "http://wails.localhost", fetchSite: "cross-site"},
		"dev webview":     {origin: "wails://wails.localhost", fetchSite: "cross-site"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ts := tokenServer(t)

			res := desktopFeedRequest(t, ts, tc.origin, tc.fetchSite)

			if res.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("status = %d, want 101; the desktop app connects its socket straight to the loopback port", res.StatusCode)
			}
		})
	}
}

func TestAForeignPageStillCannotReachTheFeed(t *testing.T) {
	ts := tokenServer(t)

	res := desktopFeedRequest(t, ts, "https://evil.example", "cross-site")

	if res.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("a page on another site opened the feed")
	}
}
