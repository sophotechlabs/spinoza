package main

import (
	"os"
	"path/filepath"
	"testing"

	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
)

func TestTheFlagAloneAllowsANodeShell(t *testing.T) {
	allow := allowNodeShell(true, settingsstore.Memory())

	if !allow() {
		t.Fatal("--node-shell did not allow a node shell")
	}
}

func TestWithoutTheFlagTheStoredSettingDecides(t *testing.T) {
	store := settingsstore.Memory()
	allow := allowNodeShell(false, store)

	if allow() {
		t.Fatal("a node shell was allowed before anything turned it on")
	}

	err := store.Replace(map[string]string{settingsstore.NodeShellKey: "on"})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	if !allow() {
		t.Fatal("turning the setting on did not reach a shell asked for afterwards")
	}

	off := store.Replace(map[string]string{})
	if off != nil {
		t.Fatalf("replace: %v", off)
	}

	if allow() {
		t.Fatal("turning the setting off left node shells allowed")
	}
}

func TestTheStoreIsReadFromTheUsualPlace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := settingsstore.DefaultPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700)
	if mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}
	body := `{"values":{"` + settingsstore.NodeShellKey + `":"on"}}`
	writeErr := os.WriteFile(path, []byte(body), 0o600)
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	store := settingsStore()

	if !store.On(settingsstore.NodeShellKey) {
		t.Fatal("the settings written on disk were not read back")
	}
}

func TestNowhereToKeepSettingsStillLeavesAStore(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	store := settingsStore()

	if store == nil {
		t.Fatal("settings with nowhere to live left no store at all")
	}
	err := store.Replace(map[string]string{settingsstore.NodeShellKey: "on"})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if !store.On(settingsstore.NodeShellKey) {
		t.Fatal("a store with nowhere to write forgot what it was told")
	}
}

func TestUnreadableSettingsStillLeaveAStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := settingsstore.DefaultPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700)
	if mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}
	writeErr := os.WriteFile(path, []byte("{not json"), 0o600)
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	store := settingsStore()

	if store == nil {
		t.Fatal("settings that cannot be read left no store at all")
	}
	if store.On(settingsstore.NodeShellKey) {
		t.Fatal("settings that cannot be read turned a node shell on")
	}
}
