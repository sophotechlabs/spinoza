package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}
