package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func runStore(t *testing.T) *Store {
	t.Helper()
	held, err := Open(t.Context(), filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	return held
}

func aRun(at time.Time, findings int) Run {
	return Run{At: at, Findings: findings, Fresh: 1, Cleared: 2, Scanned: 42}
}

func TestARunReadsBackTheWayItWasRecorded(t *testing.T) {
	held := runStore(t)
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	if err := held.RecordRun(t.Context(), "kind-spinoza", aRun(at, 4)); err != nil {
		t.Fatalf("record: %v", err)
	}

	back, err := held.Runs(t.Context(), "kind-spinoza", 0)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(back) != 1 {
		t.Fatalf("runs = %d, want 1", len(back))
	}
	one := back[0]
	if one.Findings != 4 || one.Fresh != 1 || one.Cleared != 2 || one.Scanned != 42 {
		t.Fatalf("run = %+v", one)
	}
	if !one.At.Equal(at) {
		t.Fatalf("at = %s, want %s", one.At, at)
	}
	if one.ID == 0 {
		t.Fatal("the run came back with no id")
	}
}

func TestRunsComeBackNewestFirstAndOnlyForTheClusterAsked(t *testing.T) {
	held := runStore(t)
	for at, one := range []struct {
		cluster string
		run     Run
	}{
		{cluster: "one", run: aRun(noon.Add(-2*time.Hour), 1)},
		{cluster: "one", run: aRun(noon, 3)},
		{cluster: "two", run: aRun(noon.Add(-time.Hour), 2)},
	} {
		if err := held.RecordRun(t.Context(), one.cluster, one.run); err != nil {
			t.Fatalf("record %d: %v", at, err)
		}
	}

	onlyOne, err := held.Runs(t.Context(), "one", 0)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(onlyOne) != 2 {
		t.Fatalf("runs for one = %d, want 2", len(onlyOne))
	}
	if onlyOne[0].Findings != 3 {
		t.Fatalf("first run = %+v, want the newest", onlyOne[0])
	}

	everything, allErr := held.Runs(t.Context(), "", 0)
	if allErr != nil {
		t.Fatalf("runs: %v", allErr)
	}
	if len(everything) != 3 {
		t.Fatalf("runs across every cluster = %d, want 3", len(everything))
	}
}

func TestAskingForMoreRunsThanAreKeptGivesWhatIsKept(t *testing.T) {
	held := runStore(t)
	for at := range 5 {
		if err := held.RecordRun(t.Context(), "one", aRun(noon.Add(time.Duration(at)*time.Minute), at)); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	for _, asked := range []int{-1, 0, runsKept + 1} {
		back, err := held.Runs(t.Context(), "one", asked)
		if err != nil {
			t.Fatalf("runs(%d): %v", asked, err)
		}
		if len(back) != 5 {
			t.Fatalf("runs(%d) = %d, want every one of 5", asked, len(back))
		}
	}

	two, err := held.Runs(t.Context(), "one", 2)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(two) != 2 {
		t.Fatalf("runs(2) = %d", len(two))
	}
}

func TestRunsArePrunedByAgeAndByCount(t *testing.T) {
	held := runStore(t)
	if err := held.RecordRun(t.Context(), "one", aRun(noon.AddDate(0, 0, -30), 1)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := held.RecordRun(t.Context(), "one", aRun(noon, 2)); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := held.PruneRuns(t.Context(), Retention{Days: 7}, noon); err != nil {
		t.Fatalf("prune: %v", err)
	}

	back, err := held.Runs(t.Context(), "one", 0)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(back) != 1 || back[0].Findings != 2 {
		t.Fatalf("runs after pruning = %+v, want only the recent one", back)
	}

	if err := held.PruneRuns(t.Context(), Retention{Rows: 0}, noon); err != nil {
		t.Fatalf("prune with no cap: %v", err)
	}
	kept, keptErr := held.Runs(t.Context(), "one", 0)
	if keptErr != nil {
		t.Fatalf("runs: %v", keptErr)
	}
	if len(kept) != 1 {
		t.Fatalf("a prune with no window removed something: %d left", len(kept))
	}
}

func TestAStoreWithNowhereToWriteKeepsNoRunsAndSaysSo(t *testing.T) {
	held := &Store{}

	if err := held.RecordRun(t.Context(), "one", aRun(time.Now(), 1)); err != nil {
		t.Fatalf("record: %v", err)
	}
	back, err := held.Runs(t.Context(), "one", 0)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(back) != 0 {
		t.Fatalf("runs = %d, want none", len(back))
	}
	if pruneErr := held.PruneRuns(t.Context(), Retention{Days: 1}, time.Now()); pruneErr != nil {
		t.Fatalf("prune: %v", pruneErr)
	}
}

func TestAClosedStoreKeepsNoRunsRatherThanBreaking(t *testing.T) {
	held := runStore(t)
	if err := held.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := held.RecordRun(context.Background(), "one", aRun(time.Now(), 1)); err != nil {
		t.Fatalf("a closed store returned an error rather than keeping nothing: %v", err)
	}
	back, readErr := held.Runs(context.Background(), "one", 0)
	if readErr != nil {
		t.Fatalf("a closed store returned an error rather than nothing: %v", readErr)
	}
	if len(back) != 0 {
		t.Fatalf("a closed store answered with %d runs", len(back))
	}
}

func TestARunThatCannotBeWrittenIsReported(t *testing.T) {
	held := runStore(t)
	canceled, stop := context.WithCancel(t.Context())
	stop()

	err := held.RecordRun(canceled, "one", aRun(time.Now(), 1))

	if err == nil {
		t.Fatal("a write against a canceled request came back clean")
	}
	if _, readErr := held.Runs(canceled, "one", 0); readErr == nil {
		t.Fatal("a read against a canceled request came back clean")
	}
}
