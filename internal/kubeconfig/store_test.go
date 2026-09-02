package kubeconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/filetx"
)

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "kubeconfigs.json")
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return store
}

func holdTransaction(t *testing.T, path string) func() {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- filetx.Exclusive(t.Context(), path, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("hold transaction: %v", err)
	}
	return func() {
		close(release)
		if err := <-done; err != nil {
			t.Fatalf("release transaction: %v", err)
		}
	}
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

func TestConcurrentAddsAreAllPersisted(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	const additions = 32
	var group sync.WaitGroup

	for index := range additions {
		entry := fmt.Sprintf("/tmp/cluster-%02d.yaml", index)
		group.Go(func() {
			if err := store.Add(entry); err != nil {
				t.Errorf("add %s: %v", entry, err)
			}
		})
	}
	group.Wait()

	want := make([]string, additions)
	for index := range additions {
		want[index] = fmt.Sprintf("/tmp/cluster-%02d.yaml", index)
	}
	slices.Sort(want)
	held := store.Paths()
	slices.Sort(held)
	if !slices.Equal(held, want) {
		t.Fatalf("paths = %v, want every concurrent addition", held)
	}
	persisted := openStore(t, path).Paths()
	slices.Sort(persisted)
	if !slices.Equal(persisted, want) {
		t.Fatalf("persisted paths = %v, want every concurrent addition", persisted)
	}
}

func TestSimultaneousProcessesDoNotLoseKubeconfigs(t *testing.T) {
	path := storePath(t)
	first := openStore(t, path)
	second := openStore(t, path)
	unlock := holdTransaction(t, path)
	done := make(chan error, 2)
	go func() {
		done <- first.Add("/tmp/one.yaml")
	}()
	go func() {
		done <- second.Add("/tmp/two.yaml")
	}()
	select {
	case err := <-done:
		unlock()
		t.Fatalf("a write escaped the transaction lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	paths := openStore(t, path).Paths()
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"/tmp/one.yaml", "/tmp/two.yaml"}) {
		t.Fatalf("paths = %v, want both processes' kubeconfigs", paths)
	}
}

func TestPathsPickUpAnotherProcessesAddition(t *testing.T) {
	path := storePath(t)
	first := openStore(t, path)
	second := openStore(t, path)
	if err := second.Add("/tmp/other.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if !slices.Equal(first.Paths(), []string{"/tmp/other.yaml"}) {
		t.Fatalf("paths = %v, want the other process's addition", first.Paths())
	}
}

func TestConcurrentDuplicateAddsStoreOneEntry(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	const callers = 16
	results := make(chan error, callers)
	var group sync.WaitGroup

	for range callers {
		group.Go(func() {
			results <- store.Add("/tmp/one.yaml")
		})
	}
	group.Wait()
	close(results)

	added := 0
	for err := range results {
		if err == nil {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("successful adds = %d, want one", added)
	}
	if !slices.Equal(store.Paths(), []string{"/tmp/one.yaml"}) {
		t.Fatalf("paths = %v, want one entry", store.Paths())
	}
	if !slices.Equal(openStore(t, path).Paths(), []string{"/tmp/one.yaml"}) {
		t.Fatal("the single entry was not persisted")
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

	store, err := Open(t.Context(), path)

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

func TestAddDoesNotReplaceAMalformedListFromStaleMemory(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Add("/tmp/held.yaml"); err != nil {
		t.Fatalf("initial add: %v", err)
	}
	broken := []byte("{not json")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("break list: %v", err)
	}

	err := store.Add("/tmp/new.yaml")

	if err == nil {
		t.Fatal("an add over a malformed list reported success")
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read broken list: %v", readErr)
	}
	if !bytes.Equal(body, broken) {
		t.Fatalf("malformed list was replaced with %q", body)
	}
	if !slices.Equal(store.Paths(), []string{"/tmp/held.yaml"}) {
		t.Fatalf("paths = %v, want the last readable list", store.Paths())
	}
}

func TestAListLargerThanTheReadLimitIsNotWritten(t *testing.T) {
	store := openStore(t, storePath(t))

	err := store.write([]string{strings.Repeat("x", maxFileBytes)})

	if err == nil {
		t.Fatal("an oversized kubeconfig list was written")
	}
}

func TestAnUnreadableListSurfaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfigs.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := Open(t.Context(), path)

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
	store := &Store{ctx: t.Context(), path: filepath.Join(blocked, "spinoza", "kubeconfigs.json")}

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
	store := &Store{ctx: t.Context(), path: path}

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

	store, err := Open(t.Context(), path)

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

	store, err := Open(t.Context(), path)

	if err == nil {
		t.Fatal("a list that is not json opened without complaint")
	}
	if len(store.Paths()) != 0 {
		t.Fatalf("paths = %v, want nothing kept from a file that could not be read", store.Paths())
	}
}

func TestAKubeconfigThatCannotBeSavedIsNotRemembered(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(lockedDir(t), "nested", "kubeconfigs.json"))
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
	store, err := Open(t.Context(), path)
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
