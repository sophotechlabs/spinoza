package protect

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const remote = "https://10.0.0.5:6443"

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "protected.json")
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

func TestAClusterNobodyDecidedOnIsUnknown(t *testing.T) {
	store := openStore(t, storePath(t))

	if store.Verdict(remote) != api.ProtectionUnknown {
		t.Fatalf("verdict = %q, want unknown so the app can ask", store.Verdict(remote))
	}
}

func TestALocalClusterIsLeftOpenWithoutAsking(t *testing.T) {
	store := openStore(t, storePath(t))

	for _, server := range []string{"https://127.0.0.1:6443", "https://localhost:6443", "https://[::1]:6443"} {
		if store.Verdict(server) != api.ProtectionOpen {
			t.Fatalf("verdict(%s) = %q, want open; kind and docker-desktop are not worth a prompt", server, store.Verdict(server))
		}
	}
}

func TestAPrivateAddressIsNotTreatedAsLocal(t *testing.T) {
	store := openStore(t, storePath(t))

	if store.Verdict("https://10.10.0.1:6443") != api.ProtectionUnknown {
		t.Fatal("a cluster on a private address was assumed local; a vpn-reachable production cluster looks exactly like this")
	}
}

func TestProtectingAClusterSticks(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)

	err := store.Set(remote, true)
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q", store.Verdict(remote))
	}
	if openStore(t, path).Verdict(remote) != api.ProtectionProtected {
		t.Fatal("the answer did not survive a restart")
	}
}

func TestAnsweringNoIsRememberedToo(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)

	err := store.Set(remote, false)
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if openStore(t, path).Verdict(remote) != api.ProtectionOpen {
		t.Fatal("saying no was forgotten, so the prompt would come back every time")
	}
}

func TestUnprotectingAClusterAgain(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := store.Set(remote, false); err != nil {
		t.Fatalf("unset: %v", err)
	}

	if openStore(t, path).Verdict(remote) != api.ProtectionOpen {
		t.Fatal("the cluster stayed protected after it was opened up")
	}
}

func TestEachClusterIsAnsweredSeparately(t *testing.T) {
	store := openStore(t, storePath(t))
	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if store.Verdict("https://10.0.0.6:6443") != api.ProtectionUnknown {
		t.Fatal("protecting one cluster answered for another")
	}
}

func TestWithoutAClusterThereIsNothingToSay(t *testing.T) {
	store := openStore(t, storePath(t))

	if store.Verdict("") != api.ProtectionUnknown {
		t.Fatalf("verdict = %q", store.Verdict(""))
	}
	if store.Set("", true) == nil {
		t.Fatal("protecting nothing reported success")
	}
}

func TestTheListIsWrittenForTheOwnerOnly(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestTheListIsReadableJSON(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var saved state
	if unmarshalErr := json.Unmarshal(body, &saved); unmarshalErr != nil {
		t.Fatalf("unmarshal %s: %v", body, unmarshalErr)
	}
	if !saved.Clusters[remote] {
		t.Fatalf("saved = %v", saved.Clusters)
	}
}

func TestABrokenListSurfacesAndProtectsNothing(t *testing.T) {
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
		t.Fatal("spinoza refused to start over a broken protected-cluster list")
	}
	if store.Verdict(remote) != api.ProtectionUnknown {
		t.Fatalf("verdict = %q", store.Verdict(remote))
	}
}

func TestAnUnreadableListSurfaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := Open(t.Context(), path)

	if err == nil {
		t.Fatal("a directory in place of the list read as an empty list")
	}
}

func TestAnAnswerThatCannotBeSavedIsNotKept(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store := &Store{ctx: t.Context(), path: filepath.Join(blocked, "spinoza", "protected.json"), clusters: map[string]bool{}}

	err := store.Set(remote, true)

	if err == nil {
		t.Fatal("an answer that could not be written reported success")
	}
	if store.Verdict(remote) != api.ProtectionUnknown {
		t.Fatal("the answer was kept in memory after it failed to save, so a restart would forget it silently")
	}
}

func TestAnAnswerThatCannotReplaceTheFileIsNotKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "protected.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &Store{ctx: t.Context(), path: path, clusters: map[string]bool{}}

	err := store.Set(remote, true)

	if err == nil {
		t.Fatal("an answer that could not replace its file reported success")
	}
	leftovers, globErr := filepath.Glob(filepath.Join(dir, "protected-*.json"))
	if globErr != nil {
		t.Fatalf("glob: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftovers = %v, want the half-written file cleaned up", leftovers)
	}
}

func TestWithNoPathTheAnswersLastForThisRunOnly(t *testing.T) {
	store := openStore(t, "")

	if err := store.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q", store.Verdict(remote))
	}
}

func TestAServerThatIsNotAURLIsNotLocal(t *testing.T) {
	if Local("://nonsense") {
		t.Fatal("an unparseable server was treated as local")
	}
	if Local("https://kubernetes.default.svc") {
		t.Fatal("a named host that is not localhost was treated as local")
	}
}

func TestDefaultPathSitsUnderTheUserConfigDirectory(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	want := filepath.Join("spinoza", "protected.json")
	if filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)) != want {
		t.Fatalf("path = %q, want it under a spinoza directory", path)
	}
}

func TestManyClustersCanBeAnswered(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)

	for i := range 5 {
		if err := store.Set("https://10.0.0."+strconv.Itoa(i)+":6443", i%2 == 0); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	reopened := openStore(t, path)
	if reopened.Verdict("https://10.0.0.0:6443") != api.ProtectionProtected {
		t.Fatal("the first answer was lost")
	}
	if reopened.Verdict("https://10.0.0.1:6443") != api.ProtectionOpen {
		t.Fatal("a later answer overwrote an earlier one")
	}
}

func TestConcurrentProtectionDecisionsAreAllPersisted(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	const clusters = 32
	var group sync.WaitGroup

	for index := range clusters {
		server := "https://10.0.1." + strconv.Itoa(index) + ":6443"
		protected := index%2 == 0
		group.Go(func() {
			if err := store.Set(server, protected); err != nil {
				t.Errorf("set %s: %v", server, err)
			}
		})
	}
	group.Wait()

	reopened := openStore(t, path)
	for index := range clusters {
		server := "https://10.0.1." + strconv.Itoa(index) + ":6443"
		want := api.ProtectionOpen
		if index%2 == 0 {
			want = api.ProtectionProtected
		}
		if got := reopened.Verdict(server); got != want {
			t.Fatalf("%s verdict = %q, want %q", server, got, want)
		}
	}
}

func TestSimultaneousProcessesDoNotLoseProtectionDecisions(t *testing.T) {
	path := storePath(t)
	first := openStore(t, path)
	second := openStore(t, path)
	unlock := holdTransaction(t, path)
	done := make(chan error, 2)
	go func() {
		done <- first.Set("https://10.0.2.1:6443", true)
	}()
	go func() {
		done <- second.Set("https://10.0.2.2:6443", false)
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
			t.Fatalf("set: %v", err)
		}
	}

	reopened := openStore(t, path)
	if reopened.Verdict("https://10.0.2.1:6443") != api.ProtectionProtected {
		t.Fatal("the first process's protection decision was lost")
	}
	if reopened.Verdict("https://10.0.2.2:6443") != api.ProtectionOpen {
		t.Fatal("the second process's protection decision was lost")
	}
}

func TestAVerdictPicksUpAnotherProcessesDecision(t *testing.T) {
	path := storePath(t)
	first := openStore(t, path)
	second := openStore(t, path)
	if err := second.Set(remote, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if first.Verdict(remote) != api.ProtectionProtected {
		t.Fatal("the first process retained its stale protection verdict")
	}
}

func readOnlyDir(t *testing.T) string {
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

func TestAFileThatCannotBeReadIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := Open(t.Context(), path)

	if err == nil {
		t.Fatal("a file that could not be read opened without complaint")
	}
}

func TestAFileThatIsNotJsonIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := Open(t.Context(), path)

	if err == nil {
		t.Fatal("a file that is not json opened without complaint")
	}
	if store == nil {
		t.Fatal("a store that could not be read must still be usable")
	}
}

func TestSetDoesNotReplaceMalformedDecisionsFromStaleMemory(t *testing.T) {
	path := storePath(t)
	store := openStore(t, path)
	if err := store.Set(remote, true); err != nil {
		t.Fatalf("initial set: %v", err)
	}
	broken := []byte("{not json")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("break decisions: %v", err)
	}

	err := store.Set("https://10.0.0.6:6443", true)

	if err == nil {
		t.Fatal("a write over malformed decisions reported success")
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read broken decisions: %v", readErr)
	}
	if !bytes.Equal(body, broken) {
		t.Fatalf("malformed decisions were replaced with %q", body)
	}
	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatal("the last readable decision was discarded")
	}
}

func TestDecisionsLargerThanTheReadLimitAreNotWritten(t *testing.T) {
	store := openStore(t, storePath(t))

	err := store.write(map[string]bool{strings.Repeat("x", maxFileBytes): true})

	if err == nil {
		t.Fatal("oversized protection decisions were written")
	}
}

func TestAnAnswerThatCannotBeSavedIsReported(t *testing.T) {
	store := openStore(t, filepath.Join(readOnlyDir(t), "nested", "protected.json"))

	err := store.Set(remote, true)

	if err == nil {
		t.Fatal("an answer that could not be written was reported as saved")
	}
	if store.Verdict(remote) != api.ProtectionUnknown {
		t.Fatal("an answer that was not written was remembered anyway")
	}
}

func TestAStoreWithNowhereToWriteKeepsTheAnswerInMemory(t *testing.T) {
	store, err := Open(t.Context(), "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if setErr := store.Set(remote, true); setErr != nil {
		t.Fatalf("set: %v", setErr)
	}

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q", store.Verdict(remote))
	}
}

func writeRaw(t *testing.T, path string, clusters map[string]bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, err := json.Marshal(map[string]any{"clusters": clusters})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(path, body, 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
}

func readRaw(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var saved struct {
		Clusters map[string]bool `json:"clusters"`
	}
	if unmarshalErr := json.Unmarshal(body, &saved); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	return saved.Clusters
}

func TestAnAnswerWrittenBeforeTheKeyWasNormalisedIsStillFound(t *testing.T) {
	path := storePath(t)
	writeRaw(t, path, map[string]bool{"https://10.0.0.5:6443/": true})

	store := openStore(t, path)

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q, want protected; an older file must not silently lose its answer", store.Verdict(remote))
	}
}

func TestTheSameClusterSpeltTwoWaysIsOneAnswer(t *testing.T) {
	store := openStore(t, storePath(t))
	if err := store.Set("HTTPS://10.0.0.5:6443/", true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatal("a trailing slash and a capital letter made a second cluster out of one")
	}
}

func TestWritingAnAnswerLeavesOnlyTheNormalisedKey(t *testing.T) {
	path := storePath(t)
	stale := "https://10.0.0.5:6443/"
	writeRaw(t, path, map[string]bool{stale: true})
	store := openStore(t, path)

	if err := store.Set(stale, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	held := readRaw(t, path)
	if _, found := held[stale]; found {
		t.Fatalf("the old key survived the write: %v", held)
	}
	if !held[remote] {
		t.Fatalf("the normalised key was not written: %v", held)
	}
}

func TestAClusterBehindARancherPathIsItsOwnAnswer(t *testing.T) {
	store := openStore(t, storePath(t))
	one := "https://rancher.example.com/k8s/clusters/c-m-aaaaaaaa"
	two := "https://rancher.example.com/k8s/clusters/c-m-bbbbbbbb"
	if err := store.Set(one, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	if store.Verdict(two) != api.ProtectionUnknown {
		t.Fatal("protecting one Rancher cluster answered for another on the same host")
	}
}

func TestTwoSpellingsOfOneClusterResolveToProtected(t *testing.T) {
	path := storePath(t)
	writeRaw(t, path, map[string]bool{
		"https://10.0.0.5:6443/": true,
		"HTTPS://10.0.0.5:6443":  false,
	})

	store := openStore(t, path)

	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatalf("verdict = %q, want protected; a conflict must never quietly open a cluster", store.Verdict(remote))
	}
}

func TestAnEmptyKeyInTheFileIsIgnored(t *testing.T) {
	path := storePath(t)
	writeRaw(t, path, map[string]bool{"": true, remote: true})

	store := openStore(t, path)

	if store.Verdict("") != api.ProtectionUnknown {
		t.Fatalf("verdict = %q, want unknown", store.Verdict(""))
	}
	if store.Verdict(remote) != api.ProtectionProtected {
		t.Fatal("a junk key stopped the real ones being read")
	}
}

func TestProtectedWinsWhateverOrderTheKeysAreRead(t *testing.T) {
	protectedFirst := map[string]bool{}
	adopt(protectedFirst, map[string]bool{"https://10.0.0.5:6443/": true})
	adopt(protectedFirst, map[string]bool{"HTTPS://10.0.0.5:6443": false})

	openFirst := map[string]bool{}
	adopt(openFirst, map[string]bool{"HTTPS://10.0.0.5:6443": false})
	adopt(openFirst, map[string]bool{"https://10.0.0.5:6443/": true})

	if !protectedFirst[remote] {
		t.Fatal("reading the protected spelling first left the cluster open")
	}
	if !openFirst[remote] {
		t.Fatal("reading the open spelling first left the cluster open")
	}
}
