package main

import (
	"io"
	"log/slog"
)

// logHandler is what every line spinoza writes goes through. Half of what gets
// logged — a path, a pod name, the value a panic carried — comes from whoever
// made the request, and what stops one of those writing a log line of its own is
// this handler escaping what it is given. The call sites hand over the value as
// they received it and nothing quotes it twice.
func logHandler(out io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
}
