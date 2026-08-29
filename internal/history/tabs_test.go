package history

import (
	"testing"
	"time"
)

func tab(id, name string, seen time.Time) Tab {
	return Tab{ID: id, Context: name, Kubeconfig: "/work.yaml", Seen: seen}
}

func remember(t *testing.T, store *Store, held Tab) {
	t.Helper()
	if err := store.Tabs().Remember(t.Context(), held); err != nil {
		t.Fatalf("remember: %v", err)
	}
}

func allTabs(t *testing.T, store *Store) []Tab {
	t.Helper()
	found, err := store.Tabs().All(t.Context())
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	return found
}

func tabIDs(held []Tab) []string {
	out := make([]string, 0, len(held))
	for _, one := range held {
		out = append(out, one.ID)
	}
	return out
}

func TestAClusterThatWasOpenIsRememberedForNextTime(t *testing.T) {
	store := openHistory(t, dbPath(t))

	remember(t, store, tab(p1, "p-mk1", noon))

	found := allTabs(t, store)
	if len(found) != 1 {
		t.Fatalf("remembered %d tabs, want the one that was open", len(found))
	}
	if found[0].Context != "p-mk1" || found[0].Kubeconfig != "/work.yaml" {
		t.Fatalf("tab = %+v, want the context and kubeconfig that opened it", found[0])
	}
}

func TestTheTimeAClusterWasOpenedSurvivesTheRoundTrip(t *testing.T) {
	store := openHistory(t, dbPath(t))

	remember(t, store, tab(p1, "p-mk1", noon))

	if seen := allTabs(t, store)[0].Seen; !seen.Equal(noon) {
		t.Fatalf("seen = %s, want %s", seen, noon)
	}
}

func TestOpeningTheSameClusterAgainDoesNotDoubleTheTab(t *testing.T) {
	store := openHistory(t, dbPath(t))
	remember(t, store, tab(p1, "p-mk1", noon))

	remember(t, store, tab(p1, "p-mk1-admin", noon.Add(time.Hour)))

	found := allTabs(t, store)
	if len(found) != 1 {
		t.Fatalf("remembered %d tabs, want one row per cluster", len(found))
	}
	if found[0].Context != "p-mk1-admin" {
		t.Fatalf("context = %q, want the one that opened it last", found[0].Context)
	}
}

func TestTabsComeBackInTheOrderTheyWereOpened(t *testing.T) {
	store := openHistory(t, dbPath(t))
	remember(t, store, tab(p2, "p-mk2", noon.Add(time.Hour)))
	remember(t, store, tab(p1, "p-mk1", noon))

	if got := tabIDs(allTabs(t, store)); got[0] != p1 || got[1] != p2 {
		t.Fatalf("tabs = %v, want the strip in the order they were opened", got)
	}
}

func TestClosingATabStopsItComingBack(t *testing.T) {
	store := openHistory(t, dbPath(t))
	remember(t, store, tab(p1, "p-mk1", noon))
	remember(t, store, tab(p2, "p-mk2", noon))

	if err := store.Tabs().Forget(t.Context(), p1); err != nil {
		t.Fatalf("forget: %v", err)
	}

	if got := tabIDs(allTabs(t, store)); len(got) != 1 || got[0] != p2 {
		t.Fatalf("tabs = %v, want only the one that stayed open", got)
	}
}

func TestAStoreWithNowhereToWriteRemembersNoTabs(t *testing.T) {
	store, err := Open(t.Context(), "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	remember(t, store, tab(p1, "p-mk1", noon))

	if found := allTabs(t, store); len(found) != 0 {
		t.Fatalf("tabs = %v, want none from a store with no database", found)
	}
	if forgetErr := store.Tabs().Forget(t.Context(), p1); forgetErr != nil {
		t.Fatalf("forget: %v", forgetErr)
	}
}

func TestATabTableThatIsGoneIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	if _, err := store.writes.ExecContext(t.Context(), "DROP TABLE clusters"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	if err := store.Tabs().Remember(t.Context(), tab(p1, "p-mk1", noon)); err == nil {
		t.Fatal("remembering into a missing table reported success")
	}
	if _, err := store.Tabs().All(t.Context()); err == nil {
		t.Fatal("reading a missing table reported success")
	}
	if err := store.Tabs().Forget(t.Context(), p1); err == nil {
		t.Fatal("forgetting from a missing table reported success")
	}
}

func TestATabRowThatChangedShapeIsReported(t *testing.T) {
	store := openHistory(t, dbPath(t))
	remember(t, store, tab(p1, "p-mk1", noon))
	if _, err := store.writes.ExecContext(t.Context(), "UPDATE clusters SET seen = 'not a number'"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	if _, err := store.Tabs().All(t.Context()); err == nil {
		t.Fatal("a row that could not be read came back as a silent zero")
	}
}

func TestTheTabsOfAnOlderSpinozaSurviveTheMigration(t *testing.T) {
	path := dbPath(t)
	store := openHistory(t, path)
	record(t, store, entry(p1, noon, "web"))
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened := openHistory(t, path)

	if len(recent(t, reopened, Query{}).Entries) != 1 {
		t.Fatal("the audit did not survive the migration that added the tabs")
	}
	if found := allTabs(t, reopened); len(found) != 0 {
		t.Fatalf("tabs = %v, want none before anything is opened", found)
	}
}
