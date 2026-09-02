package filetx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestReadReportsAPathThatCanBeInspectedButNotOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, 0o700)
	})

	_, err := Read(path, 10)

	if errors.Is(err, fs.ErrPermission) {
		return
	}
	if err == nil {
		t.Fatal("an unreadable path was returned as file contents")
	}
	t.Skipf("this platform rejected the path after opening it: %v", err)
}
