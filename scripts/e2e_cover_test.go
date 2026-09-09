package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildInstrumented(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sources := map[string]string{
		"go.mod":  "module hello\n\ngo 1.27\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
	}
	for name, body := range sources {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	binary := filepath.Join(dir, "hello")
	build := exec.Command("go", "build", "-cover", "-covermode=atomic", "-o", binary, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build instrumented program: %v\n%s", err, out)
	}
	return binary
}

func runInstrumented(t *testing.T, binary, counters string) {
	t.Helper()
	if err := os.MkdirAll(counters, 0o700); err != nil {
		t.Fatalf("make %s: %v", counters, err)
	}
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+counters)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run instrumented program: %v\n%s", err, out)
	}
}

func runE2ECover(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the e2e coverage merge runs in the Linux CI job")
	}
	cmd := exec.Command("bash", append([]string{"e2e-cover.sh"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestE2ECoverMergesEveryInstance(t *testing.T) {
	binary := buildInstrumented(t)
	dir := t.TempDir()
	runInstrumented(t, binary, filepath.Join(dir, "main"))
	runInstrumented(t, binary, filepath.Join(dir, "readonly"))
	profile := filepath.Join(dir, "coverage.out")

	out, err := runE2ECover(t, nil, dir, profile)
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}
	for _, want := range []string{
		"E2E Go coverage: 100.0%",
		"2 instrumented instances were merged.",
		"hello",
		"coverage: 100.0% of statements",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	written, readErr := os.ReadFile(profile)
	if readErr != nil {
		t.Fatalf("read profile: %v", readErr)
	}
	if !strings.HasPrefix(string(written), "mode: atomic") {
		t.Fatalf("profile starts with %q, want an atomic mode line", firstLine(string(written)))
	}
}

func TestE2ECoverMarksASelectiveRunPartial(t *testing.T) {
	binary := buildInstrumented(t)
	dir := t.TempDir()
	runInstrumented(t, binary, filepath.Join(dir, "job-a"))
	runInstrumented(t, binary, filepath.Join(dir, "job-b"))
	summary := filepath.Join(t.TempDir(), "summary.md")

	out, err := runE2ECover(t, []string{"GITHUB_STEP_SUMMARY=" + summary}, dir, filepath.Join(dir, "coverage.out"), "39", "2", "16")
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}
	for _, want := range []string{
		"E2E Go coverage (partial): 100.0%",
		"2 of 39 browser jobs contributed coverage.",
		"2 of 16 groups were selected for this change.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	written, readErr := os.ReadFile(summary)
	if readErr != nil {
		t.Fatalf("read step summary: %v", readErr)
	}
	if !strings.HasPrefix(string(written), "## E2E Go coverage (partial): 100.0%\n") {
		t.Fatalf("step summary starts with %q", firstLine(string(written)))
	}
	if strings.Count(string(written), "```") != 2 {
		t.Fatalf("step summary has %d fences, want 2:\n%s", strings.Count(string(written), "```"), written)
	}
}

func TestE2ECoverRefusesAnInstanceThatWasKilled(t *testing.T) {
	binary := buildInstrumented(t)
	dir := t.TempDir()
	runInstrumented(t, binary, filepath.Join(dir, "main"))
	marker := filepath.Join(dir, "readonly.unclean")
	if err := os.WriteFile(marker, []byte("pid 4242 was still running\n"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	out, err := runE2ECover(t, nil, dir, filepath.Join(dir, "coverage.out"))
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "killed before they could write their counters") {
		t.Fatalf("output does not name the killed instance:\n%s", out)
	}
	if !strings.Contains(out, "readonly.unclean: pid 4242 was still running") {
		t.Fatalf("output does not quote the marker:\n%s", out)
	}
}

func TestE2ECoverRefusesADirectoryWithoutCounters(t *testing.T) {
	dir := t.TempDir()

	out, err := runE2ECover(t, nil, dir, filepath.Join(dir, "coverage.out"))
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "no counter files under") {
		t.Fatalf("output does not explain the missing counters:\n%s", out)
	}
}

func TestE2ECoverRefusesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")

	out, err := runE2ECover(t, nil, dir, filepath.Join(t.TempDir(), "coverage.out"))
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "does not exist; the suite did not run") {
		t.Fatalf("output does not say the suite did not run:\n%s", out)
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
