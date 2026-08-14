package protect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const remote = "https://10.0.0.5:6443"

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "protected.json")
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return store
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

	store, err := Open(path)

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

	_, err := Open(path)

	if err == nil {
		t.Fatal("a directory in place of the list read as an empty list")
	}
}

func TestAnAnswerThatCannotBeSavedIsNotKept(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store := &Store{path: filepath.Join(blocked, "spinoza", "protected.json"), clusters: map[string]bool{}}

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
	store := &Store{path: path, clusters: map[string]bool{}}

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
