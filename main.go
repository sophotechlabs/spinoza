//go:build !desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/localshell"
	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/store"
	"github.com/sophotechlabs/spinoza/internal/toolpath"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const shutdownGrace = 3 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("spinoza stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	opts, err := settingsFromArgs()
	if errors.Is(err, errHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if printedNotice(os.Stdout, opts) {
		return nil
	}
	slog.SetDefault(slog.New(logHandler(os.Stderr, opts.logLevel)))
	klog.SetSlogLogger(slog.Default())

	listenErr := checkListen(opts)
	if listenErr != nil {
		return listenErr
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	toolpath.Ensure(ctx, localshell.ShellPath())

	held := settingsStore()
	opts.cluster.NodeShell = allowNodeShell(opts.nodeShell, held)
	opts.cluster.Columns = customColumns(held)
	opts.cluster.ToolKubeconfig = toolKubeconfig(opts)

	clusters, err := cluster.New(ctx, opts.cluster)
	if err != nil {
		return err
	}

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	token, tokenErr := runToken(opts)
	if tokenErr != nil {
		return tokenErr
	}
	if opts.tokenFile != "" {
		defer func() { _ = os.Remove(opts.tokenFile) }()
	}

	srv := server.New(clusters, assets, token)
	srv.StartOn(opts.startView, opts.cluster.Context)
	srv.UseProfiler(opts.pprof)
	srv.UseSettings(held)
	srv.UseBaselines(baselineStore())
	past := historyStore(ctx)
	defer func() { _ = past.Close() }()
	srv.UseHistory(past)
	wiredErr := wireMode(ctx, srv, opts, past)
	if wiredErr != nil {
		return wiredErr
	}
	httpServer := &http.Server{
		Addr:              opts.addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	idle := make(chan struct{})
	announceListening(ctx, srv, opts, token, idle)

	ended := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		select {
		case <-ended:
			return
		case <-idle:
		case <-ctx.Done():
		}
		srv.Close()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	err = httpServer.ListenAndServe()
	close(ended)
	<-drained
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func openURL(ctx context.Context, url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		name = "xdg-open"
		args = []string{url}
	}
	opener := exec.CommandContext(ctx, name, args...)
	if opener.Start() != nil {
		return
	}
	go func() {
		_ = opener.Wait()
	}()
}

type modeServer interface {
	teamServer
	RestoreTabs(ctx context.Context, held server.Tabs)
	UseUpdates(checker server.Updates)
	UseInstaller(installer server.Installs)
}

func wireMode(ctx context.Context, srv modeServer, opts settings, past *store.Store) error {
	if opts.serve.on {
		teamErr := serveTeam(ctx, srv, opts)
		if teamErr != nil {
			return teamErr
		}
	}
	srv.RestoreTabs(ctx, past.Tabs())
	if opts.serve.on {
		return nil
	}
	srv.UseUpdates(updateChecker())
	srv.UseInstaller(updateInstaller())
	return nil
}

func announceListening(ctx context.Context, srv *server.Server, opts settings, token string, idle chan struct{}) {
	if opts.serve.on {
		slog.Info(
			"spinoza is serving this cluster",
			"listen", opts.addr,
			"url", opts.serve.publicURL,
			"auth", opts.serve.auth.Mode,
			"impersonate", opts.serve.impersonate,
			"version", version.String(),
		)
		return
	}
	url := server.BrowserURL(opts.addr, token)
	slog.Info("spinoza is listening, open this in a browser", "url", url, "version", version.String())
	if opts.openBrowser {
		openURL(ctx, url)
	}
	var once sync.Once
	srv.UseIdleExit(func() {
		once.Do(func() {
			slog.Info("every spinoza view is gone, stopping")
			close(idle)
		})
	})
}
