package history

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
	Cluster string
	Limit   int
}

type Page struct {
	Entries []Entry
	More    bool
}

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	reason string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("history: %w", err)
	}
	return filepath.Join(dir, "spinoza", "history.db"), nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return unavailable("spinoza has nowhere to keep history"), nil
	}
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		return unavailable(reasonFor(path, err)), fmt.Errorf("history: %w", err)
	}
	db := sql.OpenDB(connector{dsn: path + pragmas})
	migrateErr := migrate(ctx, db)
	if migrateErr != nil {
		_ = db.Close()
		return unavailable(reasonFor(path, migrateErr)), fmt.Errorf("history: %w", migrateErr)
	}
	return &Store{db: db}, nil
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

func (s *Store) handle() *sql.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db
}

func (s *Store) Record(ctx context.Context, entry Entry) error {
	db := s.handle()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(
		ctx, insertAudit,
		entry.Cluster, entry.At.UTC().UnixMilli(), entry.Verb,
		entry.Group, entry.Version, entry.Resource, entry.Kind,
		entry.Namespace, entry.Name,
		entry.Detail, entry.Outcome, entry.Message,
	)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (s *Store) Recent(ctx context.Context, query Query) (Page, error) {
	db := s.handle()
	if db == nil {
		return Page{Entries: []Entry{}}, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	rows, err := db.QueryContext(ctx, selectAudit, query.Cluster, query.Cluster, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found, scanErr := scanEntries(rows)
	if scanErr != nil {
		return Page{}, scanErr
	}
	return pageOf(found, limit), nil
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
			&entry.ID, &entry.Cluster, &at, &entry.Verb,
			&entry.Group, &entry.Version, &entry.Resource, &entry.Kind,
			&entry.Namespace, &entry.Name,
			&entry.Detail, &entry.Outcome, &entry.Message,
		)
		if err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		entry.At = time.UnixMilli(at).UTC()
		found = append(found, entry)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("history: %w", rows.Err())
	}
	return found, nil
}

func (s *Store) Forget(ctx context.Context) error {
	db := s.handle()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, "DELETE FROM audit")
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	db := s.db
	s.db = nil
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	err := db.Close()
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return nil
}
