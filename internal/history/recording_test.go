package history

import "testing"

func TestATabRemembersWhatItRecords(t *testing.T) {
	store := openHistory(t, dbPath(t))
	tabs := store.Tabs()
	err := tabs.Remember(t.Context(), Tab{ID: p1, Context: "p-mk1", Seen: noon})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}

	recordErr := tabs.Recording(t.Context(), p1, "workloads")
	if recordErr != nil {
		t.Fatalf("recording: %v", recordErr)
	}

	found, readErr := tabs.All(t.Context())
	if readErr != nil {
		t.Fatalf("all: %v", readErr)
	}
	if len(found) != 1 || found[0].Timeline != "workloads" {
		t.Fatalf("the tab came back as %+v", found)
	}
}

func TestATabRecordsNothingUntilItIsAskedTo(t *testing.T) {
	store := openHistory(t, dbPath(t))
	tabs := store.Tabs()
	err := tabs.Remember(t.Context(), Tab{ID: p1, Context: "p-mk1", Seen: noon})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}

	found, readErr := tabs.All(t.Context())
	if readErr != nil {
		t.Fatalf("all: %v", readErr)
	}
	if found[0].Timeline != "" {
		t.Fatalf("a new tab was recording %q", found[0].Timeline)
	}
}

func TestReopeningATabKeepsWhatItRecords(t *testing.T) {
	store := openHistory(t, dbPath(t))
	tabs := store.Tabs()
	err := tabs.Remember(t.Context(), Tab{ID: p1, Context: "p-mk1", Seen: noon})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	recordErr := tabs.Recording(t.Context(), p1, "wide")
	if recordErr != nil {
		t.Fatalf("recording: %v", recordErr)
	}

	againErr := tabs.Remember(t.Context(), Tab{ID: p1, Context: "p-mk1", Seen: noon.Add(1)})
	if againErr != nil {
		t.Fatalf("remember: %v", againErr)
	}

	found, readErr := tabs.All(t.Context())
	if readErr != nil {
		t.Fatalf("all: %v", readErr)
	}
	if found[0].Timeline != "wide" {
		t.Fatalf("reopening lost the setting: %+v", found[0])
	}
}

func TestRecordingOnAStoreThatCouldNotOpenIsQuiet(t *testing.T) {
	store := unavailable("nowhere to keep history")

	err := store.Tabs().Recording(t.Context(), p1, "workloads")
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
}
