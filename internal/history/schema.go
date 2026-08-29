package history

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
WHERE (? = '' OR cluster = ?)
ORDER BY at DESC, id DESC
LIMIT ?`

const deleteAudit = `
DELETE FROM audit
WHERE (? = '' OR cluster = ?)`

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
		return fmt.Errorf("history: %w", err)
	}
	_, execErr := tx.ExecContext(ctx, statements)
	if execErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("history: %w", execErr)
	}
	_, versionErr := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version))
	if versionErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("history: %w", versionErr)
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("history: %w", commitErr)
	}
	return nil
}
