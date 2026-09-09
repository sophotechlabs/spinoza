package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func runNightlyDue(t *testing.T, event, last string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the nightly guard runs in Linux CI jobs")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatalf("create fake binary directory: %v", err)
	}
	gh := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GH_CALLS"
printf '%s' "$GH_LAST"
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	calls := filepath.Join(dir, "calls")
	if err := os.WriteFile(calls, nil, 0o600); err != nil {
		t.Fatalf("seed call log: %v", err)
	}
	command := exec.Command("bash", "nightly-due.sh")
	command.Env = append(
		os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_CALLS="+calls,
		"GH_LAST="+last,
		"GITHUB_EVENT_NAME="+event,
		"NIGHTLY_WINDOW_HOURS=20",
	)
	out, err := command.Output()
	if err != nil {
		t.Fatalf("run nightly-due.sh: %v", err)
	}
	raw, readErr := os.ReadFile(calls)
	if readErr != nil {
		t.Fatalf("read gh calls: %v", readErr)
	}
	logged := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(logged) == 1 && logged[0] == "" {
		logged = nil
	}
	return strings.TrimSpace(string(out)), logged
}

func TestNightlyIsDueWhenNoRunHasEverSucceeded(t *testing.T) {
	verdict, calls := runNightlyDue(t, "push", "")
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true", verdict)
	}
	if len(calls) != 1 {
		t.Fatalf("gh calls = %q, want one lookup", calls)
	}
}

func TestNightlyIsDueWhenTheLastRunAgedOutOfTheWindow(t *testing.T) {
	last := time.Now().UTC().Add(-21 * time.Hour).Format("2006-01-02T15:04:05Z")
	verdict, _ := runNightlyDue(t, "push", last)
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true for a run %s", verdict, last)
	}
}

func TestNightlyIsNotDueInsideTheWindow(t *testing.T) {
	last := time.Now().UTC().Add(-3 * time.Hour).Format("2006-01-02T15:04:05Z")
	verdict, _ := runNightlyDue(t, "push", last)
	if verdict != "false" {
		t.Fatalf("verdict = %q, want false for a run %s", verdict, last)
	}
}

func TestNightlyIsNotDueOnTheHourItLastSucceeded(t *testing.T) {
	last := time.Now().UTC().Add(-19*time.Hour - 59*time.Minute).Format("2006-01-02T15:04:05Z")
	verdict, _ := runNightlyDue(t, "schedule", last)
	if verdict != "false" {
		t.Fatalf("verdict = %q, want false just inside the window", verdict)
	}
}

func TestAnExplicitDispatchSkipsTheWindowAndAsksGitHubNothing(t *testing.T) {
	for _, event := range []string{"workflow_dispatch", "repository_dispatch"} {
		last := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		verdict, calls := runNightlyDue(t, event, last)
		if verdict != "true" {
			t.Fatalf("%s verdict = %q, want true", event, verdict)
		}
		if calls != nil {
			t.Fatalf("%s made gh calls %q, want none", event, calls)
		}
	}
}

func TestTheScheduleIsSubjectToTheSameWindowAsAPush(t *testing.T) {
	last := time.Now().UTC().Add(-21 * time.Hour).Format("2006-01-02T15:04:05Z")
	verdict, calls := runNightlyDue(t, "schedule", last)
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true", verdict)
	}
	if len(calls) != 1 {
		t.Fatalf("gh calls = %q, want one lookup", calls)
	}
}
