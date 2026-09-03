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
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_not_covered":0}`, "255")
	if err != nil {
		t.Fatalf("check complete report: %v", err)
	}
}

func TestMutationReportCheckAcceptsTheExistingUncoveredBaseline(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_not_covered":255}`, "255")
	if err != nil {
		t.Fatalf("check baseline report: %v", err)
	}
}

func TestMutationReportCheckRejectsASurvivingMutant(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":99.5,"mutants_not_covered":255}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check incomplete report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 10 {
		t.Fatalf("exit code = %d, want 10", exit.ExitCode())
	}
}

func TestMutationReportCheckRejectsAnIncreaseInUncoveredMutants(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_not_covered":256}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check incomplete report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 11 {
		t.Fatalf("exit code = %d, want 11", exit.ExitCode())
	}
}

func TestMutationReportCheckRejectsAReportWithNoTestedMutants(t *testing.T) {
	err := runMutationReportCheck(t, `{"test_efficacy":0,"mutants_not_covered":0}`, "255")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check empty report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 10 {
		t.Fatalf("exit code = %d, want 10", exit.ExitCode())
	}
}
