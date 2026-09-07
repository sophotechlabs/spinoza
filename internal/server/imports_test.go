package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/settings"
)

func TestImportedFindingsAreReadOnlyOnTheOperatorsOwnMachine(t *testing.T) {
	mgr, _ := testManager(t)
	local := New(fixed(mgr), testAssets(), testToken)
	store := settings.Memory()
	if err := store.Merge(map[string]string{checks.ImportsKey: "/scans/trivy.json\n"}); err != nil {
		t.Fatal(err)
	}
	local.UseSettings(store)
	req := httptest.NewRequest(http.MethodGet, "/api/checks", http.NoBody)
	if paths := local.checkFilterOn(req, "cluster").Imports; len(paths) != 1 || paths[0] != "/scans/trivy.json" {
		t.Fatalf("local imports = %v, want the configured path", paths)
	}

	served, ts := settingsServerForUsers(t, settings.Memory())
	resp, body := putUserSettings(t, ts, "alice@example.com", map[string]string{checks.ImportsKey: "/scans/trivy.json\n"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	alice := httptest.NewRequest(http.MethodGet, "/api/checks", http.NoBody)
	alice = alice.WithContext(auth.WithIdentity(alice.Context(), auth.Identity{User: "alice@example.com"}))
	if paths := served.checkFilterOn(alice, "cluster").Imports; len(paths) != 0 {
		t.Fatalf("a served cluster read import paths %v from a user's settings", paths)
	}
}
