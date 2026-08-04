//go:build desktop

package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/server"
)

func main() {
	err := runDesktop()
	if err != nil {
		log.Fatalf("spinoza: %v", err)
	}
}

func runDesktop() error {
	opts, flagErr := settingsFromArgs()
	if flagErr != nil {
		return flagErr
	}

	ctx, stop := context.WithCancel(context.Background())
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

	srv := server.New(clusters, assets, token)
	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		serveErr := httpServer.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("server: %v", serveErr)
		}
	}()

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
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
	})
	if runErr != nil {
		return fmt.Errorf("wails: %w", runErr)
	}
	return nil
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
