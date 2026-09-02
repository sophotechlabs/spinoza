package settings

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanceledStoreDoesNotKeepMergedSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	path := tempPath(t)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cancel()

	err = store.Merge(map[string]string{"theme": "nord"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("merge error = %v, want cancellation", err)
	}
	if len(store.All()) != 0 {
		t.Fatalf("settings = %v, want no canceled merge", store.All())
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state file error = %v, want no state file", statErr)
	}
}

func TestWriteReportsAtomicReplacementFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: path}

	err := store.write(map[string]string{"theme": "nord"})

	if err == nil || !strings.Contains(err.Error(), "settings") {
		t.Fatalf("write error = %v, want settings context", err)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "settings-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want failed temporary state removed", leftovers)
	}
}
