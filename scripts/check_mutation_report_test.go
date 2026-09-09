package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runMutationReportCheck(t *testing.T, report string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the mutation report checker runs in the Linux CI job")
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	out, err := exec.Command("bash", "check-mutation-report.sh", path).CombinedOutput()
	return string(out), err
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("error = %v, want an exit error", err)
	}
	return exit.ExitCode()
}

func TestMutationReportCheckReportsTheCounts(t *testing.T) {
	out, err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":7,"mutants_lived":0,"mutants_not_covered":3,"files":[]}`)
	if err != nil {
		t.Fatalf("check complete report: %v", err)
	}
	if !strings.Contains(out, "killed 7, survived 0, uncovered 3") {
		t.Fatalf("output = %q, want the three counts", out)
	}
}

func TestMutationReportCheckAcceptsAnyNumberOfUncoveredMutants(t *testing.T) {
	if _, err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":99999,"files":[]}`); err != nil {
		t.Fatalf("check uncovered report: %v", err)
	}
}

func TestMutationReportCheckAcceptsASurvivingMutant(t *testing.T) {
	report := `{"test_efficacy":50,"mutants_killed":1,"mutants_lived":1,"mutants_not_covered":0,` +
		`"files":[{"file_name":"a.go","mutations":[{"status":"LIVED","line":4,"type":"CONDITIONALS_NEGATION"}]}]}`
	out, err := runMutationReportCheck(t, report)
	if err != nil {
		t.Fatalf("check surviving mutant: %v", err)
	}
	if !strings.Contains(out, "survived 1") {
		t.Fatalf("output = %q, want the survivor counted", out)
	}
}

func TestMutationReportCheckRejectsAReportWithNoMutants(t *testing.T) {
	_, err := runMutationReportCheck(t, `{"test_efficacy":0,"mutants_killed":0,"mutants_lived":0,"mutants_not_covered":0,"files":[]}`)
	if code := exitCode(t, err); code != 10 {
		t.Fatalf("exit code = %d, want 10", code)
	}
}

func TestMutationReportCheckRejectsAMissingCount(t *testing.T) {
	_, err := runMutationReportCheck(t, `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"files":[]}`)
	if code := exitCode(t, err); code != 11 {
		t.Fatalf("exit code = %d, want 11", code)
	}
}

func TestMutationReportCheckRejectsATimedOutMutantHiddenFromTheSummary(t *testing.T) {
	report := `{"test_efficacy":100,"mutants_killed":1,"mutants_lived":0,"mutants_not_covered":0,"files":[{"mutations":[{"status":"TIMED OUT"}]}]}`
	_, err := runMutationReportCheck(t, report)
	if code := exitCode(t, err); code != 12 {
		t.Fatalf("exit code = %d, want 12", code)
	}
}

func TestMutationReportCheckRejectsADryRunPresentedAsACompleteReport(t *testing.T) {
	report := `{"test_efficacy":0,"mutants_killed":0,"mutants_lived":0,"mutants_not_covered":1,"files":[{"mutations":[{"status":"RUNNABLE"}]}]}`
	_, err := runMutationReportCheck(t, report)
	if code := exitCode(t, err); code != 12 {
		t.Fatalf("exit code = %d, want 12", code)
	}
}
