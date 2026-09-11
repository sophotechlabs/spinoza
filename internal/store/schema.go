package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const defaultLimit = 200

const maxLimit = 1000

const readers = 4

const insertAudit = `
INSERT INTO audit (
	cluster, at, verb, actor, api_group, api_version, resource, kind,
	namespace, name, detail, outcome, message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectAudit = `
SELECT current.id, current.cluster, current.at, current.verb, current.actor,
	current.api_group, current.api_version, current.resource, current.kind,
	current.namespace, current.name, current.detail, current.outcome, current.message
FROM audit AS current
WHERE (? = '' OR current.cluster = ?)
	AND (
		? = 0 OR EXISTS (
			SELECT 1
			FROM audit AS cursor
			WHERE cursor.id = ?
				AND (? = '' OR cursor.cluster = ?)
				AND (
					current.at < cursor.at OR
					(current.at = cursor.at AND current.id < cursor.id)
				)
		)
	)
ORDER BY current.at DESC, current.id DESC
LIMIT ?`

const deleteAudit = `
DELETE FROM audit
WHERE (? = '' OR cluster = ?)`

const insertChange = `
INSERT INTO changes (
	cluster, at, verb, api_group, api_version, resource, kind,
	namespace, name, uid, cells, was
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectChanges = `
SELECT current.id, current.cluster, current.at, current.verb, current.api_group,
	current.api_version, current.resource, current.kind, current.namespace,
	current.name, current.uid, current.cells, current.was
FROM changes AS current
WHERE (? = '' OR current.cluster = ?)
	AND (
		? = 0 OR EXISTS (
			SELECT 1
			FROM changes AS cursor
			WHERE cursor.id = ?
				AND (? = '' OR cursor.cluster = ?)
				AND (
					current.at < cursor.at OR
					(current.at = cursor.at AND current.id < cursor.id)
				)
		)
	)
ORDER BY current.at DESC, current.id DESC
LIMIT ?`

const insertRun = `
INSERT INTO audit_runs (cluster, at, findings, fresh, cleared, scanned)
VALUES (?, ?, ?, ?, ?, ?)`

const selectRuns = `
SELECT id, at, findings, fresh, cleared, scanned
FROM audit_runs
WHERE (? = '' OR cluster = ?)
ORDER BY at DESC, id DESC
LIMIT ?`

const deleteRunsBefore = `DELETE FROM audit_runs WHERE at < ?`

const oldestRunKept = `SELECT id FROM audit_runs ORDER BY id DESC LIMIT 1 OFFSET ?`

const deleteRunsBelow = `DELETE FROM audit_runs WHERE id <= ?`

const deleteAuditBefore = `DELETE FROM audit WHERE at < ?`

const oldestAuditKept = `SELECT id FROM audit ORDER BY id DESC LIMIT 1 OFFSET ?`

const deleteAuditBelow = `DELETE FROM audit WHERE id <= ?`

const deleteChanges = `
DELETE FROM changes
WHERE (? = '' OR cluster = ?)`

const deleteChangesBefore = `DELETE FROM changes WHERE at < ?`

const oldestChangeKept = `SELECT id FROM changes ORDER BY id DESC LIMIT 1 OFFSET ?`

const deleteChangesBelow = `DELETE FROM changes WHERE id <= ?`

const upsertCluster = `
INSERT INTO clusters (id, context, kubeconfig, seen, color, label, grouping, reopen)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET context = excluded.context,
	kubeconfig = excluded.kubeconfig, seen = excluded.seen, color = excluded.color`

const recolorCluster = `UPDATE clusters SET color = ? WHERE id = ?`

const renameCluster = `UPDATE clusters SET label = ?, grouping = ? WHERE id = ?`

const reopenCluster = `UPDATE clusters SET reopen = ? WHERE id = ?`

const deleteCluster = `DELETE FROM clusters WHERE id = ?`

const recordCluster = `UPDATE clusters SET timeline = ? WHERE id = ?`

const selectClusters = `
SELECT id, context, kubeconfig, seen, color, label, grouping, reopen, timeline
FROM clusters
ORDER BY seen ASC, id ASC`

const readableFrom = 7

const progressTable = `
SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_progress'`

const createProgress = `
CREATE TABLE IF NOT EXISTS schema_progress (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	applied INTEGER NOT NULL
)`

const selectProgress = `SELECT applied FROM schema_progress WHERE id = 1`

const recordProgress = `
INSERT INTO schema_progress (id, applied) VALUES (1, ?)
ON CONFLICT (id) DO UPDATE SET applied = excluded.applied`

var migrations = []string{`
CREATE TABLE audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cluster TEXT NOT NULL,
	at INTEGER NOT NULL,
	verb TEXT NOT NULL,
	api_group TEXT NOT NULL,
	api_version TEXT NOT NULL,
	resource TEXT NOT NULL,
	kind TEXT NOT NULL,
	namespace TEXT NOT NULL,
	name TEXT NOT NULL,
	detail TEXT NOT NULL,
	outcome TEXT NOT NULL,
	message TEXT NOT NULL
);
CREATE INDEX audit_by_time ON audit (at DESC, id DESC);
CREATE INDEX audit_by_cluster ON audit (cluster, at DESC, id DESC);
`, `
CREATE TABLE clusters (
	id TEXT PRIMARY KEY,
	context TEXT NOT NULL,
	kubeconfig TEXT NOT NULL,
	seen INTEGER NOT NULL
);
`, `
ALTER TABLE clusters ADD COLUMN color INTEGER NOT NULL DEFAULT 0;
`, `
ALTER TABLE clusters ADD COLUMN label TEXT NOT NULL DEFAULT '';
ALTER TABLE clusters ADD COLUMN grouping TEXT NOT NULL DEFAULT '';
ALTER TABLE clusters ADD COLUMN reopen INTEGER NOT NULL DEFAULT 1;
`, `
CREATE TABLE changes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cluster TEXT NOT NULL,
	at INTEGER NOT NULL,
	verb TEXT NOT NULL,
	api_group TEXT NOT NULL,
	api_version TEXT NOT NULL,
	resource TEXT NOT NULL,
	kind TEXT NOT NULL,
	namespace TEXT NOT NULL,
	name TEXT NOT NULL,
	uid TEXT NOT NULL,
	cells TEXT NOT NULL
);
CREATE INDEX changes_by_time ON changes (at DESC, id DESC);
CREATE INDEX changes_by_cluster ON changes (cluster, at DESC, id DESC);
ALTER TABLE clusters ADD COLUMN timeline TEXT NOT NULL DEFAULT '';
`, `
ALTER TABLE changes ADD COLUMN was TEXT NOT NULL DEFAULT '[]';
`, `
ALTER TABLE audit ADD COLUMN actor TEXT NOT NULL DEFAULT 'unknown';
`, `
CREATE TABLE IF NOT EXISTS audit_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cluster TEXT NOT NULL,
	at INTEGER NOT NULL,
	findings INTEGER NOT NULL,
	fresh INTEGER NOT NULL,
	cleared INTEGER NOT NULL,
	scanned INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_runs_by_time ON audit_runs (cluster, at DESC, id DESC);
`}

func migrate(ctx context.Context, db *sql.DB) error {
	floor, err := schemaFloor(ctx, db)
	if err != nil {
		return err
	}
	applied, tracked, countErr := appliedCount(ctx, db, floor)
	if countErr != nil {
		return countErr
	}
	for version := applied; version < len(migrations); version++ {
		stepErr := apply(ctx, db, migrations[version], version+1)
		if stepErr != nil {
			return stepErr
		}
		applied = version + 1
		floor = floorFor(applied)
		tracked = true
	}
	if tracked && floor == floorFor(applied) {
		return nil
	}
	return settle(ctx, db, applied)
}

func floorFor(applied int) int {
	if applied < readableFrom {
		return applied
	}
	return readableFrom
}

func schemaFloor(ctx context.Context, db *sql.DB) (int, error) {
	var floor int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&floor)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errNoSchema, err)
	}
	if floor > len(migrations) {
		return 0, fmt.Errorf("%w: it needs one that knows %d migrations and this one knows %d", errFromTheFuture, floor, len(migrations))
	}
	return floor, nil
}

func appliedCount(ctx context.Context, db *sql.DB, floor int) (int, bool, error) {
	var tables int
	err := db.QueryRowContext(ctx, progressTable).Scan(&tables)
	if err != nil {
		return 0, false, fmt.Errorf("%w: %w", errNoSchema, err)
	}
	if tables == 0 {
		return floor, false, nil
	}
	var applied int
	readErr := db.QueryRowContext(ctx, selectProgress).Scan(&applied)
	if errors.Is(readErr, sql.ErrNoRows) {
		return floor, false, nil
	}
	if readErr != nil {
		return 0, false, fmt.Errorf("%w: %w", errNoSchema, readErr)
	}
	return applied, true, nil
}

func apply(ctx context.Context, db *sql.DB, statements string, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	_, execErr := tx.ExecContext(ctx, statements)
	if execErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", execErr)
	}
	stampErr := stamp(ctx, tx, version)
	if stampErr != nil {
		_ = tx.Rollback()
		return stampErr
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}

func settle(ctx context.Context, db *sql.DB, applied int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	stampErr := stamp(ctx, tx, applied)
	if stampErr != nil {
		_ = tx.Rollback()
		return stampErr
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}

func stamp(ctx context.Context, tx *sql.Tx, applied int) error {
	_, createErr := tx.ExecContext(ctx, createProgress)
	if createErr != nil {
		return fmt.Errorf("store: %w", createErr)
	}
	_, recordErr := tx.ExecContext(ctx, recordProgress, applied)
	if recordErr != nil {
		return fmt.Errorf("store: %w", recordErr)
	}
	_, floorErr := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", floorFor(applied)))
	if floorErr != nil {
		return fmt.Errorf("store: %w", floorErr)
	}
	return nil
}
