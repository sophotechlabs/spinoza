package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func runMutationReportCheck(t *testing.T, report, maxNotCovered string) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the mutation report checker runs in the Linux CI job")
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return exec.Command("bash", "check-mutation-report.sh", path, maxNotCovered).Run()
}

func TestMutationReportCheckAcceptsCompleteCoverage(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":0,"files":[]}`, "255")
	if err != nil {
		t.Fatalf("check complete report: %v", err)
	}
}

func TestMutationReportCheckAcceptsTheExistingUncoveredBaseline(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":255,"files":[]}`, "255")
	if err != nil {
		t.Fatalf("check baseline report: %v", err)
	}
}

func TestMutationReportCheckRejectsASurvivingMutant(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":50,"mutants_killed":1,"mutants_lived":1,"mutants_not_covered":255,"files":[]}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check incomplete report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 10 {
		t.Fatalf("exit code = %d, want 10", exit.ExitCode())
	}
}

func TestMutationReportCheckRejectsAnIncreaseInUncoveredMutants(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":256,"files":[]}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check incomplete report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 11 {
		t.Fatalf("exit code = %d, want 11", exit.ExitCode())
	}
}

func TestMutationReportCheckRejectsAReportWithNoTestedMutants(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":0,"mutants_killed":0,"mutants_lived":0,"mutants_not_covered":0,"files":[]}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check empty report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 10 {
		t.Fatalf("exit code = %d, want 10", exit.ExitCode())
	}
}

func TestMutationReportCheckAcceptsKnownUncoveredMutantsWithoutRunnableMutants(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":0,"mutants_killed":0,"mutants_lived":0,"mutants_not_covered":3,"files":[]}`, "3")
	if err != nil {
		t.Fatalf("check uncovered-only report: %v", err)
	}
}

func TestMutationReportCheckRejectsATimedOutMutantHiddenFromTheSummary(t *testing.T) {
	report := `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":0,"files":[{"mutations":[{"status":"TIMED OUT"}]}]}`
	err := runMutationReportCheck(t, report, "0")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check timed-out report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 12 {
		t.Fatalf("exit code = %d, want 12", exit.ExitCode())
	}
}

func TestMutationReportCheckRejectsADryRunPresentedAsACompleteReport(t *testing.T) {
	report := `{"test_efficacy":0,"mutants_killed":0,"mutants_lived":0,"mutants_not_covered":1,"files":[{"mutations":[{"status":"RUNNABLE"}]}]}`
	err := runMutationReportCheck(t, report, "1")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check dry-run report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 12 {
		t.Fatalf("exit code = %d, want 12", exit.ExitCode())
	}
}
