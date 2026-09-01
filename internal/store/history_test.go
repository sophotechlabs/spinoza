package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const p1 = "https://10.0.0.5:6443"

const p2 = "https://10.0.0.6:6443"

var noon = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "spinoza", "history.db")
}

func openHistory(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func entry(cluster string, at time.Time, name string) Entry {
	return Entry{
		Cluster:   cluster,
		At:        at,
		Verb:      "delete",
		Actor:     "alice@example.com",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Kind:      "Deployment",
		Namespace: "default",
		Name:      name,
		Detail:    "deleted the deployment",
		Outcome:   "ok",
		Message:   "",
	}
}

func record(t *testing.T, store *Store, held Entry) {
	t.Helper()
	if err := store.For(held.Cluster).Record(t.Context(), held); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func recent(t *testing.T, store *Store, query Query) Page {
	t.Helper()
	page, err := store.Recent(t.Context(), query)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	return page
}

func names(page Page) []string {
	out := make([]string, 0, len(page.Entries))
	for _, held := range page.Entries {
		out = append(out, held.Name)
	}
	return out
}

func pragma(t *testing.T, store *Store, name string) string {
	t.Helper()
	var value string
	err := store.reads.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&value)
	if err != nil {
		t.Fatalf("pragma %s: %v", name, err)
	}
	return value
}

func TestTheDefaultPathSitsBesideTheOtherStores(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Skipf("this platform names no config directory: %v", err)
	}
	if filepath.Base(path) != "history.db" {
		t.Fatalf("path = %q, want it to end in history.db", path)
	}
	if filepath.Base(filepath.Dir(path)) != "spinoza" {
		t.Fatalf("path = %q, want it beside settings.json and protected.json", path)
	}
}

func TestTheDefaultPathNeedsAConfigDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := DefaultPath()

	if err == nil {
		t.Skip("this platform still names a config directory without HOME")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Fatalf("error = %q, want it to say what it was doing", err.Error())
	}
}

func TestTheDatabaseIsOpenedInWalMode(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if mode := pragma(t, store, "journal_mode"); !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q, want wal; two spinoza windows share this file", mode)
	}
}

func TestAWriterWaitsForAnotherRatherThanFailing(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if timeout := pragma(t, store, "busy_timeout"); timeout != "5000" {
		t.Fatalf("busy_timeout = %q, want 5000; without it a second window's write fails outright", timeout)
	}
}

func TestAWorkingStoreHasNothingToApologiseFor(t *testing.T) {
	if reason := openHistory(t, dbPath(t)).Reason(); reason != "" {
		t.Fatalf("reason = %q, want none from a healthy store", reason)
	}
}

func TestAStoreWithNowhereToWriteStillWorks(t *testing.T) {
	store, err := Open(t.Context(), "")
	if err != nil {
		t.Fatalf("open: %v, want a degraded store rather than a failure", err)
	}
	if store.Reason() == "" {
		t.Fatal("the store did not say why it is not recording")
	}
	record(t, store, entry(p1, noon, "web"))
	if len(recent(t, store, Query{}).Entries) != 0 {
		t.Fatal("a store with no database returned rows")
	}
	if forgetErr := store.Forget(t.Context(), ""); forgetErr != nil {
		t.Fatalf("forget: %v", forgetErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}
}

func TestAReadOnlyDirectoryDoesNotStopSpinoza(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	store, err := Open(t.Context(), filepath.Join(root, "spinoza", "history.db"))

	if err == nil {
		t.Skip("this filesystem let the write through")
	}
	if store == nil {
		t.Fatal("Open returned no store; spinoza must still start without history")
	}
	if !strings.Contains(store.Reason(), "history.db") {
		t.Fatalf("reason = %q, want it to name the file it could not use", store.Reason())
	}
	record(t, store, entry(p1, noon, "web"))
}

func TestACorruptFileDoesNotStopSpinoza(t *testing.T) {
	path := dbPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := Open(t.Context(), path)

	if err == nil {
		t.Fatal("a corrupt file opened cleanly")
	}
	if store.Reason() == "" {
		t.Fatal("the store did not say why it is not recording")
	}
	record(t, store, entry(p1, noon, "web"))
	if len(recent(t, store, Query{}).Entries) != 0 {
		t.Fatal("a corrupt store returned rows")
	}
}

func TestAFileFromANewerSpinozaIsRefusedRatherThanMigrated(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	if _, err := store.writes.ExecContext(t.Context(), "PRAGMA user_version = 99"); err != nil {
		t.Fatalf("bump: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, reopenErr := Open(t.Context(), path)

	if !errors.Is(reopenErr, errFromTheFuture) {
		t.Fatalf("error = %v, want it to refuse a newer schema rather than migrate over it", reopenErr)
	}
	if !strings.Contains(reopened.Reason(), "newer spinoza") {
		t.Fatalf("reason = %q, want it to say the file is from a newer spinoza", reopened.Reason())
	}
}

func TestASchemaThatWillNotApplyIsReported(t *testing.T) {
	saved := migrations
	migrations = []string{"CREATE TABLE ("}
	t.Cleanup(func() { migrations = saved })

	store, err := Open(t.Context(), dbPath(t))

	if err == nil {
		t.Fatal("a broken migration was accepted")
	}
	if store.Reason() == "" {
		t.Fatal("the store did not say why it is not recording")
	}
}

func TestAnAbsentLimitFallsBackToTheDefault(t *testing.T) {
	store := openHistory(t, dbPath(t))
	for at := range defaultLimit + 5 {
		record(t, store, entry(p1, noon.Add(time.Duration(at)*time.Second), "one"))
	}

	page := recent(t, store, Query{})

	if len(page.Entries) != defaultLimit {
		t.Fatalf("returned %d entries with no limit asked for, want the default %d", len(page.Entries), defaultLimit)
	}
}

func TestAnEnormousLimitIsClampedRatherThanObeyed(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "web"))

	page, err := store.Recent(t.Context(), Query{Limit: maxLimit * 100})
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("returned %d entries, want the one that exists", len(page.Entries))
	}
}

func TestWhatWasDoneComesBackUnchanged(t *testing.T) {
	store := openHistory(t, dbPath(t))
	written := entry(p1, noon, "web")
	written.Message = "the api server said no"
	written.Outcome = "refused"
	record(t, store, written)

	page := recent(t, store, Query{})

	if len(page.Entries) != 1 {
		t.Fatalf("found %d entries, want 1", len(page.Entries))
	}
	got := page.Entries[0]
	got.ID = 0
	if got != written {
		t.Fatalf("entry = %+v, want %+v", got, written)
	}
}

func TestTheInstantSurvivesToTheMillisecond(t *testing.T) {
	store := openHistory(t, dbPath(t))
	at := time.Date(2026, 8, 29, 12, 0, 0, 123_000_000, time.UTC)
	record(t, store, entry(p1, at, "web"))

	got := recent(t, store, Query{}).Entries[0].At

	if !got.Equal(at) {
		t.Fatalf("at = %s, want %s; the stored unit must be milliseconds", got, at)
	}
}

func TestAnInstantInAnotherZoneComesBackAsUTC(t *testing.T) {
	store := openHistory(t, dbPath(t))
	zone := time.FixedZone("CEST", 2*60*60)
	at := time.Date(2026, 8, 29, 14, 0, 0, 0, zone)
	record(t, store, entry(p1, at, "web"))

	got := recent(t, store, Query{}).Entries[0].At

	if !got.Equal(at) {
		t.Fatalf("at = %s, want the same instant as %s", got, at)
	}
	if got.Location() != time.UTC {
		t.Fatalf("location = %s, want UTC so two windows agree on what a row says", got.Location())
	}
}

func TestEveryRowCarriesAnIdentityOfItsOwn(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "first"))
	record(t, store, entry(p1, noon, "second"))

	page := recent(t, store, Query{})

	if page.Entries[0].ID == page.Entries[1].ID {
		t.Fatal("two rows share an id; a list has no stable key to draw them by")
	}
	if page.Entries[0].ID == 0 {
		t.Fatal("a row came back without an id")
	}
}

func TestHistorySurvivesARestart(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	record(t, store, entry(p1, noon, "web"))
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	page := recent(t, openHistory(t, path), Query{})

	if len(page.Entries) != 1 {
		t.Fatalf("found %d entries after reopening, want 1", len(page.Entries))
	}
}

func TestTheNewestIsFirst(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "older"))
	record(t, store, entry(p1, noon.Add(time.Minute), "newer"))

	page := recent(t, store, Query{})

	if got := names(page); got[0] != "newer" {
		t.Fatalf("order = %v, want the newest first", got)
	}
}

func TestTwoEntriesAtTheSameInstantKeepAStableOrder(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "first"))
	record(t, store, entry(p1, noon, "second"))

	page := recent(t, store, Query{})

	if got := names(page); got[0] != "second" || got[1] != "first" {
		t.Fatalf("order = %v, want the later write first so rows do not shuffle between polls", got)
	}
}

func TestOneClusterAtATime(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "here"))
	record(t, store, entry(p2, noon, "elsewhere"))

	page := recent(t, store, Query{Cluster: p1})

	if got := names(page); len(got) != 1 || got[0] != "here" {
		t.Fatalf("entries = %v, want only the one from the cluster asked for", got)
	}
}

func TestNoClusterMeansEveryCluster(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "here"))
	record(t, store, entry(p2, noon, "elsewhere"))

	if got := names(recent(t, store, Query{})); len(got) != 2 {
		t.Fatalf("entries = %v, want both clusters", got)
	}
}

func TestAPageThatLeftSomethingOutSaysSo(t *testing.T) {
	store := openHistory(t, dbPath(t))
	for at := range 4 {
		record(t, store, entry(p1, noon.Add(time.Duration(at)*time.Second), "one"))
	}

	page := recent(t, store, Query{Limit: 2})

	if len(page.Entries) != 2 {
		t.Fatalf("returned %d entries, want 2", len(page.Entries))
	}
	if !page.More {
		t.Fatal("the page dropped two entries and did not say so")
	}
}

func TestAPageThatHeldEverythingSaysThatToo(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "web"))

	page := recent(t, store, Query{Limit: 2})

	if page.More {
		t.Fatal("a complete page claimed there was more")
	}
}

func TestForgettingClearsEverything(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "web"))
	record(t, store, entry(p2, noon, "elsewhere"))

	if err := store.Forget(t.Context(), ""); err != nil {
		t.Fatalf("forget: %v", err)
	}

	if got := names(recent(t, store, Query{})); len(got) != 0 {
		t.Fatalf("entries = %v, want none after clearing", got)
	}
}

func TestTwoStoresWritingAtOnceBothLand(t *testing.T) {
	path := dbPath(t)
	one := openHistory(t, path)
	two := openHistory(t, path)
	const each = 25

	var wg sync.WaitGroup
	for _, store := range []*Store{one, two} {
		wg.Add(1)
		go func(into *Store) {
			defer wg.Done()
			for at := range each {
				held := entry(p1, noon.Add(time.Duration(at)*time.Second), "web")
				if err := into.For(p1).Record(context.Background(), held); err != nil {
					t.Errorf("record: %v", err)
					return
				}
			}
		}(store)
	}
	wg.Wait()

	page := recent(t, one, Query{Limit: maxLimit})
	if len(page.Entries) != each*2 {
		t.Fatalf("found %d entries, want %d; a second window's writes were lost", len(page.Entries), each*2)
	}
}

func TestAWindowSeesWhatTheOtherWrote(t *testing.T) {
	path := dbPath(t)
	one := openHistory(t, path)
	two := openHistory(t, path)

	record(t, one, entry(p1, noon, "written by the first"))

	if got := names(recent(t, two, Query{})); len(got) != 1 {
		t.Fatalf("entries = %v, want the other window's write to be visible", got)
	}
}

func TestMigratingTwiceChangesNothing(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	record(t, store, entry(p1, noon, "web"))
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again := openHistory(t, path)

	if len(recent(t, again, Query{}).Entries) != 1 {
		t.Fatal("reopening re-ran the schema and lost what was recorded")
	}
}

func TestAnOlderAuditRowIsKeptWithAnUnknownActor(t *testing.T) {
	path := dbPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db := sql.OpenDB(connector{dsn: path + pragmas})
	for version, statements := range migrations[:len(migrations)-1] {
		if err := apply(t.Context(), db, statements, version+1); err != nil {
			t.Fatalf("apply version %d: %v", version+1, err)
		}
	}
	_, err := db.ExecContext(
		t.Context(), `
INSERT INTO audit (
	cluster, at, verb, api_group, api_version, resource, kind,
	namespace, name, detail, outcome, message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p1, noon.UnixMilli(), "delete", "apps", "v1", "deployments", "Deployment",
		"default", "web", "deleted the deployment", "done", "",
	)
	if err != nil {
		t.Fatalf("insert legacy audit row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	got := recent(t, openHistory(t, path), Query{}).Entries

	if len(got) != 1 {
		t.Fatalf("entries = %d, want the legacy row", len(got))
	}
	if got[0].Actor != "unknown" {
		t.Fatalf("actor = %q, want unknown for a row written before actors were recorded", got[0].Actor)
	}
	if got[0].Name != "web" {
		t.Fatalf("name = %q, want the legacy row to survive", got[0].Name)
	}
}

func TestClosingTwiceIsFine(t *testing.T) {
	store := openHistory(t, dbPath(t))

	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestADatabaseThatStopsAnsweringIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	store.mu.Lock()
	for _, db := range []*sql.DB{store.writes, store.reads} {
		if err := db.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	store.mu.Unlock()

	if err := store.For(p1).Record(t.Context(), entry(p1, noon, "web")); err == nil {
		t.Fatal("recording into a closed database reported success")
	}
	if _, err := store.Recent(t.Context(), Query{}); err == nil {
		t.Fatal("reading a closed database reported success")
	}
	if err := store.Forget(t.Context(), ""); err == nil {
		t.Fatal("clearing a closed database reported success")
	}
}

func TestAnEntryWithNoRoomForItIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if _, err := store.writes.ExecContext(t.Context(), "DROP TABLE audit"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	if recordErr := store.For(p1).Record(t.Context(), entry(p1, noon, "web")); recordErr == nil {
		t.Fatal("recording into a missing table reported success")
	}
	if _, readErr := store.Recent(t.Context(), Query{}); readErr == nil {
		t.Fatal("reading a missing table reported success")
	}
}

func TestAColumnThatChangedShapeIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "web"))
	if _, err := store.writes.ExecContext(t.Context(), "UPDATE audit SET at = 'not a number'"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	_, err := store.Recent(t.Context(), Query{})

	if err == nil {
		t.Fatal("a row that could not be read came back as a silent zero")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Fatalf("error = %q, want it to say what it was doing", err.Error())
	}
}

func TestTheAuditHalfIsBoundedByItsOwnCursor(t *testing.T) {
	store := openHistory(t, dbPath(t))
	for at := range 5 {
		record(t, store, entry(p1, noon.Add(time.Duration(at)*time.Minute), "row-"+strconv.Itoa(at)))
	}

	whole := recent(t, store, Query{})
	if len(whole.Entries) != 5 {
		t.Fatalf("read %d rows", len(whole.Entries))
	}
	boundary := whole.Entries[1].ID

	below := recent(t, store, Query{AfterAction: boundary})

	if len(below.Entries) != 3 {
		t.Fatalf("read %d rows below the cursor, want the three older ones", len(below.Entries))
	}
	for _, one := range below.Entries {
		if one.ID >= boundary {
			t.Fatalf("row %d came back at or above the cursor %d", one.ID, boundary)
		}
	}
}

func TestNoAuditCursorReadsFromTheTop(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon, "only"))

	if len(recent(t, store, Query{AfterAction: 0}).Entries) != 1 {
		t.Fatal("a zero cursor hid the newest row")
	}
}
