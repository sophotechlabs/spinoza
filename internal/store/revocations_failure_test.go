package store

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRevocationReportsEachTransactionFailure(t *testing.T) {
	cases := map[string]faults{
		"begin":  {beginFails: true},
		"sweep":  {},
		"insert": {execsPass: 1},
		"commit": {execsPass: 2, commitErr: errQueryFailed},
	}
	for name, arm := range cases {
		t.Run(name, func(t *testing.T) {
			store := &Store{writes: faultyDB(t, arm)}
			err := store.Revoke(t.Context(), "session-refused", noon.Add(time.Hour), noon)
			if !errors.Is(err, errQueryFailed) {
				t.Fatalf("error = %v, want the %s failure", err, name)
			}
			if err.Error() != "store: "+errQueryFailed.Error() {
				t.Fatalf("error = %q, want the store failure with its cause", err)
			}
		})
	}
}

func TestAFailedRevocationRollsBackItsExpiredSessionSweep(t *testing.T) {
	store := openHistory(t, dbPath(t))
	expiry := noon.Add(time.Hour)
	if err := store.Revoke(t.Context(), "expired-but-kept-on-rollback", expiry, noon); err != nil {
		t.Fatalf("seed revocation: %v", err)
	}
	trigger := `CREATE TRIGGER refuse_revocation BEFORE INSERT ON revoked_sessions
BEGIN SELECT RAISE(ABORT, 'revocation storage refused'); END`
	if _, err := store.writes.ExecContext(t.Context(), trigger); err != nil {
		t.Fatalf("create refusal trigger: %v", err)
	}
	refused := store.Revoke(t.Context(), "new-session", noon.Add(3*time.Hour), noon.Add(2*time.Hour))
	if refused == nil || !strings.Contains(refused.Error(), "revocation storage refused") {
		t.Fatalf("error = %v, want the real database refusal", refused)
	}
	var session string
	if err := store.reads.QueryRowContext(t.Context(), "SELECT session FROM revoked_sessions").Scan(&session); err != nil {
		t.Fatalf("read rolled back sweep: %v", err)
	}
	if session != "expired-but-kept-on-rollback" {
		t.Fatalf("session = %q, want the sweep rolled back with the refused insertion", session)
	}
	if _, err := store.writes.ExecContext(t.Context(), "DROP TRIGGER refuse_revocation"); err != nil {
		t.Fatalf("remove refusal: %v", err)
	}
	if err := store.Revoke(t.Context(), "new-session", noon.Add(3*time.Hour), noon.Add(2*time.Hour)); err != nil {
		t.Fatalf("revoke after recovery: %v", err)
	}
	var count int
	if err := store.reads.QueryRowContext(t.Context(), "SELECT count(*) FROM revoked_sessions").Scan(&count); err != nil {
		t.Fatalf("count recovered rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want only the new revocation after recovery", count)
	}
	held, err := store.Revocations(t.Context(), noon.Add(2*time.Hour))
	if err != nil || len(held) != 1 || !held["new-session"].Equal(noon.Add(3*time.Hour)) {
		t.Fatalf("recovered revocations = %v/%v, want the new session and its expiry", held, err)
	}
}

func TestACanceledRevocationLeavesTheStoredExpiryUnchanged(t *testing.T) {
	store := openHistory(t, dbPath(t))
	expiry := noon.Add(time.Hour)
	if err := store.Revoke(t.Context(), "held", expiry, noon); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := store.Revoke(ctx, "held", noon.Add(2*time.Hour), noon)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
	held, err := store.Revocations(t.Context(), noon)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(held) != 1 || !held["held"].Equal(expiry) {
		t.Fatalf("revocations = %v, want the original expiry %s", held, expiry)
	}
}

func TestARevocationReadFailureReturnsNoPartialSessionSet(t *testing.T) {
	db := faultyDB(t, faults{
		columns: 2, values: []driver.Value{"first-session", noon.Add(time.Hour).UnixMilli()},
		readErr: errQueryFailed,
	})
	store := &Store{writes: db, reads: db}
	held, err := store.Revocations(t.Context(), noon)
	if !errors.Is(err, errQueryFailed) || held != nil {
		t.Fatalf("revocations/error = %v/%v, want no sessions and the driver error", held, err)
	}
}

func TestARevocationWithAnUnreadableExpiryIsRefused(t *testing.T) {
	store := faultyStore(t, 2, []driver.Value{"broken", "not-an-expiry"})
	held, err := store.Revocations(t.Context(), noon)
	if err == nil || !strings.Contains(err.Error(), "not-an-expiry") || held != nil {
		t.Fatalf("revocations/error = %v/%v, want no sessions and the malformed expiry", held, err)
	}
}

func TestACanceledRevocationReadIsNotReportedAsAnEmptySuccess(t *testing.T) {
	store := openHistory(t, dbPath(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	held, err := store.Revocations(ctx, noon)
	if !errors.Is(err, context.Canceled) || held != nil {
		t.Fatalf("revocations/error = %v/%v, want canceled and no result", held, err)
	}
}

func TestRevocationExpiryIsExclusiveAtTheStoredMillisecond(t *testing.T) {
	store := openHistory(t, dbPath(t))
	expiry := noon.Add(time.Hour)
	if err := store.Revoke(t.Context(), "boundary", expiry, noon); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, err := store.Revocations(t.Context(), expiry.Add(-time.Millisecond))
	if err != nil || len(before) != 1 {
		t.Fatalf("before expiry = %v/%v, want the active revocation", before, err)
	}
	at, err := store.Revocations(t.Context(), expiry)
	if err != nil || len(at) != 0 {
		t.Fatalf("at expiry = %v/%v, want no active revocation", at, err)
	}
}
