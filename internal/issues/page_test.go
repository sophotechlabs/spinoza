package issues

import (
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func rowsAcrossEveryOrderingField() []api.Issue {
	severities := []string{api.SeverityWarning, api.SeverityDegraded, api.SeverityFatal, "info"}
	folded := []int{0, 3, 50}
	when := []string{
		"",
		testNow.Add(-time.Hour).UTC().Format(time.RFC3339),
		testNow.UTC().Format(time.RFC3339),
	}
	clusters := []string{"alpha", "beta"}
	out := make([]api.Issue, 0, len(severities)*len(folded)*len(when)*len(clusters))
	for _, severity := range severities {
		for _, fold := range folded {
			for _, since := range when {
				for _, cluster := range clusters {
					out = append(out, api.Issue{
						ID:       cluster + "/" + severity + "/" + strconv.Itoa(fold) + "/" + since,
						Severity: severity,
						Folded:   fold,
						Since:    since,
						Cluster:  cluster,
					})
				}
			}
		}
	}
	return out
}

func TestTheCursorKeyOrdersRowsTheWayRankDoes(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)

	for at := 1; at < len(rows); at++ {
		before := issueKey(rows[at-1], ByWorst)
		after := issueKey(rows[at], ByWorst)
		if before >= after {
			t.Fatalf(
				"row %d sorts before row %d but its key does not: %q >= %q",
				at-1, at, before, after,
			)
		}
	}
}

func TestPagingWalksTheWholeQueueInOrder(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)

	walked := []api.Issue{}
	cursor := ""
	for range rows {
		page, next := Page(rows, cursorKey(t, cursor, ByWorst), 7, ByWorst)
		walked = append(walked, page...)
		if next == "" {
			break
		}
		cursor = next
	}

	if len(walked) != len(rows) {
		t.Fatalf("walked %d rows, queue holds %d", len(walked), len(rows))
	}
	for at := range rows {
		if walked[at].ID != rows[at].ID {
			t.Fatalf("row %d is %q, ranked order has %q", at, walked[at].ID, rows[at].ID)
		}
	}
}

func TestEveryPageButTheLastIsFull(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)

	cursor := ""
	pages := 0
	for {
		page, next := Page(rows, cursorKey(t, cursor, ByWorst), 7, ByWorst)
		pages++
		if next == "" {
			if len(page) > 7 {
				t.Fatalf("last page carries %d rows, the limit is 7", len(page))
			}
			break
		}
		if len(page) != 7 {
			t.Fatalf("page %d carries %d rows, the limit is 7", pages, len(page))
		}
		cursor = next
	}
	if pages != (len(rows)+6)/7 {
		t.Fatalf("walked %d pages, %d rows at 7 a page needs %d", pages, len(rows), (len(rows)+6)/7)
	}
}

func TestAPageBigEnoughForTheQueueEndsIt(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)

	page, next := Page(rows, "", len(rows)+1, ByWorst)

	if len(page) != len(rows) {
		t.Fatalf("page carries %d of %d rows", len(page), len(rows))
	}
	if next != "" {
		t.Fatalf("a page holding the whole queue still offers %q", next)
	}
}

func TestAnEmptyQueuePagesToNothing(t *testing.T) {
	page, next := Page(nil, "", Shown, ByWorst)

	if len(page) != 0 {
		t.Fatalf("an empty queue paged to %d rows", len(page))
	}
	if next != "" {
		t.Fatalf("an empty queue offers %q", next)
	}
}

func TestAnInvalidPageLimitStillMakesProgress(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)
	for _, limit := range []int{0, -1, -100} {
		page, next := Page(rows, "", limit, ByWorst)
		if len(page) == 0 {
			t.Fatalf("limit %d returned an empty page for a non-empty queue", limit)
		}
		if len(page) > Shown {
			t.Fatalf("limit %d returned %d rows, want at most %d", limit, len(page), Shown)
		}
		if len(rows) > Shown && next == "" {
			t.Fatalf("limit %d lost the continuation", limit)
		}
	}
}

func TestARowClearingBeforeTheNextPageDoesNotSkipItsNeighbour(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)
	first, next := Page(rows, "", 7, ByWorst)
	if next == "" {
		t.Fatal("the fixture is too small to have a second page")
	}

	shorter := slices.Delete(slices.Clone(rows), 0, 1)
	second, _ := Page(shorter, cursorKey(t, next, ByWorst), 7, ByWorst)

	if len(second) == 0 {
		t.Fatal("the second page came back empty")
	}
	if second[0].ID != rows[len(first)].ID {
		t.Fatalf(
			"the row after the boundary is %q, a row clearing above it made it %q",
			rows[len(first)].ID, second[0].ID,
		)
	}
}

func TestGarbageInTheCursorIsRefused(t *testing.T) {
	_, err := DecodeCursor("not base64 at all!!", ByWorst)

	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("error = %v, want invalid cursor", err)
	}
}

func TestIncompleteCursorsAreRefused(t *testing.T) {
	for _, raw := range []string{ByWorst, "\x00key", ByWorst + "\x00"} {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
		_, err := DecodeCursor(cursor, ByWorst)
		if !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor for %q returned %v, want invalid cursor", raw, err)
		}
	}
}

func cursorKey(t *testing.T, cursor, order string) string {
	t.Helper()
	key, err := DecodeCursor(cursor, order)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	return key
}

func TestAnEmptyCursorEncodesToNothing(t *testing.T) {
	if EncodeCursor("", ByWorst) != "" {
		t.Fatalf("an empty key encoded to %q", EncodeCursor("", ByWorst))
	}
	if cursorKey(t, "", ByWorst) != "" {
		t.Fatal("an empty cursor decoded to something")
	}
}

func TestACursorFromAnotherOrderIsRefused(t *testing.T) {
	rows := rowsAcrossEveryOrderingField()
	Rank(rows, ByWorst)
	_, next := Page(rows, "", 3, ByWorst)
	if next == "" {
		t.Fatal("the fixture produced no second page")
	}

	_, err := DecodeCursor(next, ByNewest)

	if !errors.Is(err, ErrCursorOrder) {
		t.Fatalf("error = %v, want cursor order mismatch", err)
	}
}

func TestPageSizeFallsBackToWhatOnePageShows(t *testing.T) {
	if PageSize(0) != Shown {
		t.Fatalf("an unset size gave %d, one page shows %d", PageSize(0), Shown)
	}
	if PageSize(-4) != Shown {
		t.Fatalf("a negative size gave %d, one page shows %d", PageSize(-4), Shown)
	}
	if PageSize(9) != 9 {
		t.Fatalf("a size of 9 gave %d", PageSize(9))
	}
}

func TestCountDownStaysInsideItsWidth(t *testing.T) {
	cases := []struct {
		value int
		want  string
	}{
		{value: 0, want: "999"},
		{value: 1, want: "998"},
		{value: 999, want: "000"},
		{value: -7, want: "999"},
		{value: 4000, want: "000"},
	}
	for _, one := range cases {
		got := countDown(one.value, 999)
		if got != one.want {
			t.Fatalf("countDown(%d, 999) = %q, want %q", one.value, got, one.want)
		}
	}
}

func TestCountUpStaysInsideItsWidth(t *testing.T) {
	cases := []struct {
		value int
		want  string
	}{
		{value: 0, want: "000"},
		{value: 1, want: "001"},
		{value: 999, want: "999"},
		{value: -7, want: "000"},
		{value: 4000, want: "999"},
	}
	for _, one := range cases {
		got := countUp(one.value, 999)
		if got != one.want {
			t.Fatalf("countUp(%d, 999) = %q, want %q", one.value, got, one.want)
		}
	}
}

func TestATimeNobodyStampedCountsAsTheOldest(t *testing.T) {
	if secondsOf(time.Time{}) != 0 {
		t.Fatalf("an unstamped time counted as %d seconds", secondsOf(time.Time{}))
	}
	stamped := time.Unix(1700000000, 0)
	if secondsOf(stamped) != 1700000000 {
		t.Fatalf("a stamped time counted as %d seconds", secondsOf(stamped))
	}
}

func TestAFoldCountPastTheCeilingStillOutranksASmallOne(t *testing.T) {
	huge := api.Issue{ID: "huge", Severity: api.SeverityWarning, Folded: foldCeiling + 10}
	modest := api.Issue{ID: "modest", Severity: api.SeverityWarning, Folded: 1}

	if issueKey(huge, ByWorst) >= issueKey(modest, ByWorst) {
		t.Fatalf("a fold count past the ceiling wrapped and sorted below a fold of 1")
	}
}

func TestNewestFirstOrdersByWhenNotBySeverity(t *testing.T) {
	rows := []api.Issue{
		{ID: "old-fatal", Severity: api.SeverityFatal, Since: "2026-08-01T00:00:00Z"},
		{ID: "new-warning", Severity: api.SeverityWarning, Since: "2026-08-29T00:00:00Z"},
	}

	Rank(rows, ByNewest)

	if rows[0].ID != "new-warning" {
		t.Fatalf("newest first put %q on top", rows[0].ID)
	}
}

func TestOldestFirstIsTheOtherWayRound(t *testing.T) {
	rows := []api.Issue{
		{ID: "new-warning", Severity: api.SeverityWarning, Since: "2026-08-29T00:00:00Z"},
		{ID: "old-fatal", Severity: api.SeverityFatal, Since: "2026-08-01T00:00:00Z"},
	}

	Rank(rows, ByOldest)

	if rows[0].ID != "old-fatal" {
		t.Fatalf("oldest first put %q on top", rows[0].ID)
	}
}

func TestEveryOrderWalksItsWholeQueueWithoutRepeating(t *testing.T) {
	for _, order := range []string{ByWorst, ByNewest, ByOldest} {
		t.Run(order, func(t *testing.T) {
			rows := rowsAcrossEveryOrderingField()
			Rank(rows, order)

			for at := 1; at < len(rows); at++ {
				if issueKey(rows[at-1], order) >= issueKey(rows[at], order) {
					t.Fatalf("row %d sorts before row %d but its key does not", at-1, at)
				}
			}

			seen := map[string]bool{}
			cursor := ""
			for range rows {
				page, next := Page(rows, cursorKey(t, cursor, order), 7, order)
				for _, row := range page {
					if seen[row.ID] {
						t.Fatalf("%s came back twice", row.ID)
					}
					seen[row.ID] = true
				}
				if next == "" {
					break
				}
				cursor = next
			}
			if len(seen) != len(rows) {
				t.Fatalf("walked %d of %d rows", len(seen), len(rows))
			}
		})
	}
}

func TestAnUnknownOrderFallsBackToWorstFirst(t *testing.T) {
	for _, asked := range []string{"", "sideways", "WORST"} {
		if OrderOf(asked) != ByWorst {
			t.Fatalf("%q read as %q", asked, OrderOf(asked))
		}
	}
	if OrderOf(ByNewest) != ByNewest || OrderOf(ByOldest) != ByOldest {
		t.Fatal("a date order was not taken")
	}
}
