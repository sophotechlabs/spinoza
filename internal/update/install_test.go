package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const script = "#!/bin/sh\n# reads SPINOZA_SKIP_APP\necho installed\n"

type call struct {
	script string
	dir    string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type failedRead struct{}

func (failedRead) Read([]byte) (int, error) {
	return 0, errors.New("response body broke")
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

func TestASavedScriptKeepsItsExactBodyAndExecutableMode(t *testing.T) {
	path, err := saveScript([]byte(script))
	if err != nil {
		t.Fatalf("saveScript: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(body) != script {
		t.Fatalf("script = %q, want the fetched body unchanged", body)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if info.Mode().Perm() != scriptMode {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), scriptMode)
	}
}

func TestAScriptThatCannotBeSavedIsNotRun(t *testing.T) {
	root := t.TempDir()
	installDir := t.TempDir()
	missing := filepath.Join(root, "missing")
	t.Setenv("TMPDIR", missing)
	var ran call
	one := installerFor(t, installDir, &ran, nil)

	err := one.Install(t.Context())

	if err == nil {
		t.Fatal("an install with nowhere to save its script reported success")
	}
	if ran.dir != "" {
		t.Fatal("the install script ran without being saved")
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

func TestAScriptResponseThatBreaksWhileReadingIsReported(t *testing.T) {
	one := NewInstaller("v1.0.0", "https://spinoza.example/install.sh")
	one.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(failedRead{}),
			Request:    request,
		}, nil
	})}

	_, err := one.fetch(t.Context())

	if err == nil || !strings.Contains(err.Error(), "response body broke") {
		t.Fatalf("error = %v, want the body read failure", err)
	}
}

func TestAFailedInstallReleasesTheInstallerForARetry(t *testing.T) {
	var ran call
	one := installerFor(t, t.TempDir(), &ran, nil)
	one.script = "http://127.0.0.1:1/install.sh"

	if err := one.Install(t.Context()); err == nil {
		t.Fatal("the unreachable first install reported success")
	}
	one.script = servingStatus(t, script, http.StatusOK).URL

	if err := one.Install(t.Context()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if ran.dir == "" {
		t.Fatal("the retry never ran")
	}
}

func TestAScriptURLThatIsNotOneIsReported(t *testing.T) {
	one := installerFor(t, t.TempDir(), &call{}, nil)
	one.script = "://"

	if err := one.Install(context.Background()); err == nil {
		t.Fatal("an unparseable url installed anyway")
	}
}

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

func TestAScriptThatDoesNotTakeTheSkipIsRefused(t *testing.T) {
	var ran call
	one := installerFor(t, t.TempDir(), &ran, nil)
	one.script = servingStatus(t, "#!/bin/sh\necho old\n", http.StatusOK).URL

	err := one.Install(context.Background())

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want the unsupported error", err)
	}
	if ran.dir != "" {
		t.Fatal("an older script ran anyway")
	}
}

func TestAScriptAtTheSizeLimitStillRuns(t *testing.T) {
	var ran call
	one := installerFor(t, t.TempDir(), &ran, nil)
	body := "#!/bin/sh\n# reads " + skipApp + "\n"
	body += strings.Repeat("x", maxScript-len(body))
	one.script = servingStatus(t, body, http.StatusOK).URL

	if err := one.Install(context.Background()); err != nil {
		t.Fatalf("install: %v", err)
	}
	if ran.dir == "" {
		t.Fatal("a script at the size limit did not run")
	}
}

func TestAnOversizedScriptIsRefusedRatherThanRunTruncated(t *testing.T) {
	var ran call
	one := installerFor(t, t.TempDir(), &ran, nil)
	body := "#!/bin/sh\n# reads " + skipApp + "\n" + strings.Repeat("x", maxScript)
	one.script = servingStatus(t, body, http.StatusOK).URL

	err := one.Install(context.Background())

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("error = %v, want the oversized script refused", err)
	}
	if ran.dir != "" {
		t.Fatal("the truncated script ran")
	}
}
