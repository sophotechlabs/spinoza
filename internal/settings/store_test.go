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
	err := store.Replace(map[string]string{NodeShellKey: "on", "other": "true"})
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

	err := store.Replace(map[string]string{"spinoza.theme.v1": `"nord"`})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	if openAt(t, path).All()["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("the theme did not survive: %v", openAt(t, path).All())
	}
}

func TestReplacingDropsWhatIsNoLongerThere(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Replace(map[string]string{"a": "1", "b": "2"})

	_ = store.Replace(map[string]string{"a": "1"})

	values := openAt(t, path).All()
	if len(values) != 1 || values["a"] != "1" {
		t.Fatalf("the old key stayed behind: %v", values)
	}
}

func TestReplacingWithNothingEmptiesTheStore(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Replace(map[string]string{"a": "1"})

	err := store.Replace(nil)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	if len(openAt(t, path).All()) != 0 {
		t.Fatalf("something survived: %v", openAt(t, path).All())
	}
}

func TestTheCallerCannotReachInsideTheStore(t *testing.T) {
	store := openAt(t, tempPath(t))
	_ = store.Replace(map[string]string{"a": "1"})

	store.All()["a"] = "tampered"

	if store.All()["a"] != "1" {
		t.Fatal("the returned map is the store's own")
	}
}

func TestTheStoreKeepsItsFileToItself(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)

	_ = store.Replace(map[string]string{"a": "1"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), fileMode)
	}
}

func TestAStoreWithoutAFileStillAnswers(t *testing.T) {
	store := Memory()

	err := store.Replace(map[string]string{"a": "1"})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if store.All()["a"] != "1" {
		t.Fatal("a store with no file forgot what it was told")
	}
}

func TestUnreadableSettingsAreReported(t *testing.T) {
	path := tempPath(t)
	err := os.MkdirAll(filepath.Dir(path), dirMode)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeErr := os.WriteFile(path, []byte("{not json"), fileMode)
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
	err := os.MkdirAll(path, dirMode)
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
	err := os.WriteFile(blocked, []byte("in the way"), fileMode)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	store, openErr := Open(filepath.Join(blocked, "settings.json"))
	if openErr == nil {
		t.Fatal("a path inside a file was accepted for reading")
	}

	replaceErr := store.Replace(map[string]string{"a": "1"})

	if replaceErr == nil {
		t.Fatal("writing into a file that is not a directory was reported as fine")
	}
}

func TestTheStoredFileIsPlainJSON(t *testing.T) {
	path := tempPath(t)
	store := openAt(t, path)
	_ = store.Replace(map[string]string{"spinoza.theme.v1": `"nord"`})

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
	err := os.Mkdir(path, dirMode)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: path, values: map[string]string{}}

	replaceErr := store.Replace(map[string]string{"a": "1"})

	if replaceErr == nil {
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
	err := os.WriteFile(blocked, []byte("not a directory"), fileMode)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	store := &Store{path: filepath.Join(blocked, "spinoza", "settings.json"), values: map[string]string{}}

	replaceErr := store.Replace(map[string]string{"a": "1"})

	if replaceErr == nil {
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

	replaceErr := store.Replace(map[string]string{"a": "1"})

	if replaceErr == nil {
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

func TestAFileThatCannotBeWrittenIsCleanedUp(t *testing.T) {
	dir := t.TempDir()
	store := &Store{path: filepath.Join(dir, "settings.json"), values: map[string]string{}}
	file, err := os.CreateTemp(dir, "settings-*.json")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	closeErr := file.Close()
	if closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	replaceErr := store.replace(file, []byte("{}"))

	if replaceErr == nil {
		t.Fatal("writing to a closed file reported success")
	}
	if _, statErr := os.Stat(file.Name()); statErr == nil {
		t.Fatal("the half-written file was left behind")
	}
}

func TestAFileThatVanishesBeforeItIsSealedIsReported(t *testing.T) {
	dir := t.TempDir()
	store := &Store{path: filepath.Join(dir, "settings.json"), values: map[string]string{}}
	file, err := os.CreateTemp(dir, "settings-*.json")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	removeErr := os.Remove(file.Name())
	if removeErr != nil {
		t.Fatalf("remove: %v", removeErr)
	}

	replaceErr := store.replace(file, []byte("{}"))

	if replaceErr == nil {
		t.Fatal("sealing a file that is gone reported success")
	}
}
