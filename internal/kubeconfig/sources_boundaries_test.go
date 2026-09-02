package kubeconfig

import (
	"os"
	"strings"
	"testing"
)

func TestResolveReportsAWorkingDirectoryThatDisappeared(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("this platform keeps the working directory in place: %v", err)
	}

	_, err := Resolve("config")

	if err == nil {
		t.Fatal("a relative path resolved without a working directory")
	}
	if !strings.Contains(err.Error(), "kubeconfig config") {
		t.Fatalf("resolve error = %q, want the path named", err.Error())
	}
}
