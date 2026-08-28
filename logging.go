package main

import (
	"io"
	"log/slog"
)

// Escapes attribute values, so call sites pass them unmodified.
func logHandler(out io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
}
