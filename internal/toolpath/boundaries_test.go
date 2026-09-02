package toolpath

import (
	"os"
	"testing"
)

func TestAPathTheEnvironmentRejectsLeavesTheCurrentPathAlone(t *testing.T) {
	const current = "/usr/bin:/bin"
	t.Setenv("PATH", current)
	shell := fakeShell(t, `printf '/opt/tools\000'`)

	got := Ensure(t.Context(), shell)

	if got != current {
		t.Fatalf("path = %q, want the current path after the environment rejected the replacement", got)
	}
	if os.Getenv("PATH") != current {
		t.Fatalf("environment path = %q, want %q", os.Getenv("PATH"), current)
	}
}
