package history

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"
	"time"
)

var errQueryFailed = errors.New("the database went away mid-read")

type faults struct {
	columns    int
	values     []driver.Value
	execsPass  int
	beginFails bool
	commitErr  error
	closeErr   error
}

type faultyConnector struct {
	arm   faults
	execs *int
}

func (c faultyConnector) Connect(context.Context) (driver.Conn, error) {
	return faultyConn(c), nil
}

func (c faultyConnector) Driver() driver.Driver {
	return faultyDriver{}
}

type faultyDriver struct{}

func (faultyDriver) Open(string) (driver.Conn, error) {
	return faultyConn{}, nil
}

type faultyConn struct {
	arm   faults
	execs *int
}

func (c faultyConn) Prepare(string) (driver.Stmt, error) {
	return faultyStmt(c), nil
}

func (c faultyConn) Close() error {
	return c.arm.closeErr
}

func (c faultyConn) Begin() (driver.Tx, error) {
	if c.arm.beginFails {
		return nil, errQueryFailed
	}
	return faultyTx{arm: c.arm}, nil
}

type faultyTx struct {
	arm faults
}

func (t faultyTx) Commit() error {
	return t.arm.commitErr
}

func (faultyTx) Rollback() error {
	return nil
}

type faultyStmt struct {
	arm   faults
	execs *int
}

func (faultyStmt) Close() error {
	return nil
}

func (faultyStmt) NumInput() int {
	return -1
}

func (s faultyStmt) Exec([]driver.Value) (driver.Result, error) {
	*s.execs++
	if *s.execs <= s.arm.execsPass {
		return driver.RowsAffected(1), nil
	}
	return nil, errQueryFailed
}

func (s faultyStmt) Query([]driver.Value) (driver.Rows, error) {
	return &faultyRows{columns: s.arm.columns, values: s.arm.values}, nil
}

type faultyRows struct {
	columns int
	values  []driver.Value
	served  bool
}

func (r *faultyRows) Columns() []string {
	names := make([]string, r.columns)
	for at := range names {
		names[at] = "c"
	}
	return names
}

func (*faultyRows) Close() error {
	return nil
}

func (r *faultyRows) Next(dest []driver.Value) error {
	if r.values == nil {
		return errQueryFailed
	}
	if r.served {
		return io.EOF
	}
	r.served = true
	copy(dest, r.values)
	return nil
}

func faultyDB(t *testing.T, arm faults) *sql.DB {
	t.Helper()
	execs := 0
	db := sql.OpenDB(faultyConnector{arm: arm, execs: &execs})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func faultyStore(t *testing.T, columns int, values []driver.Value) *Store {
	t.Helper()
	db := faultyDB(t, faults{columns: columns, values: values})
	return &Store{writes: db, reads: db}
}

func TestAWriteThatFailsIsPassedUp(t *testing.T) {
	store := faultyStore(t, 0, nil)
	tabs := store.Tabs()
	ctx := context.Background()

	cases := map[string]func() error{
		"remember":  func() error { return tabs.Remember(ctx, Tab{ID: "one"}) },
		"forget":    func() error { return tabs.Forget(ctx, "one") },
		"recolor":   func() error { return tabs.Recolor(ctx, "one", 3) },
		"rename":    func() error { return tabs.Rename(ctx, "one", "label", "group") },
		"reopening": func() error { return tabs.Reopening(ctx, "one", true) },
		"recording": func() error { return tabs.Recording(ctx, "one", "pods") },
		"record":    func() error { return store.record(ctx, Entry{Name: "web"}) },
		"forgetAll": func() error { return store.Forget(ctx, "one") },
		"prune":     func() error { return store.Prune(ctx, Retention{Days: 7}, time.Unix(1700000000, 0)) },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call()
			if !errors.Is(err, errQueryFailed) {
				t.Fatalf("err = %v, want the driver failure passed up", err)
			}
		})
	}
}

func TestAReadThatBreaksPartWayThroughIsReported(t *testing.T) {
	ctx := context.Background()

	cases := map[string]struct {
		columns int
		read    func(*Store) error
	}{
		"tabs": {columns: 9, read: func(s *Store) error {
			_, err := s.Tabs().All(ctx)
			return err
		}},
		"entries": {columns: 13, read: func(s *Store) error {
			_, err := s.Recent(ctx, Query{})
			return err
		}},
		"changes": {columns: 12, read: func(s *Store) error {
			_, err := s.Changed(ctx, Query{Cluster: "one"})
			return err
		}},
	}
	for name, one := range cases {
		t.Run(name, func(t *testing.T) {
			err := one.read(faultyStore(t, one.columns, nil))
			if !errors.Is(err, errQueryFailed) {
				t.Fatalf("err = %v, want the read failure reported", err)
			}
		})
	}
}

func TestARowThatWillNotScanIsReported(t *testing.T) {
	ctx := context.Background()

	cases := map[string]struct {
		columns int
		values  []driver.Value
		read    func(*Store) error
	}{
		"tabs": {
			columns: 9, values: []driver.Value{"id", "ctx", "path", "not a number", int64(0), "", "", false, ""},
			read: func(s *Store) error {
				_, err := s.Tabs().All(ctx)
				return err
			},
		},
		"entries": {
			columns: 13, values: []driver.Value{int64(1), "one", "not a number", "get", "", "", "", "", "", "web", "", "done", ""},
			read: func(s *Store) error {
				_, err := s.Recent(ctx, Query{})
				return err
			},
		},
		"changes": {
			columns: 12, values: []driver.Value{int64(1), "one", "not a number", "get", "", "", "", "", "", "web", "uid", ""},
			read: func(s *Store) error {
				_, err := s.Changed(ctx, Query{Cluster: "one"})
				return err
			},
		},
	}
	for name, one := range cases {
		t.Run(name, func(t *testing.T) {
			err := one.read(faultyStore(t, one.columns, one.values))
			if err == nil {
				t.Fatal("a row that would not scan was reported as read")
			}
		})
	}
}

func TestForgettingAClusterReportsTheSecondWriteFailing(t *testing.T) {
	store := &Store{writes: faultyDB(t, faults{execsPass: 1}), reads: nil}

	err := store.Forget(context.Background(), "one")

	if !errors.Is(err, errQueryFailed) {
		t.Fatalf("err = %v, want the second delete's failure", err)
	}
}

func TestCappingRowsReportsTheDeleteFailing(t *testing.T) {
	db := faultyDB(t, faults{columns: 1, values: []driver.Value{int64(42)}})
	store := &Store{writes: db, reads: db}

	err := store.Prune(context.Background(), Retention{Rows: 10}, time.Unix(1700000000, 0))

	if !errors.Is(err, errQueryFailed) {
		t.Fatalf("err = %v, want the cut-off delete's failure", err)
	}
}

func TestNotingChangesReportsWhatTheTransactionDid(t *testing.T) {
	change := Change{Cluster: "one", Verb: "update", Name: "web", Cells: []string{"web", "1/1"}}
	ctx := context.Background()

	cases := map[string]faults{
		"the transaction would not start": {beginFails: true},
		"a row would not insert":          {},
		"the commit failed":               {execsPass: 1, commitErr: errQueryFailed},
	}
	for name, arm := range cases {
		t.Run(name, func(t *testing.T) {
			store := &Store{writes: faultyDB(t, arm)}

			err := store.note(ctx, []Change{change})

			if !errors.Is(err, errQueryFailed) {
				t.Fatalf("err = %v, want %s reported", err, name)
			}
		})
	}
}

func TestApplyingAMigrationReportsWhatTheTransactionDid(t *testing.T) {
	ctx := context.Background()

	cases := map[string]faults{
		"the transaction would not start": {beginFails: true},
		"the statements would not run":    {},
		"the version would not stick":     {execsPass: 1},
		"the commit failed":               {execsPass: 2, commitErr: errQueryFailed},
	}
	for name, arm := range cases {
		t.Run(name, func(t *testing.T) {
			err := apply(ctx, faultyDB(t, arm), "CREATE TABLE t (a)", 1)

			if !errors.Is(err, errQueryFailed) {
				t.Fatalf("err = %v, want %s reported", err, name)
			}
		})
	}
}

func TestClosingReportsADatabaseThatWouldNotClose(t *testing.T) {
	db := sql.OpenDB(faultyConnector{arm: faults{closeErr: errQueryFailed}, execs: new(int)})
	store := &Store{writes: db, reads: db}
	_ = store.Tabs().Forget(context.Background(), "one")

	err := store.Close()

	if !errors.Is(err, errQueryFailed) {
		t.Fatalf("err = %v, want the close failure passed up", err)
	}
}
