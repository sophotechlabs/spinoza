//go:build !desktop

package main

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/update"
)

func TestACommandLineBuildHasAnInstaller(t *testing.T) {
	if updateInstaller() == nil {
		t.Fatal("no installer was built for a command-line build")
	}
}

func TestSelfUpdateIsDisabledByDefault(t *testing.T) {
	t.Setenv("SPINOZA_UNSAFE_SELF_UPDATE", "")

	if got := selfUpdateScript(); got != "" {
		t.Fatalf("script = %q, want self-update disabled", got)
	}
}

func TestSelfUpdateRequiresTheExplicitUnsafeOptIn(t *testing.T) {
	t.Setenv("SPINOZA_UNSAFE_SELF_UPDATE", "true")

	if got := selfUpdateScript(); got != update.Script {
		t.Fatalf("script = %q, want %q", got, update.Script)
	}
}
