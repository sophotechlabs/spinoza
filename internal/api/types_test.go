package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func keysOf(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fields := map[string]json.RawMessage{}
	unmarshalErr := json.Unmarshal(raw, &fields)
	if unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	return fields
}

func TestSnapshotKeepsRowsWhenThereAreNone(t *testing.T) {
	fields := keysOf(t, Snapshot{
		Type:       "snapshot",
		SubID:      "main",
		Columns:    []Column{},
		Namespaced: true,
		Rows:       []Row{},
	})

	if string(fields["rows"]) != "[]" {
		t.Fatalf("rows = %s, want an empty array the browser can iterate", fields["rows"])
	}
	if string(fields["columns"]) != "[]" {
		t.Fatalf("columns = %s, want an empty array", fields["columns"])
	}
}

func TestSnapshotKeepsNamespacedWhenFalse(t *testing.T) {
	fields := keysOf(t, Snapshot{
		Type:       "snapshot",
		SubID:      "main",
		Columns:    []Column{},
		Namespaced: false,
		Rows:       []Row{},
	})

	value, present := fields["namespaced"]
	if !present {
		t.Fatal("namespaced was dropped for a cluster-scoped resource")
	}
	if string(value) != "false" {
		t.Fatalf("namespaced = %s, want false", value)
	}
}

func TestSnapshotCarriesEveryFieldTheBrowserReads(t *testing.T) {
	fields := keysOf(t, Snapshot{})

	for _, name := range []string{"type", "subId", "columns", "namespaced", "rows"} {
		_, present := fields[name]
		if !present {
			t.Fatalf("%q is missing from an empty snapshot; the browser reads it unconditionally", name)
		}
	}
}

func TestSnapshotSurvivesARoundTrip(t *testing.T) {
	raw, err := json.Marshal(Snapshot{
		Type:       "snapshot",
		SubID:      "main",
		Columns:    []Column{{Name: "Name"}},
		Namespaced: true,
		Rows:       []Row{{UID: "a", Name: "web", Cells: []string{"web"}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"rows":[`) {
		t.Fatalf("payload = %s", raw)
	}
}
