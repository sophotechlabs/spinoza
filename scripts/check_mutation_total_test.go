package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func runMutationTotalCheck(t *testing.T, reports map[string]string, maxDefault, maxDesktop string) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the mutation total checker runs in the Linux CI job")
	}
	dir := t.TempDir()
	for name, report := range reports {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
			t.Fatalf("write report: %v", err)
		}
	}
	return exec.Command("bash", "check-mutation-total.sh", dir, maxDefault, maxDesktop).Run()
}

func mutationTotalReports() map[string]string {
	return map[string]string{
		"root-default-root.json": `{"mutants_not_covered":10}`,
		"root-desktop-root.json": `{"mutants_not_covered":8}`,
		"internal-a-d-auth.json": `{"mutants_not_covered":5}`,
	}
}

func TestMutationTotalCheckAcceptsBothBaselines(t *testing.T) {
	err := runMutationTotalCheck(t, mutationTotalReports(), "15", "13")
	if err != nil {
		t.Fatalf("check mutation totals: %v", err)
	}
}

func TestMutationTotalCheckRejectsADefaultRegression(t *testing.T) {
	err := runMutationTotalCheck(t, mutationTotalReports(), "14", "13")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check default total error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 11 {
		t.Fatalf("exit code = %d, want 11", exit.ExitCode())
	}
}

func TestMutationTotalCheckRejectsADesktopRegression(t *testing.T) {
	err := runMutationTotalCheck(t, mutationTotalReports(), "15", "12")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check desktop total error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 12 {
		t.Fatalf("exit code = %d, want 12", exit.ExitCode())
	}
}

func TestMutationTotalCheckRejectsAMissingDesktopRootReport(t *testing.T) {
	reports := mutationTotalReports()
	delete(reports, "root-desktop-root.json")
	err := runMutationTotalCheck(t, reports, "15", "13")

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("check missing report error = %v, want an exit error", err)
	}
	if exit.ExitCode() != 11 {
		t.Fatalf("exit code = %d, want 11", exit.ExitCode())
	}
}
