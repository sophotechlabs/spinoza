package debugcontainer

import (
	"context"
	"strings"
	"testing"
)

func TestRunnerReportsAMissingBinary(t *testing.T) {
	runner := NewRunner("kubectl-does-not-exist-anywhere")

	err := runner.Run(context.Background(), []string{"debug"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRunnerSurfacesStderr(t *testing.T) {
	runner := NewRunner("sh")

	err := runner.Run(context.Background(), []string{"-c", "echo 'Error from server (Forbidden)' >&2; exit 1"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRunnerBoundsStderr(t *testing.T) {
	runner, ok := NewRunner("sh").(*kubectlRunner)
	if !ok {
		t.Fatal("NewRunner returned something other than the kubectl runner")
	}
	runner.errorLimit = 4

	err := runner.Run(context.Background(), []string{"-c", "printf 12345 >&2; exit 1"})

	if err == nil || !strings.Contains(err.Error(), "exceeded its limit") {
		t.Fatalf("message = %v", err)
	}
}

func TestRunnerDropsTheProfileDeprecationNotice(t *testing.T) {
	runner := NewRunner("sh")

	err := runner.Run(context.Background(), []string{
		"-c",
		"echo '--profile=legacy is deprecated and will be removed' >&2; echo 'real failure' >&2; exit 1",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "deprecated") {
		t.Fatalf("the deprecation notice should be filtered out: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "real failure") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRunnerFallsBackWhenStderrIsEmpty(t *testing.T) {
	runner := NewRunner("sh")

	err := runner.Run(context.Background(), []string{"-c", "exit 3"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunnerSucceeds(t *testing.T) {
	runner := NewRunner("sh")

	err := runner.Run(context.Background(), []string{"-c", "exit 0"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestNewRunnerDefaultsToKubectl(t *testing.T) {
	if NewRunner("") == nil {
		t.Fatal("expected a runner")
	}
}
