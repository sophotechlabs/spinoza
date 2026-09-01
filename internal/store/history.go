package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"modernc.org/sqlite"
)

const pragmas = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"

var errNoSchema = errors.New("the history schema could not be read")

var errFromTheFuture = errors.New("this history file was written by a newer spinoza")

type Entry struct {
	ID        int64
	Cluster   string
	At        time.Time
	Verb      string
	Actor     string
	Group     string
	Version   string
	Resource  string
	Kind      string
	Namespace string
	Name      string
	Detail    string
	Outcome   string
	Message   string
}

type Query struct {
	Cluster     string
	Limit       int
	After       int64
	AfterAction int64
}

type Page struct {
	Entries []Entry
	More    bool
}

type Store struct {
	mu     sync.Mutex
	writes *sql.DB
	reads  *sql.DB
	reason string
}

type Recorder interface {
	Record(ctx context.Context, entry Entry) error
}

type writer struct {
	into    *Store
	cluster string
}

func (s *Store) For(cluster string) Recorder {
	return writer{into: s, cluster: cluster}
}

func (w writer) Record(ctx context.Context, entry Entry) error {
	entry.Cluster = w.cluster
	return w.into.record(ctx, entry)
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	return filepath.Join(dir, "spinoza", "history.db"), nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return unavailable("spinoza has nowhere to keep history"), nil
	}
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		return unavailable(reasonFor(path, err)), fmt.Errorf("store: %w", err)
	}
	conn := connector{dsn: path + pragmas}
	writes := sql.OpenDB(conn)
	writes.SetMaxOpenConns(1)
	migrateErr := migrate(ctx, writes)
	if migrateErr != nil {
		_ = writes.Close()
		return unavailable(reasonFor(path, migrateErr)), fmt.Errorf("store: %w", migrateErr)
	}
	reads := sql.OpenDB(conn)
	reads.SetMaxOpenConns(readers)
	return &Store{writes: writes, reads: reads}, nil
}

type connector struct {
	dsn string
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	return c.Driver().Open(c.dsn)
}

func (c connector) Driver() driver.Driver {
	return &sqlite.Driver{}
}

func unavailable(reason string) *Store {
	return &Store{reason: reason}
}

func reasonFor(path string, err error) string {
	return "spinoza is not recording history: " + path + ": " + err.Error()
}

func (s *Store) Reason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

func (s *Store) writer() *sql.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

func (s *Store) reader() *sql.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *Store) record(ctx context.Context, entry Entry) error {
	db := s.writer()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(
		ctx, insertAudit,
		entry.Cluster, entry.At.UTC().UnixMilli(), entry.Verb, entry.Actor,
		entry.Group, entry.Version, entry.Resource, entry.Kind,
		entry.Namespace, entry.Name,
		entry.Detail, entry.Outcome, entry.Message,
	)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

func (s *Store) Recent(ctx context.Context, query Query) (Page, error) {
	db := s.reader()
	if db == nil {
		return Page{Entries: []Entry{}}, nil
	}
	limit := limitOf(query.Limit)
	rows, err := db.QueryContext(
		ctx, selectAudit, query.Cluster, query.Cluster, query.AfterAction, query.AfterAction, limit+1,
	)
	if err != nil {
		return Page{}, fmt.Errorf("store: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found, scanErr := scanEntries(rows)
	if scanErr != nil {
		return Page{}, scanErr
	}
	return pageOf(found, limit), nil
}

func Limit(asked int) int {
	return limitOf(asked)
}

func limitOf(asked int) int {
	if asked <= 0 {
		return defaultLimit
	}
	if asked > maxLimit {
		return maxLimit
	}
	return asked
}

func pageOf(found []Entry, limit int) Page {
	if len(found) <= limit {
		return Page{Entries: found}
	}
	return Page{Entries: found[:limit], More: true}
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	found := []Entry{}
	for rows.Next() {
		var entry Entry
		var at int64
		err := rows.Scan(
			&entry.ID, &entry.Cluster, &at, &entry.Verb, &entry.Actor,
			&entry.Group, &entry.Version, &entry.Resource, &entry.Kind,
			&entry.Namespace, &entry.Name,
			&entry.Detail, &entry.Outcome, &entry.Message,
		)
		if err != nil {
			return nil, fmt.Errorf("store: %w", err)
		}
		entry.At = time.UnixMilli(at).UTC()
		found = append(found, entry)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("store: %w", rows.Err())
	}
	return found, nil
}

func (s *Store) Forget(ctx context.Context, cluster string) error {
	db := s.writer()
	if db == nil {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	_, auditErr := tx.ExecContext(ctx, deleteAudit, cluster, cluster)
	if auditErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", auditErr)
	}
	_, changesErr := tx.ExecContext(ctx, deleteChanges, cluster, cluster)
	if changesErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: %w", changesErr)
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("store: %w", commitErr)
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	open := []*sql.DB{s.reads, s.writes}
	s.reads = nil
	s.writes = nil
	s.mu.Unlock()
	failures := []error{}
	for _, db := range open {
		if db == nil {
			continue
		}
		err := db.Close()
		if err != nil {
			failures = append(failures, fmt.Errorf("store: %w", err))
		}
	}
	return errors.Join(failures...)
}
