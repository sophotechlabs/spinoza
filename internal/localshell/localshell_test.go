package localshell

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readFor(t *testing.T, session *Session, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var seen bytes.Buffer
	buf := make([]byte, 1024)
	for time.Now().Before(deadline) {
		read, err := session.Read(buf)
		if read > 0 {
			seen.Write(buf[:read])
			if strings.Contains(seen.String(), want) {
				return seen.String()
			}
		}
		if err != nil {
			break
		}
	}
	return seen.String()
}

func TestAShellEchoesWhatItIsGiven(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()

	_, writeErr := session.Write([]byte("echo spinoza-was-here\n"))
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	seen := readFor(t, session, "spinoza-was-here")
	if !strings.Contains(seen, "spinoza-was-here") {
		t.Fatalf("the shell never echoed the command, saw %q", seen)
	}
}

func TestTheShellStartsWhereItWasTold(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	session, startErr := Start(context.Background(), Options{Shell: "/bin/sh", Dir: dir})
	if startErr != nil {
		t.Fatalf("start: %v", startErr)
	}
	defer session.Close()

	_, _ = session.Write([]byte("pwd\n"))

	seen := readFor(t, session, resolved)
	if !strings.Contains(seen, resolved) {
		t.Fatalf("the shell did not start in %s, saw %q", resolved, seen)
	}
}

func TestTheShellIsToldItIsATerminal(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()

	_, _ = session.Write([]byte("printf '%s\\n' \"$TERM\"\n"))

	seen := readFor(t, session, "xterm-256color")
	if !strings.Contains(seen, "xterm-256color") {
		t.Fatalf("TERM was not set, saw %q", seen)
	}
}

func TestTheCallerCanAddToTheEnvironment(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh", Env: []string{"SPINOZA_TEST=yes"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()

	_, _ = session.Write([]byte("printf '%s\\n' \"$SPINOZA_TEST\"\n"))

	seen := readFor(t, session, "yes")
	if !strings.Contains(seen, "yes") {
		t.Fatalf("the extra environment did not arrive, saw %q", seen)
	}
}

func drain(session *Session) {
	buf := make([]byte, 1024)
	for {
		_, err := session.Read(buf)
		if err != nil {
			return
		}
	}
}

func TestASessionReportsWhenTheShellLeaves(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	go drain(session)

	_, _ = session.Write([]byte("exit 0\n"))

	select {
	case leftErr := <-session.Done():
		if leftErr != nil {
			t.Fatalf("a clean exit reported %v", leftErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the session never reported the shell leaving")
	}
}

func TestAFailingShellIsNotAnError(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	go drain(session)

	_, _ = session.Write([]byte("exit 3\n"))

	select {
	case leftErr := <-session.Done():
		if leftErr != nil {
			t.Fatalf("a non-zero exit reported %v", leftErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the session never reported the shell leaving")
	}
}

func TestReadingAClosedSessionEndsInsteadOfFailing(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	session.Close()

	buf := make([]byte, 16)
	for {
		_, readErr := session.Read(buf)
		if readErr == nil {
			continue
		}
		if !errors.Is(readErr, io.EOF) {
			t.Fatalf("reading a closed session gave %v", readErr)
		}
		return
	}
}

func TestClosingTwiceIsHarmless(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	session.Close()
	session.Close()
}

func TestResizingASessionIsAccepted(t *testing.T) {
	session, err := Start(context.Background(), Options{Shell: "/bin/sh", Size: Size{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()

	session.Resize(Size{Cols: 120, Rows: 40})
	session.Resize(Size{})

	_, writeErr := session.Write([]byte("printf ready\\n\n"))
	if writeErr != nil {
		t.Fatalf("the session stopped taking input after a resize: %v", writeErr)
	}
}

func TestAShellThatCannotStartIsReported(t *testing.T) {
	_, err := Start(context.Background(), Options{Shell: filepath.Join(t.TempDir(), "not-a-shell")})
	if err == nil {
		t.Fatal("starting a missing shell was not reported")
	}
	if !strings.Contains(err.Error(), "not-a-shell") {
		t.Fatalf("the error does not name the shell: %v", err)
	}
}

func TestTheShellPathFollowsTheEnvironment(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	if ShellPath() != "/bin/sh" {
		t.Fatalf("SHELL was ignored, got %s", ShellPath())
	}
}

func TestAnUnusableShellVariableFallsBack(t *testing.T) {
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "gone"))

	path := ShellPath()
	if path != preferredShell && path != fallbackShell {
		t.Fatalf("an unusable SHELL gave %s", path)
	}
}

func TestADirectoryIsNotAShell(t *testing.T) {
	t.Setenv("SHELL", t.TempDir())

	if ShellPath() == os.Getenv("SHELL") {
		t.Fatal("a directory was accepted as a shell")
	}
}

func TestTheDefaultsFillThemselvesIn(t *testing.T) {
	opts := Options{}.orDefaults()

	if opts.Shell == "" {
		t.Fatal("no shell was chosen")
	}
	if opts.Dir == "" {
		t.Fatal("no directory was chosen")
	}
	if opts.Size.Cols == 0 || opts.Size.Rows == 0 {
		t.Fatalf("the terminal has no size: %+v", opts.Size)
	}
}
