package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runMutationTotalCheck(t *testing.T, reports map[string]string) (string, error) {
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
	out, err := exec.Command("bash", "check-mutation-total.sh", dir).CombinedOutput()
	return string(out), err
}

func mutationTotalReports() map[string]string {
	return map[string]string{
		"root-default-root.json": `{"mutants_killed":30,"mutants_lived":0,"mutants_not_covered":10,"files":[]}`,
		"root-desktop-root.json": `{"mutants_killed":20,"mutants_lived":0,"mutants_not_covered":8,"files":[]}`,
		"internal-a-d-auth.json": `{"mutants_killed":10,"mutants_lived":0,"mutants_not_covered":5,` +
			`"files":[{"file_name":"auth.go","mutations":[` +
			`{"status":"NOT COVERED","line":3,"type":"CONDITIONALS_NEGATION"},` +
			`{"status":"NOT COVERED","line":4,"type":"CONDITIONALS_BOUNDARY"}]}]}`,
	}
}

func TestMutationTotalCheckSeparatesTheTwoBuilds(t *testing.T) {
	out, err := runMutationTotalCheck(t, mutationTotalReports())
	if err != nil {
		t.Fatalf("check mutation totals: %v", err)
	}
	if !strings.Contains(out, "60 killed, 0 survived; uncovered 15 default, 13 desktop") {
		t.Fatalf("output = %q, want the per-build totals", out)
	}
}

func TestMutationTotalCheckNamesTheFilesNoTestReaches(t *testing.T) {
	out, err := runMutationTotalCheck(t, mutationTotalReports())
	if err != nil {
		t.Fatalf("check mutation totals: %v", err)
	}
	if !strings.Contains(out, "| 2 | internal-a-d-auth | auth.go |") {
		t.Fatalf("output = %q, want the uncovered file listed", out)
	}
}

func TestMutationTotalCheckWarnsAboutASurvivingMutantWithoutFailing(t *testing.T) {
	reports := mutationTotalReports()
	reports["internal-a-d-auth.json"] = `{"mutants_killed":10,"mutants_lived":1,"mutants_not_covered":0,` +
		`"files":[{"file_name":"auth.go","mutations":[{"status":"LIVED","line":42,"type":"REMOVE_SELF_ASSIGNMENTS"}]}]}`

	out, err := runMutationTotalCheck(t, reports)
	if err != nil {
		t.Fatalf("check surviving mutant: %v", err)
	}
	if !strings.Contains(out, "::warning title=Mutant survived::REMOVE_SELF_ASSIGNMENTS changed auth.go:42") {
		t.Fatalf("output = %q, want a warning annotation", out)
	}
	if !strings.Contains(out, "| internal-a-d-auth | auth.go | 42 | REMOVE_SELF_ASSIGNMENTS |") {
		t.Fatalf("output = %q, want the survivor in the summary", out)
	}
}

func TestMutationTotalCheckWritesTheSummaryWhereActionsShowsIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the mutation total checker runs in the Linux CI job")
	}
	dir := t.TempDir()
	for name, report := range mutationTotalReports() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(report), 0o600); err != nil {
			t.Fatalf("write report: %v", err)
		}
	}
	summary := filepath.Join(t.TempDir(), "summary.md")
	cmd := exec.Command("bash", "check-mutation-total.sh", dir)
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY="+summary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("check mutation totals: %v: %s", err, out)
	}

	written, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(written), "## Mutation testing") {
		t.Fatalf("summary = %q, want the mutation heading", written)
	}
}

func TestMutationTotalCheckRejectsAMissingDesktopRootReport(t *testing.T) {
	reports := mutationTotalReports()
	delete(reports, "root-desktop-root.json")
	_, err := runMutationTotalCheck(t, reports)
	if code := exitCode(t, err); code != 11 {
		t.Fatalf("exit code = %d, want 11", code)
	}
}

func TestMutationTotalCheckRejectsAnEmptyReportDirectory(t *testing.T) {
	_, err := runMutationTotalCheck(t, map[string]string{})
	if code := exitCode(t, err); code != 11 {
		t.Fatalf("exit code = %d, want 11", code)
	}
}

func TestMutationTotalCheckRejectsAReportItCannotRead(t *testing.T) {
	reports := mutationTotalReports()
	reports["internal-a-d-auth.json"] = "not json"
	_, err := runMutationTotalCheck(t, reports)
	if code := exitCode(t, err); code != 11 {
		t.Fatalf("exit code = %d, want 11", code)
	}
}
