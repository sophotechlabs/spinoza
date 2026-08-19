package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func profilerServer(t *testing.T) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.UseProfiler(true)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestTheProfilerIsServedWhenEnabled(t *testing.T) {
	ts := profilerServer(t)

	res := get(t, ts, "/debug/pprof/", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the profiler served: %s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "goroutine") {
		t.Fatalf("body = %s, want the profile index", body)
	}
}

func TestTheProfilerIsOffByDefault(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/debug/pprof/", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; the profiler stays off without -pprof", res.StatusCode)
	}
}

func TestTheProfilerNeedsTheTokenLikeEverythingElse(t *testing.T) {
	ts := profilerServer(t)

	res := get(t, ts, "/debug/pprof/", nil)

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; the profiler is behind the same guard", res.StatusCode)
	}
}

func TestTheProfilerRefusesAForeignOrigin(t *testing.T) {
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.UseProfiler(true)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	res := get(t, ts, "/debug/pprof/", func(r *http.Request) {
		r.Header.Set("Origin", "https://evil.example")
	})

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
}
