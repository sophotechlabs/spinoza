package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "settings.json")
}

func openAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return store
}

func TestAFreshStoreHasNothingInIt(t *testing.T) {
	store := openAt(t, tempPath(t))

	if len(store.All()) != 0 {
		t.Fatalf("a fresh store already holds %v", store.All())
	}
}

func TestOnlyTheWordOnTurnsAKeyOn(t *testing.T) {
	store := openAt(t, tempPath(t))
	err := store.Merge(map[string]string{NodeShellKey: "on", "other": "true"})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	if !store.On(NodeShellKey) {
		t.Fatal(`a key holding "on" reads as off`)
	}
	if store.On("other") {
		t.Fatal(`a key holding "true" reads as on`)
	}
	if store.On("missing") {
		t.Fatal("a key that was never written reads as on")
	}
}

func TestWhatIsWrittenComesBack(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)

	err := store.Merge(map[string]string{"spinoza.theme.v1": `"nord"`})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	if openAt(t, path).All()["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("the theme did not survive: %v", openAt(t, path).All())
	}
}

func TestMergingKeepsWhatItWasNotGiven(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Merge(map[string]string{"a": "1", "b": "2"})

	_ = store.Merge(map[string]string{"a": "9"})

	values := openAt(t, path).All()
	if values["a"] != "9" {
		t.Fatalf("a = %q, want the new value", values["a"])
	}
	if values["b"] != "2" {
		t.Fatalf("b = %q, want the untouched value", values["b"])
	}
}

func TestMergingNothingChangesNothing(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Merge(map[string]string{"a": "1"})

	err := store.Merge(nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if openAt(t, path).All()["a"] != "1" {
		t.Fatalf("a was lost: %v", openAt(t, path).All())
	}
}

// Two spinozas run at once, each holding a copy taken when it started. The one
// that writes second must not undo what the first changed.
func TestAKeyWrittenByAnotherProcessSurvives(t *testing.T) {
	path := tempPath(t)
	first := openAt(t, path)
	second := openAt(t, path)
	_ = first.Merge(map[string]string{"spinoza.theme.v1": "borg"})

	_ = second.Merge(map[string]string{"spinoza.namespace.v1": "kube-system"})

	values := openAt(t, path).All()
	if values["spinoza.theme.v1"] != "borg" {
		t.Fatalf("theme = %q, want the other process's value", values["spinoza.theme.v1"])
	}
	if values["spinoza.namespace.v1"] != "kube-system" {
		t.Fatalf("namespace = %q, want this process's value", values["spinoza.namespace.v1"])
	}
}

// A window opened here has to be given what the file holds now, not what this
// process read when it started.
func TestAllPicksUpWhatAnotherProcessWrote(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Merge(map[string]string{"a": "1"})
	_ = openAt(t, path).Merge(map[string]string{"a": "2"})

	if got := store.All()["a"]; got != "2" {
		t.Fatalf("a = %q, want what the file holds now", got)
	}
}

func TestAnUnreadableFileLeavesWhatIsHeldAlone(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Merge(map[string]string{"a": "1"})
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := store.All()["a"]; got != "1" {
		t.Fatalf("a = %q, want the value this process holds", got)
	}
}

func TestTheCallerCannotReachInsideTheStore(t *testing.T) {
	store := openAt(t, tempPath(t))
	_ = store.Merge(map[string]string{"a": "1"})

	store.All()["a"] = "tampered"

	if store.All()["a"] != "1" {
		t.Fatal("the returned map is the store's own")
	}
}

func TestTheStoreKeepsItsFileToItself(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)

	_ = store.Merge(map[string]string{"a": "1"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), 0o600)
	}
}

func TestAStoreWithoutAFileStillAnswers(t *testing.T) {
	store := Memory()

	err := store.Merge(map[string]string{"a": "1"})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if store.All()["a"] != "1" {
		t.Fatal("a store with no file forgot what it was told")
	}
}

func TestUnreadableSettingsAreReported(t *testing.T) {
	path := tempPath(t)
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeErr := os.WriteFile(path, []byte("{not json"), 0o600)
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	store, openErr := Open(path)
	if openErr == nil {
		t.Fatal("unreadable settings were accepted")
	}
	if store == nil {
		t.Fatal("a usable store was not handed back")
	}
	if len(store.All()) != 0 {
		t.Fatalf("the broken file left values behind: %v", store.All())
	}
}

func TestADirectoryInPlaceOfTheFileIsReported(t *testing.T) {
	path := tempPath(t)
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, openErr := Open(path)

	if openErr == nil {
		t.Fatal("a directory was read as settings")
	}
}

func TestWritingWhereItCannotIsReported(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	err := os.WriteFile(blocked, []byte("in the way"), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	store, openErr := Open(filepath.Join(blocked, "settings.json"))
	if openErr == nil {
		t.Fatal("a path inside a file was accepted for reading")
	}

	mergeErr := store.Merge(map[string]string{"a": "1"})

	if mergeErr == nil {
		t.Fatal("writing into a file that is not a directory was reported as fine")
	}
}

func TestTheStoredFileIsPlainJSON(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Merge(map[string]string{"spinoza.theme.v1": `"nord"`})

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var saved state
	unmarshalErr := json.Unmarshal(body, &saved)
	if unmarshalErr != nil {
		t.Fatalf("the file is not json: %v", unmarshalErr)
	}
	if saved.Values["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("the file holds %v", saved.Values)
	}
}

func TestTheDefaultPathSitsBesideTheOtherState(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path: %v", err)
	}
	if filepath.Base(path) != "settings.json" {
		t.Fatalf("path = %s", path)
	}
	if filepath.Base(filepath.Dir(path)) != "spinoza" {
		t.Fatalf("path = %s", path)
	}
}

func TestSettingsThatCannotReplaceTheFileAreNotKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	err := os.Mkdir(path, 0o700)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: path, values: map[string]string{}}

	mergeErr := store.Merge(map[string]string{"a": "1"})

	if mergeErr == nil {
		t.Fatal("settings that could not replace their file reported success")
	}
	if len(store.All()) != 0 {
		t.Fatal("the values were kept in memory after the write failed, so a restart would forget them silently")
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "settings-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want the half-written file cleaned up", leftovers)
	}
}

func TestSettingsAreNotWrittenWhereTheDirectoryCannotBeMade(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	err := os.WriteFile(blocked, []byte("not a directory"), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	store := &Store{path: filepath.Join(blocked, "spinoza", "settings.json"), values: map[string]string{}}

	mergeErr := store.Merge(map[string]string{"a": "1"})

	if mergeErr == nil {
		t.Fatal("settings written under a file reported success")
	}
}

func TestATemporaryFileThatCannotBeMadeIsReported(t *testing.T) {
	dir := t.TempDir()
	readOnly := filepath.Join(dir, "locked")
	err := os.Mkdir(readOnly, 0o500)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: filepath.Join(readOnly, "settings.json"), values: map[string]string{}}

	mergeErr := store.Merge(map[string]string{"a": "1"})

	if mergeErr == nil {
		t.Fatal("a directory that refuses new files reported success")
	}
}

func TestAHomelessAccountHasNoDefaultPath(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := DefaultPath()

	if err == nil {
		t.Skip("this platform still finds a config directory without a home")
	}
}
