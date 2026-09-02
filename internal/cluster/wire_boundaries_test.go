package cluster

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

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
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path := filepath.Join(root, "spinoza", "protected.json")
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
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path := filepath.Join(root, "spinoza", "kubeconfigs.json")
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
