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

// Everything spinoza logs about a request — a path, a pod name, a panic — comes
// from somebody who could put a newline in it. What stops that forging a second
// log line is the handler, not the call sites, so this is the one place the
// property is worth pinning down.
func TestTheHandlerEscapesWhatItIsGiven(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logger.Warn("a websocket upgrade was refused", "path", "/api\nlevel=ERROR msg=\"forged\"")

	written := strings.TrimSuffix(buf.String(), "\n")
	if strings.Contains(written, "\n") {
		t.Fatalf("one call wrote more than one line:\n%s", written)
	}
	if !strings.Contains(written, `\nlevel=ERROR`) {
		t.Fatalf("the newline was not escaped: %s", written)
	}
}

// The same for a panic, which arrives with a stack full of newlines and a value
// somebody else chose.
func TestAPanicIsLoggedOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logger.Error("recovered from a panic", "panic", "boom\nlevel=INFO msg=\"all is well\"")

	if lines := strings.Count(strings.TrimSuffix(buf.String(), "\n"), "\n"); lines != 0 {
		t.Fatalf("a panic wrote %d extra lines:\n%s", lines, buf.String())
	}
}
