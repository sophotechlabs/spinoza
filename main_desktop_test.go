//go:build desktop

package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"k8s.io/klog/v2"

	"github.com/sophotechlabs/spinoza/internal/server"
)

func preserveDesktopLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
		klog.SetSlogLogger(previous)
	})
}

func useDesktopArgs(t *testing.T, args ...string) {
	t.Helper()
	original := os.Args
	os.Args = append([]string{"spinoza"}, args...)
	t.Cleanup(func() {
		os.Args = original
	})
}

func TestRunDesktopReportsAnInvalidLogLevel(t *testing.T) {
	useDesktopArgs(t, "-log-level", "verbose")

	err := runDesktop()

	if err == nil || !strings.Contains(err.Error(), "log level") {
		t.Fatalf("error = %v, want the invalid log level", err)
	}
}

func TestRunDesktopReportsAnUnreadableKubeconfig(t *testing.T) {
	preserveDesktopLogger(t)
	config := filepath.Join(t.TempDir(), "missing-kubeconfig")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	useDesktopArgs(t, "-kubeconfig", config, "-context", "missing")

	err := runDesktop()

	if err == nil || !strings.Contains(err.Error(), "manager:") {
		t.Fatalf("error = %v, want the manager startup failure", err)
	}
	if !strings.Contains(err.Error(), config) {
		t.Fatalf("error = %q, want kubeconfig %q named", err, config)
	}
}

func TestRunDesktopReportsWhenItsTokenCannotBeWritten(t *testing.T) {
	preserveDesktopLogger(t)
	configDir := t.TempDir()
	config := filepath.Join(configDir, "missing-kubeconfig")
	token := filepath.Join(configDir, "missing", "access-token")
	t.Setenv("XDG_CONFIG_HOME", configDir)
	useDesktopArgs(t, "-kubeconfig", config, "-token-file", token)

	err := runDesktop()

	if err == nil || !strings.Contains(err.Error(), "token file:") {
		t.Fatalf("error = %v, want the token file failure", err)
	}
}

func TestBlankPageIsACompleteHTMLResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://desktop/", http.NoBody)

	blankPage().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := recorder.Body.String(); got != "<!doctype html><title>Spinoza</title>" {
		t.Fatalf("body = %q", got)
	}
}

func TestLogFileCreatesThePrivateDesktopLog(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)

	held, openErr := logFile()
	if openErr != nil {
		t.Fatalf("log file: %v", openErr)
	}
	path := held.Name()
	if _, writeErr := held.WriteString("started\n"); writeErr != nil {
		t.Fatalf("write log: %v", writeErr)
	}
	if closeErr := held.Close(); closeErr != nil {
		t.Fatalf("close log: %v", closeErr)
	}

	want := filepath.Join(config, "spinoza", "desktop.log")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(body) != "started\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestLogFileReportsWhenItsDirectoryCannotBeCreated(t *testing.T) {
	config := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(config, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("seed config path: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)

	_, err := logFile()

	if err == nil || !strings.Contains(err.Error(), "log:") {
		t.Fatalf("error = %v, want the log path failure", err)
	}
}

func TestLogFileReportsWhenTheLogPathIsADirectory(t *testing.T) {
	config := t.TempDir()
	path := filepath.Join(config, "spinoza", "desktop.log")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("seed log directory: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)

	_, err := logFile()

	if err == nil || !strings.Contains(err.Error(), "log:") {
		t.Fatalf("error = %v, want the open failure", err)
	}
}

func TestStartLoggingKeepsAFileWhenOneCanBeCreated(t *testing.T) {
	preserveDesktopLogger(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	kept := startLogging(slog.LevelInfo)

	if kept == nil {
		t.Fatal("logging did not keep the file it created")
	}
	if err := kept.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
}

func TestStartLoggingFallsBackWhenNoConfigDirectoryExists(t *testing.T) {
	preserveDesktopLogger(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	read, write, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("capture stderr: %v", pipeErr)
	}
	original := os.Stderr
	os.Stderr = write
	t.Cleanup(func() {
		os.Stderr = original
	})

	kept := startLogging(slog.LevelInfo)
	if closeErr := write.Close(); closeErr != nil {
		t.Fatalf("close stderr: %v", closeErr)
	}
	os.Stderr = original
	warning, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}
	if closeErr := read.Close(); closeErr != nil {
		t.Fatalf("close captured stderr: %v", closeErr)
	}

	if kept != nil {
		_ = kept.Close()
		t.Fatal("logging kept a file without a config directory")
	}
	if !strings.Contains(string(warning), "this run will not be written to a log file") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestRollLogLeavesAMissingFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.log")

	rollLog(path)

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rolled file error = %v, want it not to exist", err)
	}
}

func TestRollLogLeavesASmallFileInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.log")
	if err := os.WriteFile(path, []byte("one run"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	rollLog(path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original log: %v", err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rolled file error = %v, want it not to exist", err)
	}
}

func TestRollLogMovesAFileAtTheLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.log")
	held, createErr := os.Create(path)
	if createErr != nil {
		t.Fatalf("create log: %v", createErr)
	}
	if truncateErr := held.Truncate(logFileLimit); truncateErr != nil {
		_ = held.Close()
		t.Fatalf("size log: %v", truncateErr)
	}
	if closeErr := held.Close(); closeErr != nil {
		t.Fatalf("close log: %v", closeErr)
	}

	rollLog(path)

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("original file error = %v, want it moved", statErr)
	}
	info, statErr := os.Stat(path + ".1")
	if statErr != nil {
		t.Fatalf("rolled log: %v", statErr)
	}
	if info.Size() != logFileLimit {
		t.Fatalf("rolled size = %d, want %d", info.Size(), logFileLimit)
	}
}

func TestKubeDirectoryUsesTheCurrentHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := kubeDirectory()
	want := filepath.Join(home, ".kube")

	if got != want {
		t.Fatalf("directory = %q, want %q", got, want)
	}
}

func TestKubeDirectoryIsEmptyWithoutAHome(t *testing.T) {
	t.Setenv("HOME", "")

	if got := kubeDirectory(); got != "" {
		t.Fatalf("directory = %q, want empty", got)
	}
}

func TestFilePickerRefusesARequestBeforeTheWindowStarts(t *testing.T) {
	var window atomic.Pointer[context.Context]

	_, err := filePicker(&window)(t.Context())

	want := "the spinoza window is not ready yet"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestLocalShellStartsAtTheRequestedSize(t *testing.T) {
	shell, err := localShell(91, 37)
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	if shell == nil {
		t.Fatal("the local shell was nil")
	}
	t.Cleanup(shell.Close)
	shell.Resize(92, 38)
}

func TestDesktopAssetsProxyBackendRequestsWithTheSessionToken(t *testing.T) {
	seenToken := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenToken = r.Header.Get(server.AuthHeader)
		_, _ = w.Write([]byte("backend response"))
	}))
	t.Cleanup(backend.Close)
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}
	service := server.New(nil, assets, "desktop-token")
	addr := strings.TrimPrefix(backend.URL, "http://")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://desktop/api/overview", http.NoBody)

	desktopAssets(service, assets, addr, "desktop-token").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if seenToken != "desktop-token" {
		t.Fatalf("backend token = %q", seenToken)
	}
	if recorder.Body.String() != "backend response" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestDesktopIndexNamesItsBackendAndView(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head><body>Spinoza</body></html>")}}
	service := server.New(nil, assets, "desktop-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://desktop/", http.NoBody)

	desktopAssets(service, assets, "127.0.0.1:4123", "desktop-token").ServeHTTP(recorder, request)

	body := recorder.Body.String()
	for _, want := range []string{
		`window.__SPINOZA_WS_BASE__="ws://127.0.0.1:4123"`,
		`window.__SPINOZA_TOKEN__="desktop-token"`,
		`window.__SPINOZA_VIEW__="desktop"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestDesktopIndexReportsAMissingAsset(t *testing.T) {
	assets := fstest.MapFS{"app.js": {Data: []byte("ready")}}
	service := server.New(nil, assets, "desktop-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://desktop/index.html", http.NoBody)

	desktopAssets(service, assets, "127.0.0.1:4123", "desktop-token").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(recorder.Body.String(), "index missing") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestDesktopAssetsServeStaticFilesWithoutProxying(t *testing.T) {
	assets := fstest.MapFS{"app.js": {Data: []byte("ready")}}
	service := server.New(nil, assets, "desktop-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://desktop/app.js", http.NoBody)

	desktopAssets(service, assets, "127.0.0.1:1", "desktop-token").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "ready" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestDesktopBrowserRefusesBeforeTheWindowStarts(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}
	service := server.New(nil, assets, "desktop-token")
	t.Cleanup(service.Close)
	var window atomic.Pointer[context.Context]
	useViews(service, &window, "127.0.0.1:4123", "desktop-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/view/browser", http.NoBody)
	request.Header.Set(server.AuthHeader, "desktop-token")

	service.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(recorder.Body.String(), "window is not ready") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestDesktopViewCanBeSelectedBeforeWindowStartupCompletes(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}
	service := server.New(nil, assets, "desktop-token")
	t.Cleanup(service.Close)
	var window atomic.Pointer[context.Context]
	useViews(service, &window, "127.0.0.1:4123", "desktop-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/view/desktop", http.NoBody)
	request.Header.Set(server.AuthHeader, "desktop-token")

	service.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"switched":true`) {
		t.Fatalf("body = %q, want the view switch confirmed", recorder.Body.String())
	}
}
