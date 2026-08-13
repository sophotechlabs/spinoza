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
