package toolpath

import (
	"errors"
	"os"
	"slices"
	"testing"
)

func refusing(t *testing.T, refused string) {
	t.Helper()
	previous := setenv
	setenv = func(key, value string) error {
		if key == refused {
			return errors.New("the environment refused " + key)
		}
		return previous(key, value)
	}
	t.Cleanup(func() { setenv = previous })
}

func TestAPathTheEnvironmentRejectsLeavesTheCurrentPathAlone(t *testing.T) {
	const current = "/usr/bin:/bin"
	t.Setenv("PATH", current)
	cleared(t, "KUBECONFIG")
	refusing(t, "PATH")
	shell := loginShell(t, `export PATH=/opt/tools:/usr/bin; export KUBECONFIG=/k`)

	got := Ensure(t.Context(), shell)

	if got != current {
		t.Fatalf("path = %q, want the current path after the environment rejected the replacement", got)
	}
	if os.Getenv("PATH") != current {
		t.Fatalf("environment path = %q, want %q", os.Getenv("PATH"), current)
	}
	if _, set := os.LookupEnv("KUBECONFIG"); set {
		t.Fatal("nothing else should be taken once the path was refused")
	}
}

func TestAVariableTheEnvironmentRejectsIsSkipped(t *testing.T) {
	cleared(t, "SPINOZA_TEST_REFUSED")
	cleared(t, "SPINOZA_TEST_TAKEN")
	refusing(t, "SPINOZA_TEST_REFUSED")

	added := adopt(map[string]string{"SPINOZA_TEST_REFUSED": "no", "SPINOZA_TEST_TAKEN": "yes"})

	if !slices.Equal(added, []string{"SPINOZA_TEST_TAKEN"}) {
		t.Fatalf("added = %v, want only the variable the environment accepted", added)
	}
	if os.Getenv("SPINOZA_TEST_TAKEN") != "yes" {
		t.Fatalf("SPINOZA_TEST_TAKEN = %q", os.Getenv("SPINOZA_TEST_TAKEN"))
	}
}
