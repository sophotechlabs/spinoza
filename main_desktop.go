//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const shutdownGrace = 3 * time.Second

func main() {
	err := runDesktop()
	if err != nil {
		slog.Error("spinoza stopped", "error", err)
		os.Exit(1)
	}
}

func runDesktop() error {
	opts, flagErr := settingsFromArgs()
	if errors.Is(flagErr, errHelp) {
		return nil
	}
	if flagErr != nil {
		return flagErr
	}
	if opts.showVersion {
		_, _ = os.Stdout.WriteString(version.String() + "\n")
		return nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: opts.logLevel})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clusters, err := cluster.New(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("manager: %w", err)
	}

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	addr := listener.Addr().String()

	token := server.NewToken()
	tokenErr := writeTokenFile(opts.tokenFile, token)
	if tokenErr != nil {
		return tokenErr
	}
	if opts.tokenFile != "" {
		defer func() { _ = os.Remove(opts.tokenFile) }()
	}

	var window atomic.Pointer[context.Context]
	srv := server.New(clusters, assets, token)
	srv.UseFilePicker(func(context.Context) (string, error) {
		ready := window.Load()
		if ready == nil {
			return "", errors.New("the spinoza window is not ready yet")
		}
		//nolint:contextcheck // the dialog belongs to the window, not to the request that asked for it
		return chooseKubeconfig(*ready)
	})
	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		serveErr := httpServer.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("the desktop backend stopped serving", "error", serveErr)
		}
	}()

	defer func() {
		srv.Close()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	runErr := wails.Run(&options.App{
		Title:            "Spinoza",
		Width:            1280,
		Height:           800,
		BackgroundColour: &options.RGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 255},
		AssetServer: &assetserver.Options{
			Handler: desktopAssets(assets, addr, token),
		},
		OnStartup: func(windowCtx context.Context) {
			window.Store(&windowCtx)
		},
	})
	if runErr != nil {
		return fmt.Errorf("wails: %w", runErr)
	}
	return nil
}

func chooseKubeconfig(window context.Context) (string, error) {
	path, err := wailsruntime.OpenFileDialog(window, wailsruntime.OpenDialogOptions{
		Title:                      "Choose a kubeconfig",
		DefaultDirectory:           kubeDirectory(),
		ShowHiddenFiles:            true,
		ResolvesAliases:            true,
		TreatPackagesAsDirectories: true,
	})
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return path, nil
}

func kubeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube")
}

func desktopAssets(assets fs.FS, addr, token string) http.Handler {
	fileServer := http.FileServerFS(assets)
	target := &url.URL{Scheme: "http", Host: addr}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Header.Set(server.AuthHeader, token)
		},
	}
	injected := `<script>window.__SPINOZA_WS_BASE__="ws://` + addr + `";</script>` + server.TokenScript(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if server.IsBackendPath(r.URL.Path) {
			proxy.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, readErr := fs.ReadFile(assets, "index.html")
			if readErr != nil {
				http.Error(w, "index missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(server.InjectHead(data, injected))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
