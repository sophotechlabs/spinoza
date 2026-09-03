//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"k8s.io/klog/v2"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/localshell"
	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/toolpath"
)

const shutdownGrace = 3 * time.Second

const logFileLimit = 4 << 20

func main() {
	err := runDesktop()
	if err == nil {
		return
	}
	slog.Error("spinoza stopped", "error", err)
	reportStartupFailure(err)
	os.Exit(1)
}

func reportStartupFailure(cause error) {
	runErr := wails.Run(&options.App{
		Title:       "Spinoza",
		Width:       1,
		Height:      1,
		StartHidden: true,
		AssetServer: &assetserver.Options{Handler: blankPage()},
		OnStartup: func(ctx context.Context) {
			go func() {
				_, dialogErr := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
					Type:    wailsruntime.ErrorDialog,
					Title:   "Spinoza could not start",
					Message: cause.Error(),
				})
				if dialogErr != nil {
					slog.Error("the reason could not be shown on screen", "error", dialogErr)
				}
				wailsruntime.Quit(ctx)
			}()
		},
	})
	if runErr != nil {
		slog.Error("the reason could not be shown on screen", "error", runErr)
	}
}

func blankPage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>Spinoza</title>"))
	})
}

func runDesktop() error {
	opts, flagErr := settingsFromArgs()
	if errors.Is(flagErr, errHelp) {
		return nil
	}
	if flagErr != nil {
		return flagErr
	}
	if printedNotice(os.Stdout, opts) {
		return nil
	}
	if opts.serve.on {
		return errors.New("the desktop app cannot serve a cluster; run the spinoza binary with --cluster-mode instead")
	}
	kept := startLogging(opts.logLevel)
	if kept != nil {
		defer func() { _ = kept.Close() }()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	toolpath.Ensure(ctx, localshell.ShellPath())

	store := settingsStore(ctx)
	opts.cluster.NodeShell = allowNodeShell(opts.nodeShell, store)
	opts.cluster.Columns = customColumns(store)

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
	srv.StartOn(opts.startView, opts.cluster.Context)
	srv.UseProfiler(opts.pprof)
	srv.UseSettings(store)
	srv.UseBaselines(baselineStore())
	past := historyStore(ctx)
	defer func() { _ = past.Close() }()
	srv.UseHistory(ctx, past)
	srv.RestoreTabs(ctx, past.Tabs())
	srv.UseUpdates(updateChecker())
	srv.UseLocalShell(localShell)
	useViews(srv, &window, addr, token)
	srv.UseFilePicker(filePicker(&window))
	httpServer := configuredHTTPServer("", srv.Handler())
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
			Handler: desktopAssets(srv, assets, addr, token),
		},
		Mac: &mac.Options{},
		OnStartup: func(windowCtx context.Context) {
			window.Store(&windowCtx)
		},
	})
	if runErr != nil {
		return fmt.Errorf("wails: %w", runErr)
	}
	return nil
}

func startLogging(level slog.Leveler) io.Closer {
	kept, err := logFile()
	out := io.Writer(os.Stderr)
	if err == nil {
		out = io.MultiWriter(os.Stderr, kept)
	}
	slog.SetDefault(slog.New(logHandler(out, level)))
	klog.SetSlogLogger(slog.Default())
	if err != nil {
		slog.Warn("this run will not be written to a log file", "error", err)
		return nil
	}
	return kept
}

func localShell(cols, rows uint16) (server.LocalShell, error) {
	session, err := localshell.Start(context.Background(), localshell.Options{
		Size: localshell.Size{Cols: cols, Rows: rows},
	})
	if err != nil {
		return nil, err
	}
	return shellAdapter{session: session}, nil
}

func filePicker(window *atomic.Pointer[context.Context]) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		ready := window.Load()
		if ready == nil {
			return "", errors.New("the spinoza window is not ready yet")
		}
		//nolint:contextcheck // the dialog belongs to the window, not to the request that asked for it
		return chooseKubeconfig(*ready)
	}
}

func logFile() (*os.File, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	path := filepath.Join(dir, "spinoza", "desktop.log")
	mkErr := os.MkdirAll(filepath.Dir(path), 0o755)
	if mkErr != nil {
		return nil, fmt.Errorf("log: %w", mkErr)
	}
	rollLog(path)
	held, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if openErr != nil {
		return nil, fmt.Errorf("log: %w", openErr)
	}
	return held, nil
}

func rollLog(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Size() < logFileLimit {
		return
	}
	_ = os.Rename(path, path+".1")
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

func desktopAssets(srv *server.Server, assets fs.FS, addr, token string) http.Handler {
	fileServer := http.FileServerFS(assets)
	target := &url.URL{Scheme: "http", Host: addr}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Header.Set(server.AuthHeader, token)
		},
	}
	injected := `<script>window.__SPINOZA_WS_BASE__="ws://` + addr + `";</script>`
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
			_, _ = w.Write(server.InjectHead(data, injected+srv.IndexHead(server.ViewDesktop)))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

type shellAdapter struct {
	session *localshell.Session
}

func (a shellAdapter) Read(p []byte) (int, error) {
	return a.session.Read(p)
}

func (a shellAdapter) Write(p []byte) (int, error) {
	return a.session.Write(p)
}

func (a shellAdapter) Resize(cols, rows uint16) {
	a.session.Resize(localshell.Size{Cols: cols, Rows: rows})
}

func (a shellAdapter) Done() <-chan error {
	return a.session.Done()
}

func (a shellAdapter) Close() {
	a.session.Close()
}

type desktopWindow struct {
	window *atomic.Pointer[context.Context]
}

func (d desktopWindow) Show() {
	ready := d.window.Load()
	if ready == nil {
		return
	}
	wailsruntime.WindowShow(*ready)
}

func (d desktopWindow) Hide() {
	ready := d.window.Load()
	if ready == nil {
		return
	}
	wailsruntime.WindowHide(*ready)
}

func useViews(srv *server.Server, window *atomic.Pointer[context.Context], addr, token string) {
	srv.UseWindow(desktopWindow{window: window})
	srv.UseBrowser(func() error {
		ready := window.Load()
		if ready == nil {
			return errors.New("the spinoza window is not ready yet")
		}
		wailsruntime.BrowserOpenURL(*ready, server.BrowserURL(addr, token))
		return nil
	})
	srv.UseIdleExit(func() {
		ready := window.Load()
		if ready == nil {
			return
		}
		slog.Info("every spinoza view is gone, stopping")
		wailsruntime.Quit(*ready)
	})
}
