package kubeconfig

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCanceledStoreDoesNotAddAnEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	path := storePath(t)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cancel()

	err = store.Add("/tmp/one.yaml")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("add error = %v, want cancellation", err)
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want no canceled addition", store.Paths())
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state file error = %v, want no state file", statErr)
	}
}

func TestCanceledStoreDoesNotRemoveAnEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	path := storePath(t)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if addErr := store.Add("/tmp/one.yaml"); addErr != nil {
		t.Fatalf("add: %v", addErr)
	}
	cancel()

	err = store.Remove("/tmp/one.yaml")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("remove error = %v, want cancellation", err)
	}
	if !slices.Equal(store.Paths(), []string{"/tmp/one.yaml"}) {
		t.Fatalf("paths = %v, want the persisted entry retained", store.Paths())
	}
}

func TestMemoryStoreCanRemoveAnEntry(t *testing.T) {
	store := openStore(t, "")
	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := store.Remove("/tmp/one.yaml"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want the in-memory entry removed", store.Paths())
	}
}

func TestRemoveDoesNotReplaceAMalformedListFromStaleMemory(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Add("/tmp/held.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}
	broken := []byte("{not json")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("break list: %v", err)
	}

	err := store.Remove("/tmp/held.yaml")

	if err == nil {
		t.Fatal("a removal over a malformed list reported success")
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read broken list: %v", readErr)
	}
	if !bytes.Equal(body, broken) {
		t.Fatalf("malformed list was replaced with %q", body)
	}
	if !slices.Equal(store.Paths(), []string{"/tmp/held.yaml"}) {
		t.Fatalf("paths = %v, want the last readable list", store.Paths())
	}
}

func TestAddKeepsMemoryUnchangedWhenAtomicReplacementFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfigs.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{ctx: t.Context(), path: path}

	err := store.add("/tmp/one.yaml")

	if err == nil {
		t.Fatal("an entry that could not replace the state directory reported success")
	}
	if len(store.paths) != 0 {
		t.Fatalf("paths = %v, want memory unchanged after the write failure", store.paths)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "kubeconfigs-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want failed temporary state removed", leftovers)
	}
}

func TestAnAbsentFallbackStaysAbsentWhenMadeAbsolute(t *testing.T) {
	if got := absolute(""); got != "" {
		t.Fatalf("absolute empty path = %q, want empty", got)
	}
}
