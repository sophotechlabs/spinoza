package cluster

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func configHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("AppData", root)
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("config directory: %v", err)
	}
	return dir
}

func TestProtectionFallsBackToMemoryWithoutAConfigDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	store := openProtection(t.Context())

	if err := store.Set("https://cluster.example:6443", true); err != nil {
		t.Fatalf("set in-memory protection: %v", err)
	}
	if store.Verdict("https://cluster.example:6443") != api.ProtectionProtected {
		t.Fatal("the fallback protection store did not retain its decision")
	}
}

func TestKubeconfigListFallsBackToMemoryWithoutAConfigDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	store := openStore(t.Context())

	if err := store.Add("/tmp/cluster.yaml"); err != nil {
		t.Fatalf("add in-memory kubeconfig: %v", err)
	}
	paths := store.Paths()
	if len(paths) != 1 || paths[0] != "/tmp/cluster.yaml" {
		t.Fatalf("paths = %v, want the in-memory kubeconfig", paths)
	}
}

func TestProtectionStartsEmptyWhenItsSavedFileIsMalformed(t *testing.T) {
	path := filepath.Join(configHome(t), "spinoza", "protected.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write malformed protection: %v", err)
	}

	store := openProtection(t.Context())

	if store.Verdict("https://cluster.example:6443") != api.ProtectionUnknown {
		t.Fatal("malformed saved protection became a decision")
	}
}

func TestKubeconfigListStartsEmptyWhenItsSavedFileIsMalformed(t *testing.T) {
	path := filepath.Join(configHome(t), "spinoza", "kubeconfigs.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write malformed kubeconfig list: %v", err)
	}

	store := openStore(t.Context())

	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want no paths from malformed state", store.Paths())
	}
}

func TestTheWiredStoresReadWhatTheConfigDirectoryHolds(t *testing.T) {
	dir := filepath.Join(configHome(t), "spinoza")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	kubeconfigs := filepath.Join(dir, "kubeconfigs.json")
	if err := os.WriteFile(kubeconfigs, []byte(`{"kubeconfigs":["/tmp/one.yaml"]}`), 0o600); err != nil {
		t.Fatalf("write kubeconfig list: %v", err)
	}
	protection := filepath.Join(dir, "protected.json")
	if err := os.WriteFile(protection, []byte(`{"clusters":{"https://cluster.example:6443":true}}`), 0o600); err != nil {
		t.Fatalf("write protection: %v", err)
	}

	paths := openStore(t.Context()).Paths()
	verdict := openProtection(t.Context()).Verdict("https://cluster.example:6443")

	if len(paths) != 1 || paths[0] != "/tmp/one.yaml" {
		t.Fatalf("paths = %v, want the one kubeconfig the file names", paths)
	}
	if verdict != api.ProtectionProtected {
		t.Fatalf("verdict = %v, want %v", verdict, api.ProtectionProtected)
	}
}
