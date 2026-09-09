package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
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
