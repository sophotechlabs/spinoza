package server

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestIdenticalHistoryEntriesCompareEqual(t *testing.T) {
	entry := api.HistoryEntry{
		ID:     17,
		At:     "2026-09-02T20:00:00Z",
		Source: api.HistoryChange,
	}

	if got := newestFirst(entry, entry); got != 0 {
		t.Fatalf("comparison = %d, want identical entries equal", got)
	}
}

func TestHistoryIDsBreakOtherwiseEqualTimestampsNewestFirst(t *testing.T) {
	older := api.HistoryEntry{ID: 17, At: "2026-09-02T20:00:00Z", Source: api.HistoryChange}
	newer := older
	newer.ID = 18

	if got := newestFirst(newer, older); got >= 0 {
		t.Fatalf("newer against older = %d, want the newer id first", got)
	}
	if got := newestFirst(older, newer); got <= 0 {
		t.Fatalf("older against newer = %d, want the older id last", got)
	}
}
