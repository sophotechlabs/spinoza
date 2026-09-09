package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type mirroredStore struct {
	entries []Entry
	err     error
}

func (m *mirroredStore) Record(_ context.Context, entry Entry) error {
	m.entries = append(m.entries, entry)
	return m.err
}

func mirroring(into Recorder) (Recorder, *bytes.Buffer) {
	out := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return Mirror(into, logger), out
}

func mirroredLines(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := []map[string]any{}
	for raw := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("the log line is not json: %v: %s", err, raw)
		}
		lines = append(lines, line)
	}
	return lines
}

func mirroredLine(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	lines := mirroredLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("the entry made %d log lines, want exactly one: %s", len(lines), out.String())
	}
	return lines[0]
}

func TestAMirroredEntryIsOneJSONLineCarryingEveryFieldItHas(t *testing.T) {
	into := &mirroredStore{}
	mirrored, out := mirroring(into)

	if err := mirrored.Record(t.Context(), entry(p1, noon, "web")); err != nil {
		t.Fatalf("record: %v", err)
	}

	line := mirroredLine(t, out)
	wanted := map[string]string{
		"event":     "audit",
		"msg":       auditLine,
		"at":        "2026-08-29T12:00:00Z",
		"cluster":   p1,
		"verb":      "delete",
		"actor":     "alice@example.com",
		"group":     "apps",
		"version":   "v1",
		"resource":  "deployments",
		"kind":      "Deployment",
		"namespace": "default",
		"name":      "web",
		"detail":    "deleted the deployment",
		"outcome":   "ok",
	}
	for key, want := range wanted {
		if line[key] != want {
			t.Errorf("%s = %v, want %q", key, line[key], want)
		}
	}
}

func TestAMirroredEntryLeavesOutTheFieldsItDoesNotCarry(t *testing.T) {
	into := &mirroredStore{}
	mirrored, out := mirroring(into)

	err := mirrored.Record(t.Context(), Entry{At: noon, Verb: "signed in", Actor: "alice@example.com", Outcome: "done"})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	line := mirroredLine(t, out)
	for _, key := range []string{"group", "version", "resource", "kind", "namespace", "name", "detail", "message"} {
		if _, held := line[key]; held {
			t.Errorf("%s came through as %v, want it left out when it is empty", key, line[key])
		}
	}
	if line["verb"] != "signed in" || line["actor"] != "alice@example.com" {
		t.Fatalf("the fields it does carry came through as %v", line)
	}
}

func TestAMirroredEntryReachesTheStoreUnchanged(t *testing.T) {
	into := &mirroredStore{}
	mirrored, _ := mirroring(into)
	held := entry(p1, noon, "web")

	if err := mirrored.Record(t.Context(), held); err != nil {
		t.Fatalf("record: %v", err)
	}

	if len(into.entries) != 1 {
		t.Fatalf("the store took %d entries, want one", len(into.entries))
	}
	if into.entries[0] != held {
		t.Fatalf("the store took %+v, want %+v", into.entries[0], held)
	}
}

func TestAnEntryTheStoreRefusedIsStillLoggedAndTheRefusalComesBack(t *testing.T) {
	refused := errors.New("the history file is read-only")
	into := &mirroredStore{err: refused}
	mirrored, out := mirroring(into)

	err := mirrored.Record(t.Context(), entry(p1, noon, "web"))

	if !errors.Is(err, refused) {
		t.Fatalf("error = %v, want the store's own refusal", err)
	}
	line := mirroredLine(t, out)
	if line["event"] != "audit" || line["name"] != "web" {
		t.Fatalf("the refused entry logged as %v", line)
	}
}

func TestMirroringWithoutALoggerLeavesTheRecorderAsItWas(t *testing.T) {
	into := &mirroredStore{}

	if Mirror(into, nil) != Recorder(into) {
		t.Fatal("mirroring without a logger wrapped the recorder anyway")
	}
}
