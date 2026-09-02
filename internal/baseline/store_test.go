package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

func TestConcurrentClustersKeepTheirOwnBaselines(t *testing.T) {
	held := store(t)
	const clusters = 24
	var group sync.WaitGroup

	for index := range clusters {
		server := fmt.Sprintf("https://cluster-%02d.example:6443", index)
		baseline := taken()
		baseline.Cluster = server
		baseline.TakenAt = fmt.Sprintf("2026-09-01T%02d:00:00Z", index)
		group.Go(func() {
			if err := held.Save(server, baseline); err != nil {
				t.Errorf("save %s: %v", server, err)
			}
		})
	}
	group.Wait()

	for index := range clusters {
		server := fmt.Sprintf("https://cluster-%02d.example:6443", index)
		wantTakenAt := fmt.Sprintf("2026-09-01T%02d:00:00Z", index)
		baseline, ok := held.Load(server)
		if !ok {
			t.Fatalf("%s has no baseline", server)
		}
		if baseline.Cluster != server || baseline.TakenAt != wantTakenAt {
			t.Fatalf("%s loaded %+v", server, baseline)
		}
	}
}

func TestConcurrentReadsAndReplacementsNeverExposeAPartialBaseline(t *testing.T) {
	held := store(t)
	initial := taken()
	initial.Cluster = cluster
	if err := held.Save(cluster, initial); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	const workers = 24
	var group sync.WaitGroup
	for index := range workers {
		group.Go(func() {
			next := taken()
			next.Cluster = cluster
			next.TakenAt = fmt.Sprintf("2026-09-01T%02d:00:00Z", index)
			if err := held.Save(cluster, next); err != nil {
				t.Errorf("save %d: %v", index, err)
			}
		})
		group.Go(func() {
			loaded, ok := held.Load(cluster)
			if !ok {
				t.Errorf("read %d saw no baseline", index)
				return
			}
			if loaded.Cluster != cluster || len(loaded.Checks) != 1 || len(loaded.Keys) != 2 {
				t.Errorf("read %d saw a partial baseline: %+v", index, loaded)
			}
		})
	}
	group.Wait()
}

func TestStoredBaselineAndDirectoryArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baselines")
	held := Open(dir)

	if err := held.Save(cluster, taken()); err != nil {
		t.Fatalf("save: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want 0700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(held.fileFor(cluster))
	if err != nil {
		t.Fatalf("stat baseline: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("baseline mode = %v, want 0600", fileInfo.Mode().Perm())
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

func TestABaselineTooLargeToReadBackIsNotSaved(t *testing.T) {
	held := store(t)
	huge := taken()
	huge.Keys = map[string]string{"finding": strings.Repeat("x", maxBytes)}

	if err := held.Save(cluster, huge); err == nil {
		t.Fatal("a baseline past the byte cap was written anyway")
	}
	if _, ok := held.Load(cluster); ok {
		t.Fatal("a refused baseline was left behind")
	}
}

func TestAnOversizedBaselineFileIsNotLoaded(t *testing.T) {
	held := store(t)
	path := held.fileFor(cluster)
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Truncate(path, maxBytes+1); err != nil {
		t.Fatalf("enlarge: %v", err)
	}

	if _, ok := held.Load(cluster); ok {
		t.Fatal("an oversized baseline file was loaded")
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

func TestAnImportedBaselineCannotExceedTheFindingCap(t *testing.T) {
	huge := taken()
	huge.Keys = make(map[string]string, maxKeys+1)
	for at := range maxKeys + 1 {
		huge.Keys[strconv.Itoa(at)] = "Deployment apps/api"
	}
	body, err := json.Marshal(flatten(huge))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := Decode(body); err == nil {
		t.Fatal("a baseline past the finding cap was imported")
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
