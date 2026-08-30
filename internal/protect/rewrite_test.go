package protect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func seeded(t *testing.T, clusters map[string]bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "protected.json")
	body, err := json.Marshal(state{Clusters: clusters})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(path, body, 0o600); writeErr != nil {
		t.Fatalf("seed: %v", writeErr)
	}
	return path
}

func TestEveryShapeAFileHoldsKeepsItsVerdictAcrossTheRewrite(t *testing.T) {
	written := map[string]bool{
		"https://10.10.0.1:6443":    true,
		"https://10.10.1.1:6443":    true,
		"https://127.0.0.1:50741":   true,
		"https://34.13.149.2":       true,
		"https://35.204.10.181":     true,
		"HTTPS://Cluster.Example/":  true,
		"https://one.example:443":   false,
		"https://[fd00::1]:6443":    true,
		"https://two.example:6443/": false,
	}
	path := seeded(t, written)
	before, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	was := map[string]string{}
	for server := range written {
		was[server] = before.Verdict(server)
	}

	if setErr := before.Set("https://never-seen.example:6443", true); setErr != nil {
		t.Fatalf("set: %v", setErr)
	}

	after, reopenErr := Open(path)
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	for server, verdict := range was {
		now := after.Verdict(server)
		if now != verdict {
			t.Fatalf("%s read %q before the rewrite and %q after", server, verdict, now)
		}
	}
}

func TestWhenTwoSpellingsMeetTheProtectedOneWins(t *testing.T) {
	path := seeded(t, map[string]bool{
		"https://one.example":     true,
		"https://one.example:443": false,
	})

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if store.Verdict("https://one.example") != api.ProtectionProtected {
		t.Fatalf("verdict = %q, want the protected spelling to win the collision",
			store.Verdict("https://one.example"))
	}
	if store.Verdict("https://one.example:443") != api.ProtectionProtected {
		t.Fatal("the same cluster read differently through its other spelling")
	}
}

func TestTheRewriteLeavesOneKeyPerCluster(t *testing.T) {
	path := seeded(t, map[string]bool{
		"https://one.example":      true,
		"https://one.example:443":  true,
		"https://one.example:443/": true,
	})
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if setErr := store.Set("https://two.example:6443", true); setErr != nil {
		t.Fatalf("set: %v", setErr)
	}

	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	var saved state
	if unmarshalErr := json.Unmarshal(body, &saved); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	if len(saved.Clusters) != 2 {
		t.Fatalf("the file holds %d keys, want one per cluster: %v", len(saved.Clusters), saved.Clusters)
	}
}
