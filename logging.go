package main

import (
	"io"
	"log/slog"
)

const (
	logText = "text"
	logJSON = "json"
)

func logHandler(out io.Writer, level slog.Leveler, format string) slog.Handler {
	options := &slog.HandlerOptions{Level: level}
	if format == logJSON {
		return slog.NewJSONHandler(out, options)
	}
	return slog.NewTextHandler(out, options)
}

func logFormatFor(asked string, serving bool) string {
	if asked != "" {
		return asked
	}
	if serving {
		return logJSON
	}
	return logText
}

func parseLogFormat(value string) (string, error) {
	switch value {
	case "", logText, logJSON:
		return value, nil
	}
	return "", errUnknownLogFormat
}
