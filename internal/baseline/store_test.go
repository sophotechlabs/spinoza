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
		Keys: map[string]string{
			"privileged-containers\x00aaaaaaaaaaa": "Deployment apps/api",
			"privileged-containers\x00bbbbbbbbbbb": "Deployment apps/web",
		},
	}
}

func store(t *testing.T) *Store {
	t.Helper()
	return Open(t.TempDir())
}

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
	if back.Keys["privileged-containers\x00aaaaaaaaaaa"] != "Deployment apps/api" || len(back.Keys) != 2 {
		t.Fatalf("read back %d keys: %v", len(back.Keys), back.Keys)
	}
}

func TestAnOlderStoredBaselineWithNoFingerprintsStillLoads(t *testing.T) {
	held := store(t)
	body := []byte(`{"takenAt":"2026-08-30T00:00:00Z","checks":["privileged-containers"]}`)
	if err := os.WriteFile(held.fileFor(cluster), body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	back, ok := held.Load(cluster)

	if !ok {
		t.Fatal("an older baseline without fingerprints could not be read")
	}
	if back.Keys == nil {
		t.Fatal("an older baseline came back with a nil set to look fingerprints up in")
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
	huge.Keys = make(map[string]string, maxKeys+1)
	for at := range maxKeys + 1 {
		huge.Keys[strconv.Itoa(at)] = "Deployment apps/api"
	}

	if err := store(t).Save(cluster, huge); err == nil {
		t.Fatal("a baseline past the cap was written anyway")
	}
}

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

func TestABaselineComesBackFromTheFileItWasWrittenTo(t *testing.T) {
	body, err := Encode(taken())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	back, decodeErr := Decode(body)

	if decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if back.TakenAt != taken().TakenAt || len(back.Keys) != 2 {
		t.Fatalf("read back %+v", back)
	}
}

func TestABaselineRemembersWhichClusterItWasTakenOn(t *testing.T) {
	mine := taken()
	mine.Cluster = cluster
	body, err := Encode(mine)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	back, decodeErr := Decode(body)

	if decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if back.Cluster != cluster {
		t.Fatalf("the baseline came back naming %q", back.Cluster)
	}
}

func TestSomethingThatIsNotABaselineIsRefusedRatherThanStored(t *testing.T) {
	cases := []struct{ name, body string }{
		{"not json", "{{{"},
		{"an empty object", "{}"},
		{"a baseline with no checks", `{"takenAt":"2026-08-30T00:00:00Z","checks":[]}`},
		{"a baseline with no day", `{"checks":["a"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode([]byte(tc.body)); err == nil {
				t.Fatal("it was read as a baseline")
			}
		})
	}
}

func TestABaselineTooLargeToBeOneIsRefused(t *testing.T) {
	if _, err := Decode(make([]byte, maxBytes+1)); err == nil {
		t.Fatal("a body past the cap was read as a baseline")
	}
}

func TestABaselineWithNoFingerprintsStillReads(t *testing.T) {
	back, err := Decode([]byte(`{"takenAt":"2026-08-30T00:00:00Z","checks":["a"]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.Keys == nil {
		t.Fatal("a baseline with no fingerprints came back with a nil set to look them up in")
	}
}
