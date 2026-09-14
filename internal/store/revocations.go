package store

import (
	"context"
	"fmt"
	"time"
)

const insertRevocation = `
INSERT INTO revoked_sessions (session, until) VALUES (?, ?)
ON CONFLICT (session) DO UPDATE SET until = excluded.until`

const deleteRevocationsBefore = `DELETE FROM revoked_sessions WHERE until <= ?`

const selectRevocations = `SELECT session, until FROM revoked_sessions WHERE until > ?`

func (s *Store) Revoke(ctx context.Context, session string, until, now time.Time) error {
	db := s.writer()
	if db == nil || session == "" {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	_, sweepErr := tx.ExecContext(ctx, deleteRevocationsBefore, now.UTC().UnixMilli())
	if sweepErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", sweepErr)
	}
	_, writeErr := tx.ExecContext(ctx, insertRevocation, session, until.UTC().UnixMilli())
	if writeErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", writeErr)
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}

func (s *Store) Revocations(ctx context.Context, now time.Time) (map[string]time.Time, error) {
	db := s.reader()
	if db == nil {
		return map[string]time.Time{}, nil
	}
	rows, err := db.QueryContext(ctx, selectRevocations, now.UTC().UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	out := map[string]time.Time{}
	for rows.Next() {
		var session string
		var until int64
		scanErr := rows.Scan(&session, &until)
		if scanErr != nil {
			return nil, fmt.Errorf("store: %w", scanErr)
		}
		out[session] = time.UnixMilli(until).UTC()
	}
	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, fmt.Errorf("store: %w", rowsErr)
	}
	return out, nil
}
