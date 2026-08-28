package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestKlogRoutesThroughSlog(t *testing.T) {
	var buf bytes.Buffer
	klog.SetSlogLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(klog.ClearLogger)

	klog.Warning("spinoza-klog-probe")
	klog.Flush()

	if !strings.Contains(buf.String(), "spinoza-klog-probe") {
		t.Fatalf("klog output did not reach the slog handler: %q", buf.String())
	}
}

func TestTheHandlerEscapesWhatItIsGiven(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logHandler(&buf, slog.LevelInfo))

	logger.Warn("a websocket upgrade was refused", "path", "/api\nlevel=ERROR msg=\"forged\"")

	written := strings.TrimSuffix(buf.String(), "\n")
	if strings.Contains(written, "\n") {
		t.Fatalf("one call wrote more than one line:\n%s", written)
	}
	if !strings.Contains(written, `\nlevel=ERROR`) {
		t.Fatalf("the newline was not escaped: %s", written)
	}
}

func TestAPanicIsLoggedOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logHandler(&buf, slog.LevelInfo))

	logger.Error("recovered from a panic", "panic", "boom\nlevel=INFO msg=\"all is well\"")

	if lines := strings.Count(strings.TrimSuffix(buf.String(), "\n"), "\n"); lines != 0 {
		t.Fatalf("a panic wrote %d extra lines:\n%s", lines, buf.String())
	}
}
