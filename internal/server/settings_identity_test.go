package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/settings"
)

func TestClusterSettingsWithoutASignedInUserAreEmptyAndReadOnly(t *testing.T) {
	store := settings.Memory()
	if err := store.Merge(map[string]string{"spinoza.theme.v1": `"nord"`}); err != nil {
		t.Fatalf("preload settings: %v", err)
	}
	srv := New(&stubBackendCluster{}, testAssets(), "")
	srv.UseClusterAuth(ClusterAuth{})
	srv.UseSettings(store)
	req := httptestRequest(http.MethodGet, "/api/settings")

	if got := srv.settingsFor(req); len(got) != 0 {
		t.Fatalf("settings = %v, want none without a signed-in user", got)
	}
	if got := srv.settingFor(req, store.All(), "spinoza.theme.v1"); got != "" {
		t.Fatalf("setting = %q, want none without a signed-in user", got)
	}
	_, err := srv.valuesToStore(req, map[string]string{"spinoza.theme.v1": `"solarized"`})
	if err == nil || !strings.Contains(err.Error(), "signed-in user") {
		t.Fatalf("write error = %v, want a signed-in user refusal", err)
	}
}

func TestSettingsPrefixRejectsAnIdentityWithNoUsername(t *testing.T) {
	req := httptestRequest(http.MethodGet, "/api/settings")
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{Role: auth.RoleViewer}))

	if prefix, ok := settingsPrefix(req); ok || prefix != "" {
		t.Fatalf("prefix = %q, ok = %v, want an empty username rejected", prefix, ok)
	}
}

func httptestRequest(method, target string) *http.Request {
	return httptest.NewRequest(method, target, http.NoBody)
}
