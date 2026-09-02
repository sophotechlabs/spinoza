package toolpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeShell(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shell")
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o600)
	if err != nil {
		t.Fatalf("write the fake shell: %v", err)
	}
	chmodErr := os.Chmod(path, 0o700)
	if chmodErr != nil {
		t.Fatalf("make the fake shell runnable: %v", chmodErr)
	}
	return path
}

func TestABareEnvironmentIsRecognised(t *testing.T) {
	cases := map[string]bool{
		"/usr/bin:/bin:/usr/sbin:/sbin":            true,
		"/usr/bin:/bin":                            true,
		"":                                         true,
		"/opt/homebrew/bin:/usr/bin:/bin":          false,
		"/usr/local/bin:/usr/bin":                  false,
		"/usr/bin:/bin:/Users/arch/.local/bin":     false,
		"/usr/bin::/bin":                           true,
		"/opt/homebrew/share/google-cloud-sdk/bin": false,
	}
	for path, want := range cases {
		if got := Bare(path); got != want {
			t.Fatalf("Bare(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestTheCurrentPathKeepsItsPlaceInFront(t *testing.T) {
	got := Merge("/usr/bin:/bin", "/opt/homebrew/bin:/usr/bin")

	if got != "/usr/bin:/bin:/opt/homebrew/bin" {
		t.Fatalf("merged = %q", got)
	}
}

func TestADirectoryIsNeverListedTwice(t *testing.T) {
	got := Merge("/usr/bin", "/usr/bin:/usr/bin:/opt/homebrew/bin")

	if strings.Count(got, "/usr/bin") != 1 {
		t.Fatalf("merged = %q, want /usr/bin once", got)
	}
}

func TestEmptyEntriesAreDropped(t *testing.T) {
	got := Merge("/usr/bin::", ":/opt/homebrew/bin:")

	if got != "/usr/bin:/opt/homebrew/bin" {
		t.Fatalf("merged = %q", got)
	}
}

func TestTheShellPathIsRead(t *testing.T) {
	shell := fakeShell(t, `printf %s /opt/homebrew/bin:/usr/bin`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("path = %q", got)
	}
}

func TestWithoutAShellThereIsNothingToAsk(t *testing.T) {
	_, err := FromLoginShell(t.Context(), "")

	want := "there is no login shell to ask"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestAShellThatFailsIsReported(t *testing.T) {
	shell := fakeShell(t, "exit 3")

	_, err := FromLoginShell(t.Context(), shell)

	if err == nil {
		t.Fatal("expected the failure to be reported")
	}
	if !strings.Contains(err.Error(), "asking") {
		t.Fatalf("error = %v, want it to name what it was doing", err)
	}
}

func TestAShellThatPrintsNothingIsNotUsed(t *testing.T) {
	shell := fakeShell(t, "true")

	_, err := FromLoginShell(t.Context(), shell)

	if err == nil || !strings.Contains(err.Error(), "reported no PATH") {
		t.Fatalf("error = %v, want it to say the shell reported no PATH", err)
	}
}

func TestAShellCannotFillMemoryWithItsPathReply(t *testing.T) {
	shell := fakeShell(t, `printf %s `+strings.Repeat("x", maxPathBytes+1))

	_, err := FromLoginShell(t.Context(), shell)

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("error = %v, want the bounded reply named", err)
	}
}

func TestABarePathPicksUpTheShellDirectories(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	shell := fakeShell(t, `printf %s /opt/homebrew/bin:/usr/bin`)

	got := Ensure(t.Context(), shell)

	if got != "/usr/bin:/bin:/opt/homebrew/bin" {
		t.Fatalf("path = %q", got)
	}
	if os.Getenv("PATH") != got {
		t.Fatalf("the environment was not updated: %q", os.Getenv("PATH"))
	}
}

func TestAPathThatAlreadyHasToolsIsLeftAlone(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	shell := fakeShell(t, `printf %s /somewhere/else`)

	got := Ensure(t.Context(), shell)

	if got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("path = %q, want it untouched", got)
	}
}

func TestAFailedProbeLeavesThePathAsItWas(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	got := Ensure(t.Context(), "")

	if got != "/usr/bin:/bin" {
		t.Fatalf("path = %q, want it untouched", got)
	}
	if os.Getenv("PATH") != "/usr/bin:/bin" {
		t.Fatalf("the environment changed to %q", os.Getenv("PATH"))
	}
}
