//go:build desktop

package main

import (
	"bytes"
	"context"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/sophotechlabs/spinoza/internal/server"
)

func main() {
	mgr := makeManager(context.Background())

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		log.Fatalf("assets: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	srv := server.New(mgr, assets)
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

	runErr := wails.Run(&options.App{
		Title:  "Spinoza",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Handler: desktopAssets(assets, addr),
		},
	})
	if runErr != nil {
		log.Fatalf("wails: %v", runErr)
	}
}

func desktopAssets(assets fs.FS, addr string) http.Handler {
	fileServer := http.FileServerFS(assets)
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: addr})
	injected := []byte(`<script>window.__SPINOZA_WS_BASE__="ws://` + addr + `";</script></head>`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			proxy.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, readErr := fs.ReadFile(assets, "index.html")
			if readErr != nil {
				http.Error(w, "index missing", http.StatusInternalServerError)
				return
			}
			out := bytes.Replace(data, []byte("</head>"), injected, 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(out)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
