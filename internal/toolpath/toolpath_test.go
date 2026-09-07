package toolpath

import (
	"os"
	"path/filepath"
	"slices"
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

func loginShell(t *testing.T, exports string) string {
	t.Helper()
	return fakeShell(t, exports+"\nshift 3\neval \"$1\"")
}

func cleared(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	err := os.Unsetenv(key)
	if err != nil {
		t.Fatalf("clear %s: %v", key, err)
	}
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

func TestTheShellEnvironmentIsRead(t *testing.T) {
	shell := loginShell(t, `export PATH=/opt/homebrew/bin:/usr/bin; export KUBECONFIG=/home/me/.kube/eks.yaml`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got["PATH"] != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("path = %q", got["PATH"])
	}
	if got["KUBECONFIG"] != "/home/me/.kube/eks.yaml" {
		t.Fatalf("kubeconfig = %q", got["KUBECONFIG"])
	}
}

func TestTheShellIsAskedAsALoginAndInteractiveShell(t *testing.T) {
	shell := fakeShell(t, `printf '\nspinoza-environment\nPATH=%s\0' "$1$2"`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got["PATH"] != "-l-i" {
		t.Fatalf("flags = %q, want -l and -i so that both profile and rc files are read", got["PATH"])
	}
}

func TestWhatTheProfilePrintsBeforeTheMarkerIsIgnored(t *testing.T) {
	shell := fakeShell(t, `printf 'Welcome back\n'; printf '\nspinoza-environment\n'; printf 'PATH=/opt/tools\0KUBECONFIG=/k\0'`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("environment = %v, want only the two entries after the marker", got)
	}
	if got["KUBECONFIG"] != "/k" {
		t.Fatalf("kubeconfig = %q", got["KUBECONFIG"])
	}
}

func TestAnEnvWithoutNulSupportIsStillRead(t *testing.T) {
	shell := fakeShell(t, `printf '\nspinoza-environment\nPATH=/opt/tools\nNOTE=line one\nline two\nKUBECONFIG=/k\n'`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got["KUBECONFIG"] != "/k" {
		t.Fatalf("kubeconfig = %q", got["KUBECONFIG"])
	}
	if got["NOTE"] != "line one\nline two" {
		t.Fatalf("multi-line value = %q", got["NOTE"])
	}
}

func TestANameThatIsNotAVariableIsDropped(t *testing.T) {
	shell := fakeShell(t, `printf '\nspinoza-environment\nPATH=/opt/tools\0=orphan\0bad name=1\09LIVES=1\0MY_VAR2=ok\0'`)

	got, err := FromLoginShell(t.Context(), shell)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	want := map[string]string{"PATH": "/opt/tools", "MY_VAR2": "ok"}
	if len(got) != len(want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q, want %q", key, got[key], value)
		}
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

func TestAShellCannotFillMemoryWithItsReply(t *testing.T) {
	shell := fakeShell(t, `printf '\nspinoza-environment\nPATH=/opt/tools\0BIG='; head -c 1048577 /dev/zero | tr '\0' x`)

	_, err := FromLoginShell(t.Context(), shell)

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("error = %v, want the bounded reply named", err)
	}
}

func TestABarePathPicksUpTheShellDirectories(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	shell := loginShell(t, `export PATH=/opt/homebrew/bin:/usr/bin`)

	got := Ensure(t.Context(), shell)

	if got != "/usr/bin:/bin:/opt/homebrew/bin" {
		t.Fatalf("path = %q", got)
	}
	if os.Getenv("PATH") != got {
		t.Fatalf("the environment was not updated: %q", os.Getenv("PATH"))
	}
}

func TestABarePathPicksUpTheShellVariablesToo(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	cleared(t, "KUBECONFIG")
	cleared(t, "AWS_PROFILE")
	shell := loginShell(t, `export PATH=/opt/homebrew/bin:/usr/bin; export KUBECONFIG=/home/me/.kube/eks.yaml; export AWS_PROFILE=prod`)

	Ensure(t.Context(), shell)

	if os.Getenv("KUBECONFIG") != "/home/me/.kube/eks.yaml" {
		t.Fatalf("KUBECONFIG = %q, want the login shell's", os.Getenv("KUBECONFIG"))
	}
	if os.Getenv("AWS_PROFILE") != "prod" {
		t.Fatalf("AWS_PROFILE = %q, want the login shell's", os.Getenv("AWS_PROFILE"))
	}
}

func TestAVariableAlreadySetOutranksTheShell(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("KUBECONFIG", "/given/on/launch.yaml")
	shell := loginShell(t, `export PATH=/opt/homebrew/bin:/usr/bin; export KUBECONFIG=/home/me/.kube/eks.yaml`)

	Ensure(t.Context(), shell)

	if os.Getenv("KUBECONFIG") != "/given/on/launch.yaml" {
		t.Fatalf("KUBECONFIG = %q, want the value the process started with", os.Getenv("KUBECONFIG"))
	}
}

func TestTheShellsOwnSessionVariablesStayBehind(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	for _, key := range []string{"SHLVL", "PWD", "OLDPWD", "_"} {
		cleared(t, key)
	}
	shell := loginShell(t, `export PATH=/opt/homebrew/bin:/usr/bin; export SHLVL=7; export PWD=/elsewhere; export OLDPWD=/before`)

	Ensure(t.Context(), shell)

	for _, key := range []string{"SHLVL", "PWD", "OLDPWD", "_"} {
		if value, set := os.LookupEnv(key); set {
			t.Fatalf("%s = %q, want it left unset", key, value)
		}
	}
}

func TestTheAdoptedNamesAreReportedInOrder(t *testing.T) {
	cleared(t, "SPINOZA_TEST_B")
	cleared(t, "SPINOZA_TEST_A")
	t.Setenv("SPINOZA_TEST_SET", "already")

	added := adopt(map[string]string{
		"SPINOZA_TEST_B":   "2",
		"SPINOZA_TEST_A":   "1",
		"SPINOZA_TEST_SET": "ignored",
		"PATH":             "ignored",
		"PWD":              "ignored",
	})

	want := []string{"SPINOZA_TEST_A", "SPINOZA_TEST_B"}
	if !slices.Equal(added, want) {
		t.Fatalf("added = %v, want %v", added, want)
	}
	if os.Getenv("SPINOZA_TEST_SET") != "already" {
		t.Fatalf("SPINOZA_TEST_SET = %q, want it untouched", os.Getenv("SPINOZA_TEST_SET"))
	}
}

func TestAPathThatAlreadyHasToolsIsLeftAlone(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	cleared(t, "KUBECONFIG")
	shell := loginShell(t, `export PATH=/somewhere/else; export KUBECONFIG=/not/taken`)

	got := Ensure(t.Context(), shell)

	if got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("path = %q, want it untouched", got)
	}
	if _, set := os.LookupEnv("KUBECONFIG"); set {
		t.Fatal("a process started from a shell must not ask the login shell for anything")
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
