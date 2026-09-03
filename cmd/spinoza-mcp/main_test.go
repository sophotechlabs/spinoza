package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useMCPArgs(t *testing.T, args ...string) {
	t.Helper()
	original := os.Args
	os.Args = append([]string{"spinoza-mcp"}, args...)
	t.Cleanup(func() {
		os.Args = original
	})
}

func preserveMCPLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
}

func TestRunReportsAnInvalidTimeout(t *testing.T) {
	useMCPArgs(t, "-sync-timeout", "eventually")

	err := run()

	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Fatalf("error = %v, want the invalid timeout", err)
	}
}

func TestRunReportsAnExplicitContextThatCannotOpen(t *testing.T) {
	preserveMCPLogger(t)
	config := filepath.Join(t.TempDir(), "missing-kubeconfig")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	useMCPArgs(t, "-kubeconfig", config, "-context", "missing")

	err := run()

	if err == nil || !strings.Contains(err.Error(), config) {
		t.Fatalf("error = %v, want kubeconfig %q named", err, config)
	}
}

func TestRunWithoutAReachableClusterSaysHowToChooseOne(t *testing.T) {
	preserveMCPLogger(t)
	config := filepath.Join(t.TempDir(), "missing-kubeconfig")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	useMCPArgs(t, "-kubeconfig", config)

	err := run()

	if err == nil || err.Error() != "no cluster answered; name a context with -context" {
		t.Fatalf("error = %v", err)
	}
}
