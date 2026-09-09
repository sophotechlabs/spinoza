package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func runKindClusterCreate(t *testing.T, succeedOn int) ([]string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the kind cluster creator runs in Linux CI jobs")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatalf("create fake binary directory: %v", err)
	}
	kind := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$KIND_CALLS"
if [ "$1" = create ]; then
    count=0
    if [ -f "$KIND_COUNT" ]; then
        count=$(<"$KIND_COUNT")
    fi
    count=$((count + 1))
    printf '%d\n' "$count" > "$KIND_COUNT"
    if [ "$count" -lt "$KIND_SUCCEED_ON" ]; then
        exit 1
    fi
fi
`
	if err := os.WriteFile(filepath.Join(bin, "kind"), []byte(kind), 0o700); err != nil {
		t.Fatalf("write fake kind: %v", err)
	}
	calls := filepath.Join(dir, "calls")
	count := filepath.Join(dir, "count")
	command := exec.Command("bash", "create-kind-cluster.sh", "ci-cluster", "--config", "kind.yaml", "--wait", "300s")
	command.Env = append(
		os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"KIND_CALLS="+calls,
		"KIND_COUNT="+count,
		"KIND_SUCCEED_ON="+strconv.Itoa(succeedOn),
		"KIND_CREATE_RETRY_DELAY_SECONDS=0",
	)
	err := command.Run()
	raw, readErr := os.ReadFile(calls)
	if readErr != nil {
		t.Fatalf("read kind calls: %v", readErr)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n"), err
}

func TestKindClusterCreationRetriesAfterCleaningTheFailedAttempt(t *testing.T) {
	calls, err := runKindClusterCreate(t, 3)
	if err != nil {
		t.Fatalf("create kind cluster: %v", err)
	}
	want := []string{
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
		"delete cluster --name ci-cluster",
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
		"delete cluster --name ci-cluster",
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("kind calls = %q, want %q", calls, want)
	}
}

func TestKindClusterCreationStopsAfterThreeFailedAttempts(t *testing.T) {
	calls, err := runKindClusterCreate(t, 4)
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("create kind cluster error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exit.ExitCode())
	}
	want := []string{
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
		"delete cluster --name ci-cluster",
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
		"delete cluster --name ci-cluster",
		"create cluster --name ci-cluster --config kind.yaml --wait 300s",
		"delete cluster --name ci-cluster",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("kind calls = %q, want %q", calls, want)
	}
}
