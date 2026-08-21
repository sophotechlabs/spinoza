package kubeconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "kubeconfigs.json")
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return store
}

func TestAnAbsentListStartsEmpty(t *testing.T) {
	store := openStore(t, storePath(t))

	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want none before anything is added", store.Paths())
	}
}

func TestAnAddedKubeconfigSurvivesARestart(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)

	err := store.Add("/tmp/one.yaml")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	reopened := openStore(t, path)
	if !slices.Equal(reopened.Paths(), []string{"/tmp/one.yaml"}) {
		t.Fatalf("paths = %v, want the added kubeconfig read back", reopened.Paths())
	}
}

func TestTheListKeepsTheOrderKubeconfigsWereAdded(t *testing.T) {
	store := openStore(t, storePath(t))

	for _, path := range []string{"/tmp/one.yaml", "/tmp/two.yaml", "/tmp/three.yaml"} {
		if err := store.Add(path); err != nil {
			t.Fatalf("add %s: %v", path, err)
		}
	}

	want := []string{"/tmp/one.yaml", "/tmp/two.yaml", "/tmp/three.yaml"}
	if !slices.Equal(store.Paths(), want) {
		t.Fatalf("paths = %v, want %v", store.Paths(), want)
	}
}

func TestTheSameKubeconfigIsNotAddedTwice(t *testing.T) {
	store := openStore(t, storePath(t))
	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := store.Add("/tmp/one.yaml")

	if err == nil {
		t.Fatal("the same kubeconfig was added twice")
	}
	if len(store.Paths()) != 1 {
		t.Fatalf("paths = %v, want the one entry", store.Paths())
	}
}

func TestRemovingDropsTheKubeconfigFromDisk(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Add("/tmp/two.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := store.Remove("/tmp/one.yaml")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	if !slices.Equal(openStore(t, path).Paths(), []string{"/tmp/two.yaml"}) {
		t.Fatalf("paths = %v, want only the kept kubeconfig", openStore(t, path).Paths())
	}
}

func TestRemovingSomethingThatWasNeverAddedSaysSo(t *testing.T) {
	store := openStore(t, storePath(t))

	err := store.Remove("/tmp/one.yaml")

	if err == nil {
		t.Fatal("removing an unknown kubeconfig reported success")
	}
}

func TestTheListIsWrittenForTheOwnerOnly(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)

	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600; the file names files holding cluster credentials", info.Mode().Perm())
	}
}

func TestTheListIsReadableJSON(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var saved state
	if unmarshalErr := json.Unmarshal(body, &saved); unmarshalErr != nil {
		t.Fatalf("unmarshal %s: %v", body, unmarshalErr)
	}
	if !slices.Equal(saved.Kubeconfigs, []string{"/tmp/one.yaml"}) {
		t.Fatalf("saved = %v", saved.Kubeconfigs)
	}
}

func TestPathsHandsOutACopy(t *testing.T) {
	store := openStore(t, storePath(t))
	if err := store.Add("/tmp/one.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	store.Paths()[0] = "/tmp/rewritten.yaml"

	if store.Paths()[0] != "/tmp/one.yaml" {
		t.Fatalf("paths = %v, want the store's own list left alone", store.Paths())
	}
}

func TestABrokenListSurfacesAndStartsEmpty(t *testing.T) {
	path := storePath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := Open(path)

	if err == nil {
		t.Fatal("an unreadable list was reported as an empty one")
	}
	if store == nil {
		t.Fatal("spinoza refused to start over a broken kubeconfig list")
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v", store.Paths())
	}
}

func TestAnUnreadableListSurfaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfigs.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := Open(path)

	if err == nil {
		t.Fatal("a directory in place of the list read as an empty list")
	}
}

func TestWithNoPathTheListLivesForThisRunOnly(t *testing.T) {
	store := openStore(t, "")

	err := store.Add("/tmp/one.yaml")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if !slices.Equal(store.Paths(), []string{"/tmp/one.yaml"}) {
		t.Fatalf("paths = %v, want the kubeconfig held in memory", store.Paths())
	}
}

func TestAListThatCannotBeWrittenLeavesTheStoreAlone(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store := &Store{path: filepath.Join(blocked, "spinoza", "kubeconfigs.json")}

	err := store.Add("/tmp/one.yaml")

	if err == nil {
		t.Fatal("a list that could not be written reported success")
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want nothing remembered when it could not be saved", store.Paths())
	}
}

func TestAListThatCannotBeReplacedLeavesTheStoreAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfigs.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{path: path}

	err := store.Add("/tmp/one.yaml")

	if err == nil {
		t.Fatal("a list that could not replace its file reported success")
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "kubeconfigs-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want the half-written file cleaned up", leftovers)
	}
}

func TestDefaultPathSitsUnderTheUserConfigDirectory(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	if !strings.HasSuffix(path, filepath.Join("spinoza", "kubeconfigs.json")) {
		t.Fatalf("path = %q, want it under a spinoza directory", path)
	}
}

func lockedDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	return dir
}

func TestAListThatCannotBeReadIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfigs.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	store, err := Open(path)

	if err == nil {
		t.Fatal("a list that could not be read opened without complaint")
	}
	if store == nil {
		t.Fatal("a store that could not be read must still be usable")
	}
}

func TestAListThatIsNotJsonIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfigs.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := Open(path)

	if err == nil {
		t.Fatal("a list that is not json opened without complaint")
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want nothing kept from a file that could not be read", store.Paths())
	}
}

func TestAKubeconfigThatCannotBeSavedIsNotRemembered(t *testing.T) {
	store, err := Open(filepath.Join(lockedDir(t), "nested", "kubeconfigs.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	addErr := store.Add("/home/arch/.kube/other")

	if addErr == nil {
		t.Fatal("a list that could not be written was reported as saved")
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want the list unchanged", store.Paths())
	}
}

func TestARemovalThatCannotBeSavedLeavesTheListAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfigs.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if addErr := store.Add("/home/arch/.kube/other"); addErr != nil {
		t.Fatalf("add: %v", addErr)
	}
	if chmodErr := os.Chmod(dir, 0o500); chmodErr != nil {
		t.Fatalf("chmod: %v", chmodErr)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})

	removeErr := store.Remove("/home/arch/.kube/other")

	if removeErr == nil {
		t.Fatal("a removal that could not be written was reported as saved")
	}
	if len(store.Paths()) != 1 {
		t.Fatalf("paths = %v, want the kubeconfig still on the list", store.Paths())
	}
}
