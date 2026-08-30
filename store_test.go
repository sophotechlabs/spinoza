package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/history"
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

	err := store.Merge(map[string]string{settingsstore.NodeShellKey: "on"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if !allow() {
		t.Fatal("turning the setting on did not reach a shell asked for afterwards")
	}

	off := store.Merge(map[string]string{settingsstore.NodeShellKey: "off"})
	if off != nil {
		t.Fatalf("merge: %v", off)
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
	err := store.Merge(map[string]string{settingsstore.NodeShellKey: "on"})
	if err != nil {
		t.Fatalf("merge: %v", err)
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

func TestBaselinesAreKeptBesideTheSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	store := baselineStore()

	if err := store.Save("https://one.example", checks.Baseline{TakenAt: "2026-08-30T00:00:00Z"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := store.Load("https://one.example"); !ok {
		t.Fatal("a baseline saved through the store could not be read back")
	}
}

func TestNowhereToKeepBaselinesStillLeavesAStore(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	store := baselineStore()

	if _, ok := store.Load("https://one.example"); ok {
		t.Fatal("a store with nowhere to write produced a baseline")
	}
}

func TestHistoryIsKeptInTheUsualPlace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	store := historyStore(t.Context())
	t.Cleanup(func() { _ = store.Close() })

	if store.Reason() != "" {
		t.Fatalf("reason = %q, want a store that records", store.Reason())
	}
	path, err := history.DefaultPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("the history file was not created at %s: %v", path, statErr)
	}
}

func TestNowhereToKeepHistoryStillLeavesAStore(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	store := historyStore(t.Context())
	t.Cleanup(func() { _ = store.Close() })

	if store == nil {
		t.Fatal("history with nowhere to live left no store at all")
	}
	if store.Reason() == "" {
		t.Skip("this platform still names a config directory without HOME")
	}
	if err := store.For("https://p-mk1:6443").Record(t.Context(), history.Entry{Name: "web"}); err != nil {
		t.Fatalf("record: %v, want a quiet no-op", err)
	}
}

func TestAHistoryFileThatCannotBeReadStillLeavesAStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := history.DefaultPath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(path, []byte("not a database"), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	store := historyStore(t.Context())
	t.Cleanup(func() { _ = store.Close() })

	if store == nil {
		t.Fatal("a broken history file left no store at all")
	}
	if store.Reason() == "" {
		t.Fatal("a broken history file was reported as recording")
	}
}
