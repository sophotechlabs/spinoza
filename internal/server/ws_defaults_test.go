package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func TestKnownIdentityWithoutANameIsAnonymous(t *testing.T) {
	if got := identityName(auth.Identity{}, true); got != "anonymous" {
		t.Fatalf("identity = %q, want anonymous", got)
	}
}

func TestZeroLiveConnectionLimitsUseTheIdentityDefault(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), "")
	srv.liveLimit = 0
	srv.identityLimit = 0
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	releases := make([]func(), 0, defaultIdentityConnectionLimit)
	for range defaultIdentityConnectionLimit {
		release, ok := srv.claimLiveConnection(req)
		if !ok {
			t.Fatalf("connection %d was refused before the default identity limit", len(releases)+1)
		}
		releases = append(releases, release)
	}
	if release, ok := srv.claimLiveConnection(req); ok || release != nil {
		t.Fatal("a connection beyond the default identity limit was accepted")
	}
	for _, release := range releases {
		release()
	}
}

func TestZeroLiveConnectionLimitUsesTheGlobalDefault(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), "")
	srv.liveLimit = 0
	srv.identityLimit = 0
	releases := make([]func(), 0, defaultLiveConnectionLimit)
	for at := range defaultLiveConnectionLimit {
		req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
		who := auth.Identity{User: fmt.Sprintf("user-%d@example.com", at)}
		req = req.WithContext(auth.WithIdentity(req.Context(), who))
		release, ok := srv.claimLiveConnection(req)
		if !ok {
			t.Fatalf("connection %d was refused before the default global limit", at+1)
		}
		releases = append(releases, release)
	}
	extra := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	extra = extra.WithContext(auth.WithIdentity(extra.Context(), auth.Identity{User: "extra@example.com"}))
	if release, ok := srv.claimLiveConnection(extra); ok || release != nil {
		t.Fatal("a connection beyond the default global limit was accepted")
	}
	for _, release := range releases {
		release()
	}
}
