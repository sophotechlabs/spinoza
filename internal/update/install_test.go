package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const script = "#!/bin/sh\necho installed\n"

type call struct {
	script string
	dir    string
}

func servingStatus(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func installerFor(t *testing.T, dir string, ran *call, fail error) *Installer {
	t.Helper()
	one := NewInstaller("v1.0.0", servingStatus(t, script, http.StatusOK).URL)
	one.locate = func() (string, error) {
		return filepath.Join(dir, "spinoza"), nil
	}
	one.run = func(_ context.Context, name, into string) ([]byte, error) {
		ran.script = name
		ran.dir = into
		return []byte("installed\n"), fail
	}
	return one
}

func TestInstallingRunsTheScriptAgainstThisBinarysDirectory(t *testing.T) {
	dir := t.TempDir()
	var ran call
	one := installerFor(t, dir, &ran, nil)

	if err := one.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	if ran.dir != dir {
		t.Fatalf("installed into %q, want the directory this binary is in", ran.dir)
	}
	if !strings.HasSuffix(ran.script, ".sh") {
		t.Fatalf("ran %q, want the saved script", ran.script)
	}
}

// The script is fetched, saved and handed to sh by path, and taken away after.
func TestTheSavedScriptIsRemovedAfterwards(t *testing.T) {
	dir := t.TempDir()
	var ran call
	one := installerFor(t, dir, &ran, nil)

	if err := one.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := os.Stat(ran.script); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the script is still at %s", ran.script)
	}
}

func TestTheScriptThatRanIsTheOneTheSiteServed(t *testing.T) {
	dir := t.TempDir()
	var body atomic.Value
	one := installerFor(t, dir, &call{}, nil)
	one.run = func(_ context.Context, name, _ string) ([]byte, error) {
		held, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		body.Store(string(held))
		return nil, nil
	}

	if err := one.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	if body.Load() != script {
		t.Fatalf("ran %q, want what the site served", body.Load())
	}
}

// A symlinked name has to keep working afterwards, so the file it points at is
// what gets replaced.
func TestASymlinkedBinaryIsFollowedToTheRealDirectory(t *testing.T) {
	actual := t.TempDir()
	linked := t.TempDir()
	if err := os.WriteFile(filepath.Join(actual, "spinoza"), []byte("binary"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(filepath.Join(actual, "spinoza"), filepath.Join(linked, "spinoza")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	var ran call
	one := installerFor(t, linked, &ran, nil)

	if err := one.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}

	want, _ := filepath.EvalSymlinks(actual)
	if ran.dir != want {
		t.Fatalf("installed into %q, want the directory the link points at", ran.dir)
	}
}

func TestABinaryThatCannotBeFoundIsReported(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.locate = func() (string, error) {
		return "", errors.New("no argv[0]")
	}

	err := one.Install(context.Background())

	if err == nil {
		t.Fatal("a binary that could not be found installed anyway")
	}
}

// Windows has no path through install.sh at all.
func TestASystemWithNoInstallScriptIsNotOffered(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.goos = "windows"

	err := one.Install(context.Background())

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want the unsupported error", err)
	}
}

func TestADirectoryThatCannotBeWrittenIsNotAttempted(t *testing.T) {
	var ran call
	one := installerFor(t, t.TempDir(), &ran, nil)
	one.writable = func(string) error {
		return errors.New("/usr/local/bin is not writable")
	}

	err := one.Install(context.Background())

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want the unsupported error", err)
	}
	if ran.dir != "" {
		t.Fatal("the script ran despite the directory being unwritable")
	}
}

func TestWhatTheScriptSaidComesBackWithTheFailure(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, errors.New("exit status 1"))
	one.run = func(context.Context, string, string) ([]byte, error) {
		return []byte("Downloading spinoza\ninstall: checksum did not match\n"), errors.New("exit status 1")
	}

	err := one.Install(context.Background())

	if err == nil {
		t.Fatal("a failing script was reported as a success")
	}
	if !strings.Contains(err.Error(), "checksum did not match") {
		t.Fatalf("error = %q, want the last thing the script said", err.Error())
	}
}

func TestAScriptThatSaidNothingStillReportsAFailure(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.run = func(context.Context, string, string) ([]byte, error) {
		return nil, errors.New("signal: killed")
	}

	err := one.Install(context.Background())

	if err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatalf("error = %v, want it to say there was no output", err)
	}
}

func TestASiteThatWillNotServeTheScriptIsReported(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.script = servingStatus(t, "nope", http.StatusServiceUnavailable).URL

	err := one.Install(context.Background())

	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %v, want the status the site answered", err)
	}
}

func TestASiteThatCannotBeReachedIsReported(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.script = "http://127.0.0.1:1/install.sh"

	if err := one.Install(context.Background()); err == nil {
		t.Fatal("an unreachable site installed anyway")
	}
}

func TestAScriptURLThatIsNotOneIsReported(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.script = "://"

	if err := one.Install(context.Background()); err == nil {
		t.Fatal("an unparseable url installed anyway")
	}
}

// Two presses. The second is told rather than starting a second install over
// the first one's files.
func TestASecondInstallWhileOneIsRunningIsRefused(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.run = func(context.Context, string, string) ([]byte, error) {
		close(started)
		<-release
		return nil, nil
	}

	var group sync.WaitGroup
	group.Go(func() {
		_ = one.Install(context.Background())
	})
	<-started
	second := one.Install(context.Background())
	close(release)
	group.Wait()

	if !errors.Is(second, ErrBusy) {
		t.Fatalf("error = %v, want the busy error", second)
	}
}

func TestNoScriptMeansTheProjectsOwn(t *testing.T) {
	if got := NewInstaller("v1.0.0", "").script; got != Script {
		t.Fatalf("script = %q, want %q", got, Script)
	}
}

func TestTheDefaultInstallerLooksAtThisMachine(t *testing.T) {
	one := NewInstaller("v1.0.0", "")

	if one.goos == "" || one.locate == nil || one.run == nil || one.writable == nil {
		t.Fatalf("installer = %+v, want it wired to the machine it runs on", one)
	}
}

func TestAWritableDirectoryIsAccepted(t *testing.T) {
	if err := writableDir(t.TempDir()); err != nil {
		t.Fatalf("a temporary directory was called unwritable: %v", err)
	}
}

func TestADirectoryThatIsNotThereIsNotWritable(t *testing.T) {
	if err := writableDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a directory that does not exist was called writable")
	}
}

// The directory and the skip go through the environment, so a path with a space
// or a quote in it stays one argument.
func TestTheScriptIsToldWhereToInstallAndToLeaveTheAppAlone(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")
	saved := filepath.Join(dir, "probe.sh")
	body := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$SPINOZA_INSTALL_DIR\" \"$SPINOZA_SKIP_APP\" > " + out + "\n"
	if err := os.WriteFile(saved, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := runScript(context.Background(), saved, "/somewhere else"); err != nil {
		t.Fatalf("run: %v", err)
	}

	held, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(held) != "/somewhere else\n1\n" {
		t.Fatalf("the script was told %q", string(held))
	}
}

func TestTheLastLineIsWhatIsReported(t *testing.T) {
	cases := map[string]string{
		"one\ntwo\n":   "two",
		"only":         "only",
		"trailing\n":   "trailing",
		"":             "no output",
		"\r\n":         "no output",
		"a\nb\nlast\r": "last",
	}
	for output, want := range cases {
		if got := lastLine([]byte(output)); got != want {
			t.Errorf("lastLine(%q) = %q, want %q", output, got, want)
		}
	}
}
