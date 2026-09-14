package store

import (
	"testing"
	"time"
)

func TestARevocationIsStillThereAfterTheStoreIsReopened(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	until := noon.Add(72 * time.Hour)
	if err := store.Revoke(t.Context(), "session-7", until, noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened := openHistory(t, path)
	held, err := reopened.Revocations(t.Context(), noon.Add(time.Hour))
	if err != nil {
		t.Fatalf("revocations: %v", err)
	}

	if !held["session-7"].Equal(until) {
		t.Fatalf("revocations = %v, want session-7 kept until %s", held, until)
	}
}

func TestARevocationThatHasExpiredIsNotLoaded(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if err := store.Revoke(t.Context(), "old", noon.Add(time.Hour), noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	held, err := store.Revocations(t.Context(), noon.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("revocations: %v", err)
	}

	if len(held) != 0 {
		t.Fatalf("revocations = %v, want an expired one left behind", held)
	}
}

func TestRevokingSweepsRowsThatHaveExpired(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if err := store.Revoke(t.Context(), "old", noon.Add(time.Hour), noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	later := noon.Add(2 * time.Hour)
	if err := store.Revoke(t.Context(), "new", later.Add(time.Hour), later); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	var rows int
	if err := store.reads.QueryRowContext(t.Context(), "SELECT count(*) FROM revoked_sessions").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want only the live revocation, so the table cannot only grow", rows)
	}
}

func TestRevokingTheSameSessionAgainMovesItsExpiry(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if err := store.Revoke(t.Context(), "session-7", noon.Add(time.Hour), noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := store.Revoke(t.Context(), "session-7", noon.Add(3*time.Hour), noon); err != nil {
		t.Fatalf("revoke again: %v", err)
	}

	held, err := store.Revocations(t.Context(), noon.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("revocations: %v", err)
	}
	if !held["session-7"].Equal(noon.Add(3 * time.Hour)) {
		t.Fatalf("revocations = %v, want the later expiry", held)
	}
}

func TestAnUnnamedSessionIsNotWrittenDown(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if err := store.Revoke(t.Context(), "", noon.Add(time.Hour), noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	held, err := store.Revocations(t.Context(), noon)
	if err != nil {
		t.Fatalf("revocations: %v", err)
	}
	if len(held) != 0 {
		t.Fatalf("revocations = %v, want none for the empty session id", held)
	}
}

func TestAStoreThatIsNotRecordingKeepsNoRevocations(t *testing.T) {
	store := unavailable("nowhere to write")

	if err := store.Revoke(t.Context(), "session-7", noon.Add(time.Hour), noon); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	held, err := store.Revocations(t.Context(), noon)
	if err != nil {
		t.Fatalf("revocations: %v", err)
	}
	if len(held) != 0 {
		t.Fatalf("revocations = %v, want nothing from a store with no file", held)
	}
}
