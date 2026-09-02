package kubeconfig

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
	if !strings.Contains(err.Error(), "kubeconfig list") {
		t.Fatalf("error = %q, want it to say what it was doing", err.Error())
	}
}

func TestAListThatCannotBeWrittenIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	store, err := Open(t.Context(), filepath.Join(root, "spinoza", "kubeconfigs.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	addErr := store.Add(filepath.Join(root, "config"))

	if addErr == nil {
		t.Skip("this filesystem let the write through")
	}
	if !strings.Contains(addErr.Error(), "kubeconfig list") {
		t.Fatalf("error = %q, want it to say what it was doing", addErr.Error())
	}
}

func TestATempFileThatCannotBeMadeIsReported(t *testing.T) {
	root := t.TempDir()
	store, err := Open(t.Context(), filepath.Join(root, "kubeconfigs.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if chmodErr := os.Chmod(root, 0o500); chmodErr != nil {
		t.Fatalf("chmod: %v", chmodErr)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	addErr := store.Add(filepath.Join(root, "config"))

	if addErr == nil {
		t.Skip("this filesystem let the write through")
	}
	if !strings.Contains(addErr.Error(), "kubeconfig list") {
		t.Fatalf("error = %q, want it to say what it was doing", addErr.Error())
	}
}

func TestAListThatCannotBeMovedIntoPlaceIsReported(t *testing.T) {
	root := t.TempDir()
	inTheWay := filepath.Join(root, "kubeconfigs.json")
	store, err := Open(t.Context(), inTheWay)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if mkErr := os.Mkdir(inTheWay, 0o700); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}

	addErr := store.Add(filepath.Join(root, "config"))

	if addErr == nil {
		t.Fatal("Add returned nil error with a directory sitting where the file goes")
	}
	if !strings.Contains(addErr.Error(), "kubeconfig list") {
		t.Fatalf("error = %q, want it to say what it was doing", addErr.Error())
	}
	left, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatalf("read dir: %v", readErr)
	}
	for _, entry := range left {
		if strings.HasPrefix(entry.Name(), "kubeconfigs-") {
			t.Fatalf("%s was left behind after the failed move", entry.Name())
		}
	}
}
