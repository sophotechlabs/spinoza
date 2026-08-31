package main

import (
	"io"
	"log/slog"
)

func logHandler(out io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
}
