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

type nightlyRun struct {
	id        string
	finished  time.Time
	succeeded string
}

func ranTheNightly(id string, ago time.Duration) nightlyRun {
	return nightlyRun{id: id, finished: time.Now().UTC().Add(-ago), succeeded: "97"}
}

func stoppedAtTheGuard(id string, ago time.Duration) nightlyRun {
	return nightlyRun{id: id, finished: time.Now().UTC().Add(-ago), succeeded: "0"}
}

func runNightlyDue(t *testing.T, event string, runs ...nightlyRun) (string, []string) {
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
if [ "$1 $2" = "run list" ]; then
    printf '%s' "$GH_RUNS"
    exit 0
fi
succeeded="GH_SUCCEEDED_$3"
printf '%s\n' "${!succeeded}"
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	calls := filepath.Join(dir, "calls")
	if err := os.WriteFile(calls, nil, 0o600); err != nil {
		t.Fatalf("seed call log: %v", err)
	}
	var listed strings.Builder
	env := append(
		os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_CALLS="+calls,
		"GITHUB_EVENT_NAME="+event,
		"NIGHTLY_WINDOW_HOURS=20",
	)
	for _, run := range runs {
		listed.WriteString(run.id + " " + run.finished.Format("2006-01-02T15:04:05Z") + "\n")
		env = append(env, "GH_SUCCEEDED_"+run.id+"="+run.succeeded)
	}
	env = append(env, "GH_RUNS="+listed.String())
	command := exec.Command("bash", "nightly-due.sh")
	command.Env = env
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
	verdict, calls := runNightlyDue(t, "push")
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true", verdict)
	}
	if len(calls) != 1 {
		t.Fatalf("gh calls = %q, want one lookup", calls)
	}
}

func TestNightlyIsDueWhenTheLastRunAgedOutOfTheWindow(t *testing.T) {
	verdict, _ := runNightlyDue(t, "push", ranTheNightly("40845046601", 21*time.Hour))
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true for a run 21h ago", verdict)
	}
}

func TestNightlyIsNotDueInsideTheWindow(t *testing.T) {
	verdict, _ := runNightlyDue(t, "push", ranTheNightly("40845046601", 3*time.Hour))
	if verdict != "false" {
		t.Fatalf("verdict = %q, want false for a run 3h ago", verdict)
	}
}

func TestNightlyIsNotDueOnTheHourItLastSucceeded(t *testing.T) {
	verdict, _ := runNightlyDue(t, "schedule", ranTheNightly("40845046601", 19*time.Hour+59*time.Minute))
	if verdict != "false" {
		t.Fatalf("verdict = %q, want false just inside the window", verdict)
	}
}

func TestAnExplicitDispatchSkipsTheWindowAndAsksGitHubNothing(t *testing.T) {
	for _, event := range []string{"workflow_dispatch", "repository_dispatch"} {
		verdict, calls := runNightlyDue(t, event, ranTheNightly("40845046601", 0))
		if verdict != "true" {
			t.Fatalf("%s verdict = %q, want true", event, verdict)
		}
		if calls != nil {
			t.Fatalf("%s made gh calls %q, want none", event, calls)
		}
	}
}

func TestTheScheduleIsSubjectToTheSameWindowAsAPush(t *testing.T) {
	verdict, calls := runNightlyDue(t, "schedule", ranTheNightly("40845046601", 21*time.Hour))
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true", verdict)
	}
	if len(calls) != 2 {
		t.Fatalf("gh calls = %q, want the list and one look at its newest run", calls)
	}
}

func TestARunTheGuardStoppedDoesNotRestartTheWindow(t *testing.T) {
	verdict, calls := runNightlyDue(
		t,
		"schedule",
		stoppedAtTheGuard("38885394510", 2*time.Hour),
		ranTheNightly("38881002417", 21*time.Hour),
	)
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true: the nightly last ran 21h ago, only its guard ran 2h ago", verdict)
	}
	if len(calls) != 3 {
		t.Fatalf("gh calls = %q, want the list and both runs looked at", calls)
	}
	want := []string{"run view 38885394510", "run view 38881002417"}
	for index, prefix := range want {
		if !strings.HasPrefix(calls[index+1], prefix) {
			t.Fatalf("gh call %d = %q, want it to start with %q", index+1, calls[index+1], prefix)
		}
	}
}

func TestTheNewestRunThatGotPastTheGuardDecides(t *testing.T) {
	verdict, calls := runNightlyDue(
		t,
		"push",
		stoppedAtTheGuard("38885394510", time.Hour),
		ranTheNightly("38881002417", 3*time.Hour),
		ranTheNightly("38750118803", 27*time.Hour),
	)
	if verdict != "false" {
		t.Fatalf("verdict = %q, want false: the nightly ran 3h ago", verdict)
	}
	if len(calls) != 3 {
		t.Fatalf("gh calls = %q, want the list and two runs looked at, stopping at the first that ran", calls)
	}
}

func TestOnlyRunsTheGuardStoppedLeaveNothingToAgeOut(t *testing.T) {
	verdict, calls := runNightlyDue(
		t,
		"push",
		stoppedAtTheGuard("38885394510", time.Hour),
		stoppedAtTheGuard("38870021954", 5*time.Hour),
	)
	if verdict != "true" {
		t.Fatalf("verdict = %q, want true when no listed run got past the guard", verdict)
	}
	if len(calls) != 3 {
		t.Fatalf("gh calls = %q, want the list and both runs looked at", calls)
	}
}
