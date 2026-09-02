package protect

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestCanceledStoreDoesNotKeepAProtectionDecision(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	path := storePath(t)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cancel()

	err = store.Set(remote, true)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("set error = %v, want cancellation", err)
	}
	if store.Verdict(remote) != api.ProtectionUnknown {
		t.Fatalf("verdict = %q, want no canceled decision", store.Verdict(remote))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state file error = %v, want no state file", statErr)
	}
}

func TestZeroValueStoreKeepsAnInMemoryDecision(t *testing.T) {
	var store Store

	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q, want protected", store.Verdict(remote))
	}
}

func TestSetRejectsAnOversizedDecisionWithoutRememberingIt(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	server := "https://remote.example/" + strings.Repeat("x", maxFileBytes)

	err := store.Set(server, true)

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("set error = %v, want the state size limit", err)
	}
	if len(store.clusters) != 0 {
		t.Fatalf("clusters = %d, want memory unchanged", len(store.clusters))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state file error = %v, want no oversized file", statErr)
	}
}

func TestWriteReportsAtomicReplacementFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "protected.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: path}

	err := store.write(map[string]bool{remote: true})

	if err == nil || !strings.Contains(err.Error(), "protected clusters") {
		t.Fatalf("write error = %v, want protected state context", err)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "protected-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want failed temporary state removed", leftovers)
	}
}
