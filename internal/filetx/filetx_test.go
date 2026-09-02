package filetx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRejectsAFileAboveItsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Read(path, 3)

	if err == nil || !strings.Contains(err.Error(), "larger than 3 bytes") {
		t.Fatalf("read error = %v, want the size limit", err)
	}
}
