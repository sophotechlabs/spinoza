package store

import (
	"testing"
)

func TestAuditRowsOlderThanTheWindowArePruned(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon.AddDate(0, 0, -120), "ancient"))
	record(t, store, entry(p1, noon, "today"))

	err := store.PruneAudit(t.Context(), Retention{Days: 90}, noon)
	if err != nil {
		t.Fatalf("prune audit: %v", err)
	}

	found := recent(t, store, Query{})
	if len(found.Entries) != 1 || found.Entries[0].Name != "today" {
		t.Fatalf("what survived the prune was %+v", found.Entries)
	}
}

func TestTheAuditRowCapHoldsWithoutADayWindow(t *testing.T) {
	store := openHistory(t, dbPath(t))
	for at := range 10 {
		record(t, store, entry(p1, noon, "web-"+string(rune('a'+at))))
	}

	err := store.PruneAudit(t.Context(), Retention{Rows: 4}, noon)
	if err != nil {
		t.Fatalf("prune audit: %v", err)
	}

	found := recent(t, store, Query{})
	if len(found.Entries) != 4 {
		t.Fatalf("wanted four rows left, got %d", len(found.Entries))
	}
	if found.Entries[0].Name != "web-j" {
		t.Fatalf("the newest kept was %s", found.Entries[0].Name)
	}
}

func TestPruningTheAuditLeavesTheTimelineAlone(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon.AddDate(0, 0, -120), "old-audit"))
	noteOne(t, store, p1, change(noon.AddDate(0, 0, -120), "old-change"))

	err := store.PruneAudit(t.Context(), Retention{Days: 90, Rows: 1}, noon)
	if err != nil {
		t.Fatalf("prune audit: %v", err)
	}

	audit := recent(t, store, Query{})
	if len(audit.Entries) != 0 {
		t.Fatalf("the audit row survived its own window: %+v", audit.Entries)
	}
	changes, readErr := store.Changed(t.Context(), Query{})
	if readErr != nil {
		t.Fatalf("changed: %v", readErr)
	}
	if len(changes.Rows) != 1 {
		t.Fatalf("pruning the audit took %d timeline rows with it", 1-len(changes.Rows))
	}
}

func TestPruningTheTimelineLeavesTheAuditAlone(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon.AddDate(0, 0, -120), "old-audit"))
	noteOne(t, store, p1, change(noon.AddDate(0, 0, -120), "old-change"))

	err := store.Prune(t.Context(), Retention{Days: 7, Rows: 1}, noon)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	audit := recent(t, store, Query{})
	if len(audit.Entries) != 1 {
		t.Fatalf("pruning the timeline took the audit row with it")
	}
}

func TestPruningAnEmptyAuditIsFine(t *testing.T) {
	store := openHistory(t, dbPath(t))

	err := store.PruneAudit(t.Context(), Retention{Days: 90, Rows: 10}, noon)
	if err != nil {
		t.Fatalf("prune audit: %v", err)
	}
}

func TestPruningTheAuditWithNoRetentionAskedForKeepsEverything(t *testing.T) {
	store := openHistory(t, dbPath(t))
	record(t, store, entry(p1, noon.AddDate(0, 0, -400), "ancient"))

	err := store.PruneAudit(t.Context(), Retention{}, noon)
	if err != nil {
		t.Fatalf("prune audit: %v", err)
	}

	found := recent(t, store, Query{})
	if len(found.Entries) != 1 {
		t.Fatalf("asking for no retention still dropped %d rows", 1-len(found.Entries))
	}
}
