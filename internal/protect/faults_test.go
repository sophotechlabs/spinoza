package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheDefaultPathNeedsAConfigDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := DefaultPath()

	if err == nil {
		t.Skip("this platform still names a config directory without HOME")
	}
	if !strings.Contains(err.Error(), "protected clusters") {
		t.Fatalf("error = %q, want it to say what it was doing", err.Error())
	}
}

func TestAProtectionThatCannotBeWrittenIsReported(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "spinoza", "protected.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if chmodErr := os.Chmod(root, 0o500); chmodErr != nil {
		t.Fatalf("chmod: %v", chmodErr)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	setErr := store.Set("https://kind-spinoza", true)

	if setErr == nil {
		t.Skip("this filesystem let the write through")
	}
	if !strings.Contains(setErr.Error(), "protected clusters") {
		t.Fatalf("error = %q, want it to say what it was doing", setErr.Error())
	}
}

func TestAProtectionThatCannotBeMovedIntoPlaceIsReported(t *testing.T) {
	root := t.TempDir()
	inTheWay := filepath.Join(root, "protected.json")
	store, err := Open(inTheWay)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if mkErr := os.Mkdir(inTheWay, 0o700); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}

	setErr := store.Set("https://kind-spinoza", true)

	if setErr == nil {
		t.Fatal("Set returned nil error with a directory sitting where the file goes")
	}
	if !strings.Contains(setErr.Error(), "protected clusters") {
		t.Fatalf("error = %q, want it to say what it was doing", setErr.Error())
	}
	left, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatalf("read dir: %v", readErr)
	}
	for _, entry := range left {
		if strings.HasPrefix(entry.Name(), "protected-") {
			t.Fatalf("%s was left behind after the failed move", entry.Name())
		}
	}
}
