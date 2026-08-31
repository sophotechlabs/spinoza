package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	Added   = "added"
	Changed = "changed"
	Removed = "removed"
)

type Change struct {
	ID        int64
	Cluster   string
	At        time.Time
	Verb      string
	Group     string
	Version   string
	Resource  string
	Kind      string
	Namespace string
	Name      string
	UID       string
	Cells     []string
	Was       []string
}

type Changes struct {
	Rows []Change
	More bool
}

type Retention struct {
	Days int
	Rows int
}

type Noter interface {
	Note(ctx context.Context, changes []Change) error
}

type noter struct {
	into    *Store
	cluster string
}

func (s *Store) Timeline(cluster string) Noter {
	return noter{into: s, cluster: cluster}
}

func (n noter) Note(ctx context.Context, changes []Change) error {
	for at := range changes {
		changes[at].Cluster = n.cluster
	}
	return n.into.note(ctx, changes)
}

func (s *Store) note(ctx context.Context, changes []Change) error {
	db := s.writer()
	if db == nil {
		return nil
	}
	if len(changes) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	writeErr := insertChanges(ctx, tx, changes)
	if writeErr != nil {
		_ = tx.Rollback()
		return writeErr
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}

func insertChanges(ctx context.Context, tx *sql.Tx, changes []Change) error {
	for _, one := range changes {
		_, err := tx.ExecContext(
			ctx, insertChange,
			one.Cluster, one.At.UTC().UnixMilli(), one.Verb,
			one.Group, one.Version, one.Resource, one.Kind,
			one.Namespace, one.Name, one.UID, cellsText(one.Cells), cellsText(one.Was),
		)
		if err != nil {
			return fmt.Errorf("store: %w", err)
		}
	}
	return nil
}

func cellsText(cells []string) string {
	if len(cells) == 0 {
		return "[]"
	}
	text, err := json.Marshal(cells)
	if err != nil {
		return "[]"
	}
	return string(text)
}

func cellsOf(text string) []string {
	cells := []string{}
	err := json.Unmarshal([]byte(text), &cells)
	if err != nil {
		return []string{}
	}
	return cells
}

func (s *Store) Changed(ctx context.Context, query Query) (Changes, error) {
	db := s.reader()
	if db == nil {
		return Changes{Rows: []Change{}}, nil
	}
	limit := limitOf(query.Limit)
	rows, err := db.QueryContext(
		ctx, selectChanges, query.Cluster, query.Cluster, query.After, query.After, limit+1,
	)
	if err != nil {
		return Changes{}, fmt.Errorf("store: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found, scanErr := scanChanges(rows)
	if scanErr != nil {
		return Changes{}, scanErr
	}
	if len(found) <= limit {
		return Changes{Rows: found}, nil
	}
	return Changes{Rows: found[:limit], More: true}, nil
}

func scanChanges(rows *sql.Rows) ([]Change, error) {
	found := []Change{}
	for rows.Next() {
		var one Change
		var at int64
		var cells string
		var was string
		err := rows.Scan(
			&one.ID, &one.Cluster, &at, &one.Verb,
			&one.Group, &one.Version, &one.Resource, &one.Kind,
			&one.Namespace, &one.Name, &one.UID, &cells, &was,
		)
		if err != nil {
			return nil, fmt.Errorf("store: %w", err)
		}
		one.At = time.UnixMilli(at).UTC()
		one.Cells = cellsOf(cells)
		one.Was = cellsOf(was)
		found = append(found, one)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("store: %w", rows.Err())
	}
	return found, nil
}

type trimmable struct {
	before string
	oldest string
	below  string
}

var timelineTrim = trimmable{
	before: deleteChangesBefore,
	oldest: oldestChangeKept,
	below:  deleteChangesBelow,
}

var auditTrim = trimmable{
	before: deleteAuditBefore,
	oldest: oldestAuditKept,
	below:  deleteAuditBelow,
}

func (s *Store) Prune(ctx context.Context, keep Retention, now time.Time) error {
	return s.trim(ctx, timelineTrim, keep, now)
}

func (s *Store) PruneAudit(ctx context.Context, keep Retention, now time.Time) error {
	return s.trim(ctx, auditTrim, keep, now)
}

func (s *Store) trim(ctx context.Context, table trimmable, keep Retention, now time.Time) error {
	db := s.writer()
	if db == nil {
		return nil
	}
	if keep.Days > 0 {
		cutoff := now.UTC().AddDate(0, 0, -keep.Days).UnixMilli()
		_, err := db.ExecContext(ctx, table.before, cutoff)
		if err != nil {
			return fmt.Errorf("store: %w", err)
		}
	}
	if keep.Rows <= 0 {
		return nil
	}
	return s.capRows(ctx, db, table, keep.Rows)
}

func (s *Store) capRows(ctx context.Context, db *sql.DB, table trimmable, rows int) error {
	var oldest int64
	err := db.QueryRowContext(ctx, table.oldest, rows).Scan(&oldest)
	if err != nil {
		return nothingToCap(err)
	}
	_, cutErr := db.ExecContext(ctx, table.below, oldest)
	if cutErr != nil {
		return fmt.Errorf("store: %w", cutErr)
	}
	return nil
}

func nothingToCap(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return fmt.Errorf("store: %w", err)
}
