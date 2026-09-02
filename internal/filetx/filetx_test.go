package filetx

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gofrs/flock"
)

type stubReadCloser struct {
	reader   io.Reader
	closeErr error
}

func (s stubReadCloser) Read(body []byte) (int, error) {
	return s.reader.Read(body)
}

func (s stubReadCloser) Close() error {
	return s.closeErr
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestReadReturnsAFileAtItsExactLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	body, err := Read(path, 4)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "1234" {
		t.Fatalf("body = %q, want the complete file", body)
	}
}

func TestReadAcceptsTheMaximumByteLimitWithoutOverflowing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("complete"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	body, err := Read(path, math.MaxInt64)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "complete" {
		t.Fatalf("body = %q, want the complete file", body)
	}
}

func TestReadRejectsInvalidLimitsBeforeOpeningTheFile(t *testing.T) {
	for _, limit := range []int64{-1, 0} {
		_, err := Read(filepath.Join(t.TempDir(), "missing.json"), limit)

		if err == nil || !strings.Contains(err.Error(), "invalid byte limit") {
			t.Fatalf("Read limit %d error = %v, want the invalid limit", limit, err)
		}
	}
}

func TestReadReportsAMissingFile(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "missing.json"), 10)

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read error = %v, want the missing file", err)
	}
}

func TestReadRejectsADirectory(t *testing.T) {
	_, err := Read(t.TempDir(), 10)

	if err == nil {
		t.Fatal("a directory was returned as file contents")
	}
}

func TestReadRejectsAFileAboveItsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Read(path, 3)

	if err == nil || !strings.Contains(err.Error(), "larger than 3 bytes") {
		t.Fatalf("read error = %v, want the size limit", err)
	}
}

func TestReadReturnsACloseFailure(t *testing.T) {
	want := errors.New("close failed")
	file := stubReadCloser{
		reader:   strings.NewReader("ok"),
		closeErr: want,
	}

	body, err := read("state.json", file, 2)

	if body != nil {
		t.Fatalf("body = %q, want no body after close failure", body)
	}
	if !errors.Is(err, want) {
		t.Fatalf("read error = %v, want %v", err, want)
	}
}

func TestReadReturnsAReadFailureBeforeACloseFailure(t *testing.T) {
	wantRead := errors.New("read failed")
	file := stubReadCloser{
		reader:   errorReader{err: wantRead},
		closeErr: errors.New("close failed"),
	}

	body, err := read("state.json", file, 2)

	if body != nil {
		t.Fatalf("body = %q, want no body after read failure", body)
	}
	if !errors.Is(err, wantRead) {
		t.Fatalf("read error = %v, want %v", err, wantRead)
	}
}

func TestExclusiveStopsWaitingWhenTheCallerIsCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	guard := flock.New(path + ".lock")
	locked, lockErr := guard.TryLock()
	if lockErr != nil {
		t.Fatalf("hold lock: %v", lockErr)
	}
	if !locked {
		t.Fatal("could not hold the lock")
	}
	t.Cleanup(func() {
		if err := guard.Unlock(); err != nil {
			t.Errorf("unlock: %v", err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false

	err := Exclusive(ctx, path, func() error {
		called = true
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("exclusive error = %v, want cancellation", err)
	}
	if called {
		t.Fatal("the action ran without acquiring the lock")
	}
}

func TestExclusiveCreatesPrivateLockingState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "state.json")
	called := false

	err := Exclusive(t.Context(), path, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("exclusive: %v", err)
	}
	if !called {
		t.Fatal("the exclusive action was not called")
	}
	info, statErr := os.Stat(dir)
	if statErr != nil {
		t.Fatalf("stat directory: %v", statErr)
	}
	if info.Mode().Perm() != dirMode {
		t.Fatalf("directory mode = %v, want %v", info.Mode().Perm(), dirMode)
	}
	lock, statErr := os.Stat(path + ".lock")
	if statErr != nil {
		t.Fatalf("stat lock: %v", statErr)
	}
	if lock.Mode().Perm() != fileMode {
		t.Fatalf("lock mode = %v, want %v", lock.Mode().Perm(), fileMode)
	}
}

func TestExclusiveDoesNotCallTheActionWhenItsDirectoryCannotBeCreated(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	called := false

	err := Exclusive(t.Context(), filepath.Join(blocked, "state.json"), func() error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("a transaction inside a file reported success")
	}
	if called {
		t.Fatal("the action ran without a transaction directory")
	}
}

func TestExclusiveDoesNotCallTheActionWhenTheLockCannotBeCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), strings.Repeat("x", 300))
	called := false

	err := Exclusive(t.Context(), path, func() error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("a transaction with no lock reported success")
	}
	if called {
		t.Fatal("the action ran without a lock")
	}
}

func TestExclusiveReturnsTheActionFailureAndReleasesTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := errors.New("update failed")

	err := Exclusive(t.Context(), path, func() error {
		return want
	})

	if !errors.Is(err, want) {
		t.Fatalf("exclusive error = %v, want %v", err, want)
	}
	called := false
	if retryErr := Exclusive(t.Context(), path, func() error {
		called = true
		return nil
	}); retryErr != nil {
		t.Fatalf("retry: %v", retryErr)
	}
	if !called {
		t.Fatal("the action failure left the lock held")
	}
}

func TestExclusiveReleasesTheLockWhenTheActionPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the action panic was swallowed")
			}
		}()
		_ = Exclusive(t.Context(), path, func() error {
			panic("broken update")
		})
	}()

	if err := Exclusive(t.Context(), path, func() error { return nil }); err != nil {
		t.Fatalf("transaction after panic: %v", err)
	}
}

func TestExclusiveSerializesConcurrentActions(t *testing.T) {
	const writers = 32
	path := filepath.Join(t.TempDir(), "state.json")
	var active atomic.Int32
	var completed atomic.Int32
	var overlapped atomic.Bool
	errs := make(chan error, writers)
	var group sync.WaitGroup

	for range writers {
		group.Go(func() {
			errs <- Exclusive(t.Context(), path, func() error {
				if active.Add(1) != 1 {
					overlapped.Store(true)
				}
				for range 8 {
					runtime.Gosched()
				}
				active.Add(-1)
				completed.Add(1)
				return nil
			})
		})
	}
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("exclusive: %v", err)
		}
	}
	if overlapped.Load() {
		t.Fatal("exclusive actions overlapped")
	}
	if completed.Load() != writers {
		t.Fatalf("completed = %d, want %d", completed.Load(), writers)
	}
}
