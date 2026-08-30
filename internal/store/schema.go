package store

import (
	"context"
	"database/sql"
	"fmt"
)

const defaultLimit = 200

const maxLimit = 1000

const readers = 4

const insertAudit = `
INSERT INTO audit (
	cluster, at, verb, api_group, api_version, resource, kind,
	namespace, name, detail, outcome, message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectAudit = `
SELECT id, cluster, at, verb, api_group, api_version, resource, kind,
	namespace, name, detail, outcome, message
FROM audit
WHERE (? = '' OR cluster = ?) AND (? = 0 OR id < ?)
ORDER BY at DESC, id DESC
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
SELECT id, cluster, at, verb, api_group, api_version, resource, kind,
	namespace, name, uid, cells, was
FROM changes
WHERE (? = '' OR cluster = ?) AND (? = 0 OR id < ?)
ORDER BY at DESC, id DESC
LIMIT ?`

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
`}

func migrate(ctx context.Context, db *sql.DB) error {
	applied, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	for version := applied; version < len(migrations); version++ {
		stepErr := apply(ctx, db, migrations[version], version+1)
		if stepErr != nil {
			return stepErr
		}
	}
	return nil
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errNoSchema, err)
	}
	if version > len(migrations) {
		return 0, fmt.Errorf("%w: it is at version %d and this spinoza knows %d", errFromTheFuture, version, len(migrations))
	}
	return version, nil
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
	_, versionErr := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version))
	if versionErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", versionErr)
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}
