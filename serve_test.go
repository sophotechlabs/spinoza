//go:build !desktop

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/klog/v2"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/store"
)

type wiredMode struct {
	calls []string
	tabs  []store.Tab
	err   error
}

type idleExitRecorder struct {
	quit func()
}

func useServerArgs(t *testing.T, args ...string) {
	t.Helper()
	original := os.Args
	os.Args = append([]string{"spinoza"}, args...)
	t.Cleanup(func() {
		os.Args = original
	})
}

func preserveServerLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
		klog.SetSlogLogger(previous)
	})
}

func TestRunReportsAnInvalidLogLevel(t *testing.T) {
	useServerArgs(t, "-log-level", "verbose")

	err := run()

	if err == nil || !strings.Contains(err.Error(), "log level") {
		t.Fatalf("error = %v, want the invalid log level", err)
	}
}

func TestRunRefusesANonLoopbackLocalAddress(t *testing.T) {
	preserveServerLogger(t)
	useServerArgs(t, "-addr", "0.0.0.0:34115")

	err := run()

	if err == nil || !strings.Contains(err.Error(), "not loopback") {
		t.Fatalf("error = %v, want the loopback boundary", err)
	}
}

func TestRunReportsAnExplicitContextThatCannotOpen(t *testing.T) {
	preserveServerLogger(t)
	config := filepath.Join(t.TempDir(), "missing-kubeconfig")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	useServerArgs(t, "-kubeconfig", config, "-context", "missing")

	err := run()

	if err == nil || !strings.Contains(err.Error(), config) {
		t.Fatalf("error = %v, want kubeconfig %q named", err, config)
	}
}

func TestRunRemovesItsTokenAfterServerStartupFails(t *testing.T) {
	preserveServerLogger(t)
	configDir := t.TempDir()
	config := filepath.Join(configDir, "missing-kubeconfig")
	token := filepath.Join(configDir, "access-token")
	t.Setenv("XDG_CONFIG_HOME", configDir)
	useServerArgs(
		t,
		"-addr", "127.0.0.1:not-a-port",
		"-kubeconfig", config,
		"-token-file", token,
	)

	err := run()

	if err == nil || !strings.Contains(err.Error(), "server:") {
		t.Fatalf("error = %v, want the listener startup failure", err)
	}
	if _, statErr := os.Stat(token); !os.IsNotExist(statErr) {
		t.Fatalf("token file error = %v, want it removed", statErr)
	}
}

func (r *idleExitRecorder) UseIdleExit(quit func()) {
	r.quit = quit
}

func (w *wiredMode) UseClusterAuth(server.ClusterAuth) {
	w.calls = append(w.calls, "authentication")
}

func (w *wiredMode) RestoreTabs(ctx context.Context, held server.Tabs) {
	w.calls = append(w.calls, "timeline")
	w.tabs, w.err = held.All(ctx)
}

func (w *wiredMode) UseUpdates(server.Updates) {
	w.calls = append(w.calls, "updates")
}

func (w *wiredMode) UseInstaller(server.Installs) {
	w.calls = append(w.calls, "installer")
}

func storedTimeline(t *testing.T) *store.Store {
	t.Helper()
	past, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	t.Cleanup(func() { _ = past.Close() })
	tabs := past.Tabs()
	rememberErr := tabs.Remember(t.Context(), store.Tab{ID: "cluster-1", Context: "in-cluster"})
	if rememberErr != nil {
		t.Fatalf("remember cluster: %v", rememberErr)
	}
	recordingErr := tabs.Recording(t.Context(), "cluster-1", "workloads")
	if recordingErr != nil {
		t.Fatalf("remember timeline: %v", recordingErr)
	}
	return past
}

func TestClusterModeRestoresItsPersistedTimeline(t *testing.T) {
	past := storedTimeline(t)
	wired := &wiredMode{}

	err := wireMode(t.Context(), wired, servedOpts(nil), past)
	if err != nil {
		t.Fatalf("wire cluster mode: %v", err)
	}
	if wired.err != nil {
		t.Fatalf("read restored tabs: %v", wired.err)
	}
	if strings.Join(wired.calls, ",") != "authentication,timeline" {
		t.Fatalf("wiring order = %v, want authentication before the timeline", wired.calls)
	}
	if len(wired.tabs) != 1 || wired.tabs[0].Timeline != "workloads" {
		t.Fatalf("restored tabs = %+v, want the persisted timeline", wired.tabs)
	}
}

func TestClusterModeDoesNotRestoreTimelineStateWhenAuthenticationCannotStart(t *testing.T) {
	wired := &wiredMode{}
	opts := servedOpts(func(opts *settings) {
		opts.serve.auth.Mode = "ldap"
	})

	err := wireMode(t.Context(), wired, opts, storedTimeline(t))
	if err == nil {
		t.Fatal("cluster mode restored state after authentication failed")
	}
	if len(wired.calls) != 0 {
		t.Fatalf("wiring continued with calls %v after authentication failed", wired.calls)
	}
}

func TestLocalModeStillRestoresTabsBeforeItsUpdateServices(t *testing.T) {
	wired := &wiredMode{}

	err := wireMode(t.Context(), wired, settings{}, storedTimeline(t))
	if err != nil {
		t.Fatalf("wire local mode: %v", err)
	}
	if strings.Join(wired.calls, ",") != "timeline,updates,installer" {
		t.Fatalf("wiring order = %v, want the timeline before local services", wired.calls)
	}
}

func TestLocalListeningStopsAfterTheLastViewLeaves(t *testing.T) {
	recorded := &idleExitRecorder{}
	idle := make(chan struct{})

	announceListening(t.Context(), recorded, settings{addr: "127.0.0.1:34115"}, "secret", idle)

	if recorded.quit == nil {
		t.Fatal("local listening did not register an idle exit")
	}
	recorded.quit()
	recorded.quit()
	select {
	case <-idle:
	default:
		t.Fatal("the last view leaving did not stop local listening")
	}
}

func TestClusterListeningDoesNotStopWhenThereAreNoViews(t *testing.T) {
	recorded := &idleExitRecorder{}
	idle := make(chan struct{})

	announceListening(t.Context(), recorded, servedOpts(nil), "", idle)

	if recorded.quit != nil {
		t.Fatal("cluster listening registered a local idle exit")
	}
	select {
	case <-idle:
		t.Fatal("cluster listening stopped without a view")
	default:
	}
}

func servedOpts(change func(*settings)) settings {
	opts := settings{}
	opts.serve = serving{
		on:          true,
		publicURL:   "https://spinoza.example.com",
		impersonate: true,
		auth: auth.Config{
			Mode:      auth.ModeProxy,
			PublicURL: "https://spinoza.example.com",
			Proxy:     auth.ProxyConfig{SharedSecret: auth.NewSecret()},
		},
	}
	if change != nil {
		change(&opts)
	}
	return opts
}

func TestServingTurnsTheServerIntoOneThatAsksWhoYouAre(t *testing.T) {
	srv := server.New(nil, nil, "")

	err := serveTeam(t.Context(), srv, servedOpts(nil))
	if err != nil {
		t.Fatalf("setting up cluster mode: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", http.NoBody)
	req.Host = "spinoza.example.com"
	recorded := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorded, req)

	var found api.Session
	if decodeErr := json.Unmarshal(recorded.Body.Bytes(), &found); decodeErr != nil {
		t.Fatalf("decoding the session: %v", decodeErr)
	}
	if !found.Cluster || found.Mode != auth.ModeProxy {
		t.Fatalf("session = %+v, want it serving a cluster behind a proxy", found)
	}
	if found.Authenticated {
		t.Fatal("a request with no identity on it read as signed in")
	}
}

func TestServingWithExplicitAnonymousAdminAccessStarts(t *testing.T) {
	opts := servedOpts(func(opts *settings) {
		opts.serve.auth.Mode = auth.ModeNone
		opts.serve.auth.AllowAnonymous = true
		opts.serve.impersonate = false
	})

	if err := serveTeam(t.Context(), server.New(nil, nil, ""), opts); err != nil {
		t.Fatalf("setting up cluster mode: %v", err)
	}
}

func TestAnAuthConfigThatCannotWorkStopsTheServerStarting(t *testing.T) {
	opts := servedOpts(func(opts *settings) {
		opts.serve.auth.Mode = "ldap"
	})

	err := serveTeam(t.Context(), server.New(nil, nil, ""), opts)
	if err == nil {
		t.Fatal("cluster mode started with an auth mode nobody implements")
	}
	if !strings.Contains(err.Error(), "ldap") {
		t.Fatalf("error = %q, want it to name the mode", err.Error())
	}
}

func TestOnlyALocalRunHasToBindLoopback(t *testing.T) {
	local := settings{addr: "0.0.0.0:8080"}

	if err := checkListen(local); err == nil {
		t.Fatal("a local run was allowed to bind every interface")
	}
	if err := checkListen(servedOpts(func(opts *settings) { opts.addr = "0.0.0.0:8080" })); err != nil {
		t.Fatalf("a served run was refused its own address: %v", err)
	}
}

func TestOnlyALocalRunMintsAToken(t *testing.T) {
	served, err := runToken(servedOpts(nil))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if served != "" {
		t.Fatal("a served spinoza minted a run token nobody would ever be given")
	}

	path := filepath.Join(t.TempDir(), "token")
	token, localErr := runToken(settings{tokenFile: path})
	if localErr != nil {
		t.Fatalf("minting: %v", localErr)
	}
	if token == "" {
		t.Fatal("a local run minted no token")
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the token file: %v", readErr)
	}
	if strings.TrimSpace(string(body)) != token {
		t.Fatalf("token file holds %q, want %q", strings.TrimSpace(string(body)), token)
	}
}

func TestATokenFileThatCannotBeWrittenStopsTheRun(t *testing.T) {
	_, err := runToken(settings{tokenFile: filepath.Join(t.TempDir(), "no", "such", "dir", "token")})

	if err == nil {
		t.Fatal("a token file spinoza could not write was ignored")
	}
}

func TestWhoHelmAndKubectlActAs(t *testing.T) {
	inCluster := func() bool { return true }
	elsewhere := func() bool { return false }
	dir := func() (string, error) { return "/state", nil }
	noDir := func() (string, error) { return "", errors.New("no config directory") }
	wrote := func(string) (string, error) { return "/state/spinoza/in-cluster.kubeconfig", nil }
	broke := func(string) (string, error) { return "", errors.New("read-only") }

	cases := []struct {
		name string
		opts settings
		in   func() bool
		dir  func() (string, error)
		with func(string) (string, error)
		want string
	}{
		{
			name: "a local run needs none",
			opts: settings{},
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "a served run outside a cluster reads the kubeconfig it was given",
			opts: servedOpts(nil),
			in:   elsewhere,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "a kubeconfig of your own wins",
			opts: servedOpts(func(opts *settings) { opts.cluster.Kubeconfig = "/etc/kube.yaml" }),
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "nowhere to write one",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  noDir,
			with: wrote,
			want: "",
		},
		{
			name: "writing one failed",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  dir,
			with: broke,
			want: "",
		},
		{
			name: "in a pod, with somewhere to write it",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "/state/spinoza/in-cluster.kubeconfig",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolKubeconfigFrom(tc.opts, tc.in, tc.dir, tc.with); got != tc.want {
				t.Fatalf("kubeconfig = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAServerNotServingAClusterWritesNoKubeconfig(t *testing.T) {
	if got := toolKubeconfig(settings{}); got != "" {
		t.Fatalf("kubeconfig = %q, want none when spinoza runs as your own window", got)
	}
}
