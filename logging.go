package main

import (
	"io"
	"log/slog"
)

// logHandler backs every line spinoza writes. It escapes attribute values, so
// call sites pass them through unmodified.
func logHandler(out io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
}
