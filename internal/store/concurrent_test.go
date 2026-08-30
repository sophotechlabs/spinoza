package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

const secondWindowRows = "SPINOZA_TEST_HISTORY_ROWS"

const beside = "history.db"

const otherWindow = "https://p-mk9:6443"

func idOf(n int) string {
	return fmt.Sprintf("https://c%d:6443", n)
}

func countIn(t *testing.T, store *Store, cluster string) int {
	t.Helper()
	var found int
	row := store.reads.QueryRowContext(t.Context(),
		"SELECT count(*) FROM audit WHERE (? = '' OR cluster = ?)", cluster, cluster)
	if err := row.Scan(&found); err != nil {
		t.Fatalf("count: %v", err)
	}
	return found
}

func writeFrom(t *testing.T, store *Store, clusters, each int) {
	t.Helper()
	var writing sync.WaitGroup
	for which := range clusters {
		writing.Add(1)
		go func(which int) {
			defer writing.Done()
			into := store.For(idOf(which))
			for at := range each {
				held := entry("ignored", noon.Add(time.Duration(at)*time.Second), "web")
				if err := into.Record(context.Background(), held); err != nil {
					t.Errorf("cluster %d: record: %v", which, err)
					return
				}
			}
		}(which)
	}
	writing.Wait()
}

func TestEveryOpenClusterCanWriteAtOnce(t *testing.T) {
	store := openHistory(t, dbPath(t))
	const clusters = 16
	const each = 40

	writeFrom(t, store, clusters, each)

	if found := countIn(t, store, ""); found != clusters*each {
		t.Fatalf("landed %d rows, want %d; writes from open clusters were lost", found, clusters*each)
	}
}

func TestEachClustersRowsStayItsOwn(t *testing.T) {
	store := openHistory(t, dbPath(t))
	const clusters = 16
	const each = 40

	writeFrom(t, store, clusters, each)

	for which := range clusters {
		if found := countIn(t, store, idOf(which)); found != each {
			t.Fatalf("cluster %d holds %d rows, want %d; concurrent writes were misattributed", which, found, each)
		}
	}
}

func TestAWriterStampsItsOwnClusterOverWhateverItIsHanded(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if err := store.For(p1).Record(t.Context(), entry(p2, noon, "web")); err != nil {
		t.Fatalf("record: %v", err)
	}

	page := recent(t, store, Query{Cluster: p1})
	if len(page.Entries) != 1 {
		t.Fatalf("cluster %s holds %d rows, want the one its writer wrote", p1, len(page.Entries))
	}
	if held := recent(t, store, Query{Cluster: p2}); len(held.Entries) != 0 {
		t.Fatalf("cluster %s holds %d rows, want none; the entry named it but the writer did not", p2, len(held.Entries))
	}
}

func TestWritesGoThroughOneConnection(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if open := store.writes.Stats().MaxOpenConnections; open != 1 {
		t.Fatalf("writes may open %d connections, want 1; several fight for the file lock and one of them starves", open)
	}
}

func TestReadsDoNotQueueBehindWrites(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if open := store.reads.Stats().MaxOpenConnections; open <= 1 {
		t.Fatalf("reads may open %d connections, want more than one so a read does not sit in the write queue", open)
	}
}

func TestOnlyTheClusterAskedForIsForgotten(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "here"))
	record(t, store, entry(p2, noon, "elsewhere"))

	if err := store.Forget(t.Context(), p1); err != nil {
		t.Fatalf("forget: %v", err)
	}

	if got := names(recent(t, store, Query{})); len(got) != 1 || got[0] != "elsewhere" {
		t.Fatalf("entries = %v, want another cluster's history untouched", got)
	}
}

func TestSecondWindowWrites(t *testing.T) {
	asked := os.Getenv(secondWindowRows)
	if asked == "" {
		t.Skip("this is the other half of TestASecondSpinozaWritingToTheSameFileLosesNothing")
	}
	rows, err := strconv.Atoi(asked)
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	store, openErr := Open(t.Context(), beside)
	if openErr != nil {
		t.Fatalf("open: %v", openErr)
	}
	defer func() { _ = store.Close() }()
	into := store.For(otherWindow)
	for at := range rows {
		held := entry("ignored", noon.Add(time.Duration(at)*time.Second), "web")
		if recordErr := into.Record(t.Context(), held); recordErr != nil {
			t.Fatalf("record: %v", recordErr)
		}
	}
}

func awaitFirstRow(t *testing.T, store *Store, cluster string) {
	t.Helper()
	for range 600 {
		if countIn(t, store, cluster) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the second window never wrote anything")
}

func TestASecondSpinozaWritingToTheSameFileLosesNothing(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	const each = 1000

	//nolint:gosec // the command is this test binary and every argument after it is a constant
	second := exec.Command(os.Args[0], "-test.run", "^TestSecondWindowWrites$")
	second.Dir = filepath.Dir(path)
	second.Env = append(os.Environ(), secondWindowRows+"="+strconv.Itoa(each))
	said := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := second.CombinedOutput()
		said <- struct {
			out []byte
			err error
		}{out: out, err: err}
	}()
	awaitFirstRow(t, store, otherWindow)
	writeFrom(t, store, 1, each)
	answer := <-said
	if answer.err != nil {
		t.Fatalf("the second window: %v: %s", answer.err, answer.out)
	}

	if found := countIn(t, store, otherWindow); found != each {
		t.Fatalf("the other window landed %d of %d rows", found, each)
	}
	if found := countIn(t, store, idOf(0)); found != each {
		t.Fatalf("this window landed %d of %d rows while another wrote", found, each)
	}
}
