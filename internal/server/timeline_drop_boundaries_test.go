package server

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func TestTimelineDropAccountingCrossesTheReportingBoundary(t *testing.T) {
	held := &recording{queue: make(chan store.Change, 1)}
	held.Note(resources.Note{Name: "kept"})

	for index := range timelineDropWarn {
		held.Note(resources.Note{Name: "dropped"})
		if got := held.dropped.Load(); got != int64(index+1) {
			t.Fatalf("drop %d recorded as %d", index+1, got)
		}
	}

	queued := <-held.queue
	if queued.Name != "kept" {
		t.Fatalf("queued change = %q, want the first change retained", queued.Name)
	}
}

func TestPruningWithoutARecorderIsQuiet(t *testing.T) {
	srv := &Server{}

	srv.pruneTimeline(t.Context())
}
