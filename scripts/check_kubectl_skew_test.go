package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skewFixture(t *testing.T, pinned, shipped, readme string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the skew check runs in the Linux CI job")
	}
	dir := t.TempDir()
	files := map[string]string{
		"mise.toml":  "[tools]\ngo = \"1.27.1\"\nkubectl = \"" + pinned + "\"\n",
		"Dockerfile": "FROM alpine\nARG KUBECTL_VERSION=" + shipped + "\n",
		"README.md":  readme + "\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func runSkewCheck(t *testing.T, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "check-kubectl-skew.sh", dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const promised = "Built on client-go v0.36, so Kubernetes 1.35 to 1.37 by skew policy. Runs in production against k3s v1.36."

func TestAKubectlInsideTheSkewOfEveryPromisedMinorPasses(t *testing.T) {
	out, err := runSkewCheck(t, skewFixture(t, "1.36.4", "1.36.4", promised))
	if err != nil {
		t.Fatalf("the check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "kubectl 1.36.4 covers the promised 1.35 to 1.37") {
		t.Fatalf("output = %q", out)
	}
}

func TestAKubectlTwoMinorsBehindThePromiseFails(t *testing.T) {
	out, err := runSkewCheck(t, skewFixture(t, "1.34.1", "1.34.1", promised))
	if err == nil {
		t.Fatalf("the check passed:\n%s", out)
	}
	if !strings.Contains(out, "kubectl 1.34.1 only covers 1.33 to 1.35") {
		t.Fatalf("output = %q, want it to name the skew", out)
	}
}

func TestMismatchedKubectlPinsFail(t *testing.T) {
	out, err := runSkewCheck(t, skewFixture(t, "1.36.4", "1.35.8", promised))
	if err == nil {
		t.Fatalf("the check passed:\n%s", out)
	}
	if !strings.Contains(out, "keep them on one minor") {
		t.Fatalf("output = %q", out)
	}
}

func TestAReadmeWithNoSupportRangeFails(t *testing.T) {
	out, err := runSkewCheck(t, skewFixture(t, "1.36.4", "1.36.4", "Built on client-go."))
	if err == nil {
		t.Fatalf("the check passed:\n%s", out)
	}
	if !strings.Contains(out, "could not read") {
		t.Fatalf("output = %q", out)
	}
}

func TestTheCheckedInPinsMatchTheReadme(t *testing.T) {
	out, err := runSkewCheck(t, "..")
	if err != nil {
		t.Fatalf("the repository's own pins fail the check: %v\n%s", err, out)
	}
}
