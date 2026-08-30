package store

import (
	"strings"
	"testing"
	"time"
)

func change(at time.Time, name string) Change {
	return Change{
		At:        at,
		Verb:      Changed,
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Kind:      "Deployment",
		Namespace: "default",
		Name:      name,
		UID:       "uid-" + name,
		Cells:     []string{"1/1", "Running"},
	}
}

func noteOne(t *testing.T, store *Store, cluster string, one Change) {
	t.Helper()
	err := store.Timeline(cluster).Note(t.Context(), []Change{one})
	if err != nil {
		t.Fatalf("note: %v", err)
	}
}

func TestAChangeComesBackTheWayItWentIn(t *testing.T) {
	store := openHistory(t, dbPath(t))

	noteOne(t, store, p1, change(noon, "web"))

	found, err := store.Changed(t.Context(), Query{})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows) != 1 {
		t.Fatalf("wanted one change, got %d", len(found.Rows))
	}
	row := found.Rows[0]
	if row.Name != "web" || row.Kind != "Deployment" || row.UID != "uid-web" {
		t.Fatalf("the change came back as %+v", row)
	}
	if !row.At.Equal(noon) {
		t.Fatalf("wanted %s, got %s", noon, row.At)
	}
	if len(row.Cells) != 2 || row.Cells[1] != "Running" {
		t.Fatalf("the cells came back as %v", row.Cells)
	}
}

func TestAChangeIsStampedWithTheClusterItIsOn(t *testing.T) {
	store := openHistory(t, dbPath(t))

	noteOne(t, store, p1, change(noon, "web"))
	noteOne(t, store, p2, change(noon, "api"))

	found, err := store.Changed(t.Context(), Query{Cluster: p2})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows) != 1 {
		t.Fatalf("wanted one change, got %d", len(found.Rows))
	}
	if found.Rows[0].Name != "api" || found.Rows[0].Cluster != p2 {
		t.Fatalf("the change came back as %+v", found.Rows[0])
	}
}

func TestTheNewestChangeIsFirst(t *testing.T) {
	store := openHistory(t, dbPath(t))

	noteOne(t, store, p1, change(noon.Add(-time.Hour), "old"))
	noteOne(t, store, p1, change(noon, "new"))

	found, err := store.Changed(t.Context(), Query{})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if found.Rows[0].Name != "new" || found.Rows[1].Name != "old" {
		t.Fatalf("the order came back as %s then %s", found.Rows[0].Name, found.Rows[1].Name)
	}
}

func TestAWholeBatchOfChangesLandsAtOnce(t *testing.T) {
	store := openHistory(t, dbPath(t))

	err := store.Timeline(p1).Note(t.Context(), []Change{
		change(noon, "one"),
		change(noon, "two"),
		change(noon, "three"),
	})
	if err != nil {
		t.Fatalf("note: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 3 {
		t.Fatalf("wanted three changes, got %d", len(found.Rows))
	}
}

func TestNotingNothingIsNotAWrite(t *testing.T) {
	store := openHistory(t, dbPath(t))

	err := store.Timeline(p1).Note(t.Context(), nil)
	if err != nil {
		t.Fatalf("note: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 0 {
		t.Fatalf("wanted no changes, got %d", len(found.Rows))
	}
}

func TestTheChangeQuerySaysWhenThereAreMore(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon, "one"))
	noteOne(t, store, p1, change(noon, "two"))

	found, err := store.Changed(t.Context(), Query{Limit: 1})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows) != 1 {
		t.Fatalf("wanted one change, got %d", len(found.Rows))
	}
	if !found.More {
		t.Fatal("wanted to be told there are more")
	}
}

func TestChangesOlderThanTheWindowArePruned(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon.AddDate(0, 0, -30), "ancient"))
	noteOne(t, store, p1, change(noon, "today"))

	err := store.Prune(t.Context(), Retention{Days: 7}, noon)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 1 || found.Rows[0].Name != "today" {
		t.Fatalf("what survived the prune was %+v", found.Rows)
	}
}

func TestTheRowCapIsWhatHoldsOnAClusterNobodyMeasured(t *testing.T) {
	store := openHistory(t, dbPath(t))
	for at := range 10 {
		noteOne(t, store, p1, change(noon, "web-"+string(rune('a'+at))))
	}

	err := store.Prune(t.Context(), Retention{Rows: 4}, noon)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 4 {
		t.Fatalf("wanted four changes left, got %d", len(found.Rows))
	}
	if found.Rows[0].Name != "web-j" {
		t.Fatalf("the newest kept was %s", found.Rows[0].Name)
	}
}

func TestPruningAnEmptyTimelineIsFine(t *testing.T) {
	store := openHistory(t, dbPath(t))

	err := store.Prune(t.Context(), Retention{Days: 7, Rows: 10}, noon)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
}

func TestPruningWithNoRetentionAskedForKeepsEverything(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon.AddDate(0, 0, -400), "ancient"))

	err := store.Prune(t.Context(), Retention{}, noon)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 1 {
		t.Fatalf("wanted the row kept, got %d", len(found.Rows))
	}
}

func TestClearingAClustersHistoryTakesItsChangesToo(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon, "web"))
	noteOne(t, store, p2, change(noon, "api"))

	err := store.Forget(t.Context(), p1)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}

	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 1 || found.Rows[0].Cluster != p2 {
		t.Fatalf("what survived was %+v", found.Rows)
	}
}

func TestAStoreThatCouldNotOpenSaysNothingRatherThanFailing(t *testing.T) {
	store := unavailable("nowhere to keep history")

	err := store.Timeline(p1).Note(t.Context(), []Change{change(noon, "web")})
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	found, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(found.Rows) != 0 {
		t.Fatalf("wanted nothing back, got %d", len(found.Rows))
	}
	pruneErr := store.Prune(t.Context(), Retention{Days: 1, Rows: 1}, noon)
	if pruneErr != nil {
		t.Fatalf("prune: %v", pruneErr)
	}
}

func TestCellsThatCannotBeReadComeBackEmpty(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, Change{At: noon, Verb: Added, Name: "web", Cells: nil})

	found, err := store.Changed(t.Context(), Query{})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows[0].Cells) != 0 {
		t.Fatalf("wanted no cells, got %v", found.Rows[0].Cells)
	}
}

func TestAChangeWithNoRoomForItIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if _, err := store.writes.ExecContext(t.Context(), "DROP TABLE changes"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	noteErr := store.Timeline(p1).Note(t.Context(), []Change{change(noon, "web")})
	if noteErr == nil {
		t.Fatal("noting into a missing table reported success")
	}
	if _, readErr := store.Changed(t.Context(), Query{}); readErr == nil {
		t.Fatal("reading a missing table reported success")
	}
	if pruneErr := store.Prune(t.Context(), Retention{Days: 1}, noon); pruneErr == nil {
		t.Fatal("trimming a missing table reported success")
	}
}

func TestTrimmingByRowsAgainstAMissingTableIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if _, err := store.writes.ExecContext(t.Context(), "DROP TABLE changes"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	err := store.Prune(t.Context(), Retention{Rows: 10}, noon)

	if err == nil {
		t.Fatal("trimming a missing table reported success")
	}
}

func TestAChangeColumnThatChangedShapeIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon, "web"))
	if _, err := store.writes.ExecContext(t.Context(), "UPDATE changes SET at = 'not a number'"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	_, err := store.Changed(t.Context(), Query{})

	if err == nil {
		t.Fatal("a change that could not be read came back as a silent zero")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Fatalf("error = %q, want it to say what it was doing", err.Error())
	}
}

func TestNotingIntoAClosedStoreIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if err := store.writes.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	noteErr := store.Timeline(p1).Note(t.Context(), []Change{change(noon, "web")})

	if noteErr == nil {
		t.Fatal("noting into a closed database reported success")
	}
}

func TestCellsThatAreNotAListComeBackEmpty(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon, "web"))
	if _, err := store.writes.ExecContext(t.Context(), "UPDATE changes SET cells = 'not json'"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	found, err := store.Changed(t.Context(), Query{})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows[0].Cells) != 0 {
		t.Fatalf("wanted no cells, got %v", found.Rows[0].Cells)
	}
}

func TestTheLimitAStoreWillHonourIsSayable(t *testing.T) {
	if Limit(0) != defaultLimit {
		t.Fatalf("Limit(0) = %d", Limit(0))
	}
	if Limit(maxLimit+1) != maxLimit {
		t.Fatalf("Limit above the cap = %d", Limit(maxLimit+1))
	}
	if Limit(5) != 5 {
		t.Fatalf("Limit(5) = %d", Limit(5))
	}
}

func TestRecolouringATabThatIsNotThereIsQuiet(t *testing.T) {
	store := openHistory(t, dbPath(t))

	err := store.Tabs().Recolor(t.Context(), p1, 3)
	if err != nil {
		t.Fatalf("recolor: %v", err)
	}
	closed := unavailable("nowhere to keep history")
	if quietErr := closed.Tabs().Recolor(t.Context(), p1, 3); quietErr != nil {
		t.Fatalf("recolor: %v", quietErr)
	}
}

func TestRecordingIntoAMissingTableIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if _, err := store.writes.ExecContext(t.Context(), "DROP TABLE clusters"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	err := store.Tabs().Recording(t.Context(), p1, "workloads")

	if err == nil {
		t.Fatal("recording against a missing table reported success")
	}
}

func TestAChangeRemembersWhatItMovedFrom(t *testing.T) {
	store := openHistory(t, dbPath(t))
	one := change(noon, "web")
	one.Was = []string{"2/2", "Running"}

	noteOne(t, store, p1, one)

	found, err := store.Changed(t.Context(), Query{})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(found.Rows[0].Was) != 2 || found.Rows[0].Was[1] != "Running" {
		t.Fatalf("was = %v", found.Rows[0].Was)
	}
}

func TestAPageOfChangesContinuesBelowTheCursor(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon.Add(-2*time.Minute), "oldest"))
	noteOne(t, store, p1, change(noon.Add(-time.Minute), "middle"))
	noteOne(t, store, p1, change(noon, "newest"))
	first, err := store.Changed(t.Context(), Query{Limit: 1})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}

	next, nextErr := store.Changed(t.Context(), Query{Limit: 1, After: first.Rows[0].ID})

	if nextErr != nil {
		t.Fatalf("changed: %v", nextErr)
	}
	if next.Rows[0].Name != "middle" {
		t.Fatalf("the next page started at %s", next.Rows[0].Name)
	}
}

func TestTheLastPageSaysThereIsNoMore(t *testing.T) {
	store := openHistory(t, dbPath(t))
	noteOne(t, store, p1, change(noon, "only"))

	found, err := store.Changed(t.Context(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if found.More {
		t.Fatal("a page that held everything said there was more")
	}
}
