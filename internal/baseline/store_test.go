package baseline

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/checks"
)

const cluster = "https://one.example:6443"

func taken() checks.Baseline {
	return checks.Baseline{
		TakenAt: "2026-08-30T00:00:00Z",
		Checks:  []string{"privileged-containers"},
		Counts:  map[string]int{"privileged-containers": 2},
		Keys:    map[string]bool{"aaaaaaaaaaa": true, "bbbbbbbbbbb": true},
	}
}

func store(t *testing.T) *Store {
	t.Helper()
	return Open(t.TempDir())
}

// what a baseline survives

func TestABaselineComesBackAsItWasSaved(t *testing.T) {
	held := store(t)

	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}
	back, ok := held.Load(cluster)

	if !ok {
		t.Fatal("a baseline that was just saved could not be read")
	}
	if back.TakenAt != taken().TakenAt || back.Counts["privileged-containers"] != 2 {
		t.Fatalf("read back %+v", back)
	}
	if !back.Keys["aaaaaaaaaaa"] || len(back.Keys) != 2 {
		t.Fatalf("read back %d keys: %v", len(back.Keys), back.Keys)
	}
}

func TestOneClusterCannotReadAnotherClusterBaseline(t *testing.T) {
	held := store(t)
	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, ok := held.Load("https://two.example:6443"); ok {
		t.Fatal("a second cluster was given the first one's baseline")
	}
}

func TestTakingANewBaselineReplacesTheOldOne(t *testing.T) {
	held := store(t)
	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}
	next := taken()
	next.TakenAt = "2026-09-01T00:00:00Z"

	if err := held.Save(cluster, next); err != nil {
		t.Fatalf("save again: %v", err)
	}

	back, _ := held.Load(cluster)
	if back.TakenAt != next.TakenAt {
		t.Fatalf("the older baseline survived: %q", back.TakenAt)
	}
}

func TestForgettingABaselineLeavesNothingToCompareAgainst(t *testing.T) {
	held := store(t)
	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := held.Clear(cluster); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if _, ok := held.Load(cluster); ok {
		t.Fatal("a forgotten baseline was still there")
	}
}

func TestForgettingABaselineThatWasNeverTakenIsNotAnError(t *testing.T) {
	if err := store(t).Clear(cluster); err != nil {
		t.Fatalf("clear: %v", err)
	}
}

// what a baseline refuses

func TestAClusterWithNoBaselineSaysSo(t *testing.T) {
	if _, ok := store(t).Load(cluster); ok {
		t.Fatal("a cluster with no baseline was given one")
	}
}

func TestAFileThatIsNotABaselineIsIgnoredRatherThanObeyed(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}
	name, err := os.ReadDir(dir)
	if err != nil || len(name) != 1 {
		t.Fatalf("the store wrote %v (%v)", name, err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, name[0].Name()), []byte("{{{"), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	if _, ok := held.Load(cluster); ok {
		t.Fatal("an unreadable file was read as a baseline")
	}
}

func TestMoreFindingsThanOneBaselineHoldsIsRefused(t *testing.T) {
	huge := taken()
	huge.Keys = make(map[string]bool, maxKeys+1)
	for at := range maxKeys + 1 {
		huge.Keys[strconv.Itoa(at)] = true
	}

	if err := store(t).Save(cluster, huge); err == nil {
		t.Fatal("a baseline past the cap was written anyway")
	}
}

// what a store with nowhere to write does

func TestAStoreWithNoDirectoryKeepsNothingAndSaysNothing(t *testing.T) {
	held := Open("")

	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := held.Clear(cluster); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := held.Load(cluster); ok {
		t.Fatal("a store with nowhere to write produced a baseline")
	}
}

func TestADirectoryThatCannotBeMadeIsReported(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Open(filepath.Join(blocked, "baselines")).Save(cluster, taken()); err == nil {
		t.Fatal("saving under a file succeeded")
	}
}

func TestADirectoryThatCannotBeWrittenToIsReported(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baselines")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := Open(dir).Save(cluster, taken()); err == nil {
		t.Fatal("saving into a directory that refuses writes succeeded")
	}
}

func TestABaselineThatCannotBeRemovedIsReported(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	if err := os.MkdirAll(held.fileFor(cluster), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(held.fileFor(cluster), "in-the-way"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := held.Clear(cluster); err == nil {
		t.Fatal("clearing something that cannot be removed succeeded")
	}
}

func TestNoHomeMeansNoDefaultDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if _, err := DefaultDir(); err == nil {
		t.Fatal("a machine with no config directory named one anyway")
	}
}

func TestTheDefaultDirectorySitsBesideTheSettings(t *testing.T) {
	dir, err := DefaultDir()
	if err != nil {
		t.Skipf("no user config directory here: %v", err)
	}
	if filepath.Base(dir) != "baselines" {
		t.Fatalf("the default directory is %q", dir)
	}
}
