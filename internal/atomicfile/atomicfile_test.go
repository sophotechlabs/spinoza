package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func target(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "nested", "state.json")
}

func TestTheBodyLandsAtThePath(t *testing.T) {
	path := target(t)

	if err := Save(path, "state-*.json", []byte("{}\n")); err != nil {
		t.Fatalf("save: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(body) != "{}\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestTheDirectoryIsMadeIfItIsNotThere(t *testing.T) {
	path := target(t)

	if err := Save(path, "state-*.json", []byte("{}")); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != dirMode {
		t.Fatalf("directory mode = %v, want %v", info.Mode().Perm(), dirMode)
	}
}

func TestTheFileIsWrittenForItsOwnerOnly(t *testing.T) {
	path := target(t)

	if err := Save(path, "state-*.json", []byte("{}")); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("file mode = %v, want %v", info.Mode().Perm(), fileMode)
	}
}

func TestSavingAgainReplacesWhatWasThere(t *testing.T) {
	path := target(t)
	if err := Save(path, "state-*.json", []byte("first")); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := Save(path, "state-*.json", []byte("second")); err != nil {
		t.Fatalf("save again: %v", err)
	}

	body, _ := os.ReadFile(path)
	if string(body) != "second" {
		t.Fatalf("body = %q, want the newer one", body)
	}
}

func TestNothingIsLeftBehindWhenItWorks(t *testing.T) {
	path := target(t)

	if err := Save(path, "state-*.json", []byte("{}")); err != nil {
		t.Fatalf("save: %v", err)
	}

	left, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("directory holds %d files, want only the target", len(left))
	}
}

func TestTheContainingDirectoryIsSyncedAfterReplacement(t *testing.T) {
	path := target(t)
	saver := New()
	steps := []string{}
	saver.rename = func(from, to string) error {
		steps = append(steps, "rename")
		return os.Rename(from, to)
	}
	saver.syncDir = func(dir string) error {
		steps = append(steps, "sync "+dir)
		return nil
	}

	if err := saver.Save(path, "state-*.json", []byte("{}")); err != nil {
		t.Fatalf("save: %v", err)
	}

	want := []string{"rename", "sync " + filepath.Dir(path)}
	if strings.Join(steps, "|") != strings.Join(want, "|") {
		t.Fatalf("steps = %q, want %q", steps, want)
	}
}

func TestADirectoryFlushFailureIsReportedAfterReplacement(t *testing.T) {
	path := target(t)
	saver := New()
	saver.syncDir = func(string) error {
		return errors.New("directory flush failed")
	}

	err := saver.Save(path, "state-*.json", []byte("new"))

	if err == nil || !strings.Contains(err.Error(), "directory flush failed") {
		t.Fatalf("error = %v, want the durability failure", err)
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read replacement: %v", readErr)
	}
	if string(body) != "new" {
		t.Fatalf("body = %q, want the completed replacement", body)
	}
}

func TestADirectoryThatCannotBeMadeIsReported(t *testing.T) {
	saver := New()
	saver.makeDir = func(string, os.FileMode) error {
		return errors.New("read-only file system")
	}

	err := saver.Save(target(t), "state-*.json", []byte("{}"))

	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error = %v, want what the filesystem said", err)
	}
}

func TestATemporaryFileThatCannotBeMadeIsReported(t *testing.T) {
	saver := New()
	saver.create = func(string, string) (*os.File, error) {
		return nil, errors.New("no space left on device")
	}

	err := saver.Save(target(t), "state-*.json", []byte("{}"))

	if err == nil || !strings.Contains(err.Error(), "no space") {
		t.Fatalf("error = %v", err)
	}
}

func TestAWriteThatFailsLeavesTheOldFileAndNoLitter(t *testing.T) {
	path := target(t)
	if err := Save(path, "state-*.json", []byte("first")); err != nil {
		t.Fatalf("save: %v", err)
	}
	saver := New()
	saver.write = func(*os.File, []byte) (int, error) {
		return 0, errors.New("input/output error")
	}

	err := saver.Save(path, "state-*.json", []byte("second"))

	if err == nil || !strings.Contains(err.Error(), "input/output") {
		t.Fatalf("error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "first" {
		t.Fatalf("body = %q, want the file that was already there", body)
	}
	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 1 {
		t.Fatalf("directory holds %d files, want the temporary one taken away", len(left))
	}
}

func TestAFlushThatFailsLeavesTheOldFileAndNoLitter(t *testing.T) {
	path := target(t)
	if err := Save(path, "state-*.json", []byte("first")); err != nil {
		t.Fatalf("save: %v", err)
	}
	saver := New()
	saver.sync = func(*os.File) error {
		return errors.New("disk went away")
	}

	err := saver.Save(path, "state-*.json", []byte("second"))

	if err == nil || !strings.Contains(err.Error(), "disk went away") {
		t.Fatalf("error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "first" {
		t.Fatalf("body = %q, want the file that was already there", body)
	}
	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 1 {
		t.Fatalf("directory holds %d files, want the temporary one taken away", len(left))
	}
}

func TestAModeThatCannotBeSetLeavesNoLitter(t *testing.T) {
	path := target(t)
	saver := New()
	saver.chmod = func(string, os.FileMode) error {
		return errors.New("operation not permitted")
	}

	err := saver.Save(path, "state-*.json", []byte("{}"))

	if err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("a file nobody could set the mode on was left at the target")
	}
	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 0 {
		t.Fatalf("directory holds %d files, want the temporary one taken away", len(left))
	}
}

func TestARenameThatFailsLeavesTheOldFileAndNoLitter(t *testing.T) {
	path := target(t)
	if err := Save(path, "state-*.json", []byte("first")); err != nil {
		t.Fatalf("save: %v", err)
	}
	saver := New()
	saver.rename = func(string, string) error {
		return errors.New("cross-device link")
	}

	err := saver.Save(path, "state-*.json", []byte("second"))

	if err == nil || !strings.Contains(err.Error(), "cross-device") {
		t.Fatalf("error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "first" {
		t.Fatalf("body = %q, want the file that was already there", body)
	}
	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 1 {
		t.Fatalf("directory holds %d files, want the temporary one taken away", len(left))
	}
}

func TestTheOriginalFailureSurvivesATidyUpThatFailsToo(t *testing.T) {
	saver := New()
	saver.rename = func(string, string) error {
		return errors.New("cross-device link")
	}
	saver.remove = func(string) error {
		return errors.New("could not remove it either")
	}

	err := saver.Save(target(t), "state-*.json", []byte("{}"))

	if err == nil || !strings.Contains(err.Error(), "cross-device") {
		t.Fatalf("error = %v, want the failure that mattered", err)
	}
}
