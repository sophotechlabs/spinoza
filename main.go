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
	if opts.showVersion {
		_, _ = os.Stdout.WriteString(version.String() + "\n")
		return nil
	}
	slog.SetDefault(slog.New(logHandler(os.Stderr, opts.logLevel)))
	klog.SetSlogLogger(slog.Default())

	addrErr := server.CheckLoopback(opts.addr)
	if addrErr != nil {
		return addrErr
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	toolpath.Ensure(ctx, localshell.ShellPath())

	store := settingsStore()
	opts.cluster.NodeShell = allowNodeShell(opts.nodeShell, store)

	clusters, err := cluster.New(ctx, opts.cluster)
	if err != nil {
		return err
	}

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	token := server.NewToken()
	tokenErr := writeTokenFile(opts.tokenFile, token)
	if tokenErr != nil {
		return tokenErr
	}
	if opts.tokenFile != "" {
		defer func() { _ = os.Remove(opts.tokenFile) }()
	}

	srv := server.New(clusters, assets, token)
	srv.UseProfiler(opts.pprof)
	srv.UseSettings(store)
	srv.UseUpdates(updateChecker())
	srv.UseInstaller(updateInstaller())
	httpServer := &http.Server{
		Addr:              opts.addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := server.BrowserURL(opts.addr, token)
	slog.Info("spinoza is listening, open this in a browser", "url", url, "version", version.String())
	if opts.openBrowser {
		openURL(ctx, url)
	}

	idle := make(chan struct{})
	var once sync.Once
	srv.UseIdleExit(func() {
		once.Do(func() {
			slog.Info("every spinoza view is gone, stopping")
			close(idle)
		})
	})

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
