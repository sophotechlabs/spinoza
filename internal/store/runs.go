package store

import (
	"context"
	"fmt"
	"time"
)

const runsKept = 200

type Run struct {
	ID       int64
	At       time.Time
	Findings int
	Fresh    int
	Cleared  int
	Scanned  int
}

var runTrim = trimmable{
	before: deleteRunsBefore,
	oldest: oldestRunKept,
	below:  deleteRunsBelow,
}

func (s *Store) RecordRun(ctx context.Context, cluster string, run Run) error {
	db := s.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(
		ctx, insertRun,
		cluster, run.At.UTC().UnixMilli(), run.Findings, run.Fresh, run.Cleared, run.Scanned,
	)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

func (s *Store) Runs(ctx context.Context, cluster string, limit int) ([]Run, error) {
	db := s.reader()
	if db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > runsKept {
		limit = runsKept
	}
	rows, err := db.QueryContext(ctx, selectRuns, cluster, cluster, limit)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Run{}
	for rows.Next() {
		var run Run
		var at int64
		scanErr := rows.Scan(&run.ID, &at, &run.Findings, &run.Fresh, &run.Cleared, &run.Scanned)
		if scanErr != nil {
			return nil, fmt.Errorf("store: %w", scanErr)
		}
		run.At = time.UnixMilli(at).UTC()
		out = append(out, run)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("store: %w", rows.Err())
	}
	return out, nil
}

func (s *Store) PruneRuns(ctx context.Context, keep Retention, now time.Time) error {
	return s.trim(ctx, runTrim, keep, now)
}
