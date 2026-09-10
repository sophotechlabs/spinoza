package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func header(id string) Header {
	return Header{
		ID:     id,
		At:     "2026-09-09T12:00:00Z",
		Actor:  "alice@example.com",
		Target: "prod/web",
		Kind:   "exec",
	}
}

func TestAStoreWithNoDirectoryRecordsNothing(t *testing.T) {
	held := Open("")

	if held.On() {
		t.Fatal("a store with no directory said it was recording")
	}
	if _, err := held.Start(header("one")); err == nil {
		t.Fatal("a store with no directory started a session")
	}
	if _, err := held.List(0); err == nil {
		t.Fatal("a store with no directory listed sessions")
	}
}

func TestASessionKeepsWhatWasTypedAndWhatWasShown(t *testing.T) {
	held := Open(t.TempDir())

	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Typed([]byte("ls -l\n"))
	session.Shown([]byte("total 0\n"))
	session.Close()

	read, text, textErr := held.Text("one")
	if textErr != nil {
		t.Fatalf("text: %v", textErr)
	}
	if text != "ls -l\ntotal 0\n" {
		t.Fatalf("text = %q", text)
	}
	if read.Actor != "alice@example.com" || read.Target != "prod/web" || read.Kind != "exec" {
		t.Fatalf("header = %+v", read)
	}
	if read.Bytes != len(text) {
		t.Fatalf("bytes = %d, want %d", read.Bytes, len(text))
	}
}

func TestASessionStopsAtTheSizeThisDeploymentKeeps(t *testing.T) {
	held := Open(t.TempDir())
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	session.Shown(make([]byte, maxBytes+1024))
	session.Shown([]byte("this should not be kept"))
	session.Close()

	_, text, textErr := held.Text("one")
	if textErr != nil {
		t.Fatalf("text: %v", textErr)
	}
	if !strings.HasSuffix(text, truncated) {
		t.Fatal("the transcript did not say it stopped recording")
	}
	if strings.Contains(text, "this should not be kept") {
		t.Fatal("the transcript kept writing past its size")
	}
}

func TestSessionsAreListedNewestFirst(t *testing.T) {
	held := Open(t.TempDir())
	for id, at := range map[string]string{
		"older": "2026-09-08T12:00:00Z",
		"newer": "2026-09-09T12:00:00Z",
	} {
		one := header(id)
		one.At = at
		session, err := held.Start(one)
		if err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		session.Close()
	}

	listed, err := held.List(0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d sessions", len(listed))
	}
	if listed[0].ID != "newer" {
		t.Fatalf("first listed = %s, want newer", listed[0].ID)
	}
}

func TestAnIdThatCouldLeaveTheDirectoryIsRefused(t *testing.T) {
	held := Open(t.TempDir())

	for _, bad := range []string{"", "../escape", "a/b", `a\b`, "with.dot"} {
		if _, err := held.Start(header(bad)); err == nil {
			t.Fatalf("id %q was accepted", bad)
		}
		if _, _, err := held.Text(bad); err == nil {
			t.Fatalf("id %q was read", bad)
		}
	}
}

func TestAnUnknownSessionIsNamedRatherThanEmpty(t *testing.T) {
	held := Open(t.TempDir())

	if _, _, err := held.Text("missing"); err == nil {
		t.Fatal("an unknown session read back without an error")
	}
}

func TestTrimRemovesWhatIsPastTheWindowAndKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for id, at := range map[string]time.Time{
		"old":    now.Add(-48 * time.Hour),
		"recent": now.Add(-time.Hour),
	} {
		one := header(id)
		one.At = at.Format(time.RFC3339)
		session, err := held.Start(one)
		if err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		session.Close()
	}

	removed, err := held.Trim(24*time.Hour, now)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d, want 1", removed)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "recent"+extension)); statErr != nil {
		t.Fatal("the recent session was removed too")
	}
}

func TestTrimWithNoWindowRemovesNothing(t *testing.T) {
	held := Open(t.TempDir())
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()

	removed, trimErr := held.Trim(0, time.Now())
	if trimErr != nil {
		t.Fatalf("trim: %v", trimErr)
	}
	if removed != 0 {
		t.Fatalf("removed %d with no window", removed)
	}
}

func TestTheHeaderIsStillReadableJSON(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Shown([]byte("hello"))
	session.Close()

	body, readErr := os.ReadFile(filepath.Join(dir, "one"+extension))
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(body) < headerWidth {
		t.Fatalf("the file is %d bytes, shorter than one header", len(body))
	}
	var read Header
	line := strings.TrimRight(string(body[:headerWidth]), " \n")
	if unmarshalErr := json.Unmarshal([]byte(line), &read); unmarshalErr != nil {
		t.Fatalf("the header did not parse: %v (%s)", unmarshalErr, line)
	}
	if read.Bytes != 5 {
		t.Fatalf("bytes = %d, want 5", read.Bytes)
	}
}

func TestANilSessionIsSafeToUse(t *testing.T) {
	var session *Session

	session.Typed([]byte("nothing"))
	session.Shown([]byte("nothing"))
	session.Close()
}

func TestALongTargetOrActorStillFitsInTheHeader(t *testing.T) {
	held := Open(t.TempDir())
	one := header("one")
	one.Target = strings.Repeat("a", 253) + "/" + strings.Repeat("b", 253)
	one.Actor = strings.Repeat("c", 400)

	session, err := held.Start(one)
	if err != nil {
		t.Fatalf("a long target stopped the session being recorded: %v", err)
	}
	session.Shown([]byte("hello"))
	session.Close()

	read, text, textErr := held.Text("one")
	if textErr != nil {
		t.Fatalf("text: %v", textErr)
	}
	if text != "hello" {
		t.Fatalf("text = %q", text)
	}
	if len(read.Target) > fieldWidth || len(read.Actor) > fieldWidth {
		t.Fatalf("header fields were not shortened: target %d, actor %d", len(read.Target), len(read.Actor))
	}
	if read.Bytes != 5 {
		t.Fatalf("bytes = %d, want 5", read.Bytes)
	}
}

func TestTheDefaultDirectorySitsBesideTheOtherStateSpinozaKeeps(t *testing.T) {
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("default dir: %v", err)
	}
	config, configErr := os.UserConfigDir()
	if configErr != nil {
		t.Skipf("this machine has no config directory: %v", configErr)
	}
	if dir != filepath.Join(config, "spinoza", "sessions") {
		t.Fatalf("dir = %q, want it under %q", dir, config)
	}
}

func TestASessionCannotBeStartedWhereItCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	held := Open(blocked)

	if _, err := held.Start(header("one")); err == nil {
		t.Fatal("a session started inside a file")
	}
	if _, err := held.List(0); err == nil {
		t.Fatal("a listing read a file as a directory")
	}
}

func TestADirectoryThatIsNotThereYetListsNothing(t *testing.T) {
	held := Open(filepath.Join(t.TempDir(), "not-yet"))

	listed, err := held.List(0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed %d sessions from a directory that is not there", len(listed))
	}
}

func TestWhatIsNotASessionIsLeftOutOfTheListing(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "short"+extension), []byte("tiny"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-directory"+extension), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	listed, listErr := held.List(0)

	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(listed) != 1 || listed[0].ID != "one" {
		t.Fatalf("listed %v, want only the session", listed)
	}
}

func TestOnlyAsManySessionsAsAskedForComeBack(t *testing.T) {
	held := Open(t.TempDir())
	for _, id := range []string{"a", "b", "c"} {
		one := header(id)
		one.At = "2026-09-0" + id[:1] + "T12:00:00Z"
		session, err := held.Start(one)
		if err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		session.Close()
	}

	listed, err := held.List(2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d, want 2", len(listed))
	}
}

func TestAHeaderThatIsNotJSONIsNotASession(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	line := make([]byte, headerWidth)
	for at := range line {
		line[at] = ' '
	}
	copy(line, "not json")
	line[headerWidth-1] = '\n'
	if err := os.WriteFile(filepath.Join(dir, "broken"+extension), line, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	listed, err := held.List(0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed %v, want none", listed)
	}
	if _, _, textErr := held.Text("broken"); textErr == nil {
		t.Fatal("a file with an unreadable header was read as a session")
	}
}

func TestASessionThatWasCutShortIsNotReadAsOne(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	if err := os.WriteFile(filepath.Join(dir, "short"+extension), []byte("too short"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, err := held.Text("short"); err == nil {
		t.Fatal("a truncated file was read as a session")
	}
}

func TestTrimLeavesASessionWithAnUnreadableTimeAlone(t *testing.T) {
	held := Open(t.TempDir())
	one := header("one")
	one.At = "whenever"
	session, err := held.Start(one)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()

	removed, trimErr := held.Trim(time.Hour, time.Now())

	if trimErr != nil {
		t.Fatalf("trim: %v", trimErr)
	}
	if removed != 0 {
		t.Fatalf("removed %d, want none", removed)
	}
}

func TestTrimOnAStoreWithNoDirectoryRemovesNothing(t *testing.T) {
	removed, err := Open("").Trim(time.Hour, time.Now())
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed %d", removed)
	}
}

func TestWritingToASessionThatIsClosedKeepsNothing(t *testing.T) {
	held := Open(t.TempDir())
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Shown([]byte("kept"))
	session.Close()
	session.Close()

	session.Shown([]byte("not kept"))

	_, text, textErr := held.Text("one")
	if textErr != nil {
		t.Fatalf("text: %v", textErr)
	}
	if text != "kept" {
		t.Fatalf("text = %q", text)
	}
}

func TestASessionDescribedInTooManyBytesIsRefused(t *testing.T) {
	one := header("one")
	one.ID = strings.Repeat("a", headerWidth)

	if _, err := headerLine(one); err == nil {
		t.Fatal("a header wider than the file format was accepted")
	}
}

func TestAnUnknownSessionHasItsOwnError(t *testing.T) {
	if ErrUnknown() == nil {
		t.Fatal("there is no error for an unknown session")
	}
}

func TestASessionWhoseFileStopsAcceptingWritesStopsRecording(t *testing.T) {
	held := Open(t.TempDir())
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Shown([]byte("kept"))
	if closeErr := session.file.Close(); closeErr != nil {
		t.Fatalf("close the file behind it: %v", closeErr)
	}

	session.Shown([]byte("cannot be kept"))
	session.Shown([]byte("nor this"))
	session.Close()

	_, text, textErr := held.Text("one")
	if textErr != nil {
		t.Fatalf("text: %v", textErr)
	}
	if text != "kept" {
		t.Fatalf("text = %q, want only what was written before the file went away", text)
	}
}

func TestASessionThatCannotBeReadIsReportedRatherThanEmpty(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	session, err := held.Start(header("one"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()
	if chmodErr := os.Chmod(filepath.Join(dir, "one"+extension), 0o000); chmodErr != nil {
		t.Skipf("this filesystem does not enforce permissions: %v", chmodErr)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "one"+extension), 0o600) })

	if _, _, textErr := held.Text("one"); textErr == nil {
		t.Fatal("a session that could not be opened read back clean")
	}
	if _, listErr := held.List(0); listErr != nil {
		t.Fatalf("a listing gave up because one session could not be read: %v", listErr)
	}
}

func TestTrimReportsADirectoryItCannotList(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Open(blocked).Trim(time.Hour, time.Now()); err == nil {
		t.Fatal("a trim of a directory that is not one came back clean")
	}
}

func TestTrimCountsOnlyWhatItActuallyRemoved(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	noon := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	one := header("one")
	one.At = noon.Add(-48 * time.Hour).Format(time.RFC3339)
	session, err := held.Start(one)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()
	if rmErr := os.Remove(filepath.Join(dir, "one"+extension)); rmErr != nil {
		t.Fatalf("remove: %v", rmErr)
	}

	removed, trimErr := held.Trim(24*time.Hour, noon)

	if trimErr != nil {
		t.Fatalf("trim: %v", trimErr)
	}
	if removed != 0 {
		t.Fatalf("removed %d sessions that were not there", removed)
	}
}

func TestASessionCannotBeStartedWithNowhereToPutTheDirectory(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	held := Open(filepath.Join(blocked, "under-a-file"))

	if _, err := held.Start(header("one")); err == nil {
		t.Fatal("a session started under a path that cannot hold a directory")
	}
}

func TestWithNoConfigDirectoryThereIsNowhereToRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows finds its config directory another way")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if _, err := DefaultDir(); err == nil {
		t.Fatal("a machine with no config directory still named one")
	}
}

func TestASessionCannotBeStartedOverADirectory(t *testing.T) {
	dir := t.TempDir()
	held := Open(dir)
	if err := os.Mkdir(filepath.Join(dir, "one"+extension), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := held.Start(header("one")); err == nil {
		t.Fatal("a session opened a directory as its file")
	}
}

func TestASessionThatCannotBeDescribedIsRefusedBeforeItIsWritten(t *testing.T) {
	held := Open(t.TempDir())
	one := header("one")
	one.Target = strings.Repeat("a", fieldWidth)
	one.Actor = strings.Repeat("b", fieldWidth)
	one.Kind = strings.Repeat("c", fieldWidth)
	one.ID = strings.Repeat("d", headerWidth)

	if _, err := held.Start(one); err == nil {
		t.Fatal("a session too wide to describe was started")
	}
}

func TestAnEmptyValueIsLeftAloneWhenShortening(t *testing.T) {
	if got := shortened(""); got != "" {
		t.Fatalf("shortened an empty value to %q", got)
	}
	if got := shortened("short"); got != "short" {
		t.Fatalf("shortened a short value to %q", got)
	}
}
