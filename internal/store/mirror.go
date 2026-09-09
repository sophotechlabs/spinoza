package store

import (
	"context"
	"log/slog"
	"time"
)

const auditMarker = "audit"

const auditLine = "what spinoza did"

type mirror struct {
	into Recorder
	out  *slog.Logger
}

func Mirror(into Recorder, out *slog.Logger) Recorder {
	if out == nil {
		return into
	}
	return mirror{into: into, out: out}
}

func (m mirror) Record(ctx context.Context, entry Entry) error {
	err := m.into.Record(ctx, entry)
	m.out.LogAttrs(ctx, slog.LevelInfo, auditLine, auditAttrs(entry)...)
	return err
}

type auditField struct {
	key   string
	value string
}

func auditAttrs(entry Entry) []slog.Attr {
	out := []slog.Attr{slog.String("event", auditMarker)}
	if !entry.At.IsZero() {
		out = append(out, slog.String("at", entry.At.UTC().Format(time.RFC3339)))
	}
	for _, field := range auditFields(entry) {
		if field.value == "" {
			continue
		}
		out = append(out, slog.String(field.key, field.value))
	}
	return out
}

func auditFields(entry Entry) []auditField {
	return []auditField{
		{key: "cluster", value: entry.Cluster},
		{key: "verb", value: entry.Verb},
		{key: "actor", value: entry.Actor},
		{key: "group", value: entry.Group},
		{key: "version", value: entry.Version},
		{key: "resource", value: entry.Resource},
		{key: "kind", value: entry.Kind},
		{key: "namespace", value: entry.Namespace},
		{key: "name", value: entry.Name},
		{key: "detail", value: entry.Detail},
		{key: "outcome", value: entry.Outcome},
		{key: "message", value: entry.Message},
	}
}
