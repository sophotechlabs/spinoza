//go:build desktop

package main

import (
	"bytes"
	"context"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/sophotechlabs/spinoza/internal/server"
)

const desktopAddr = "127.0.0.1:34115"

type desktopApp struct {
	assets fs.FS
}

func (a *desktopApp) startup(ctx context.Context) {
	go func() {
		mgr := makeManager(ctx)
		srv := server.New(mgr, a.assets)
		httpServer := &http.Server{
			Addr:              desktopAddr,
			Handler:           srv.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		listenErr := httpServer.ListenAndServe()
		if listenErr != nil && listenErr != http.ErrServerClosed {
			log.Printf("server: %v", listenErr)
		}
	}()
}

func main() {
	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		log.Fatalf("assets: %v", err)
	}
	app := &desktopApp{assets: assets}
	err = wails.Run(&options.App{
		Title:     "Spinoza",
		Width:     1280,
		Height:    800,
		OnStartup: app.startup,
		AssetServer: &assetserver.Options{
			Handler: desktopAssets(assets),
		},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}

func desktopAssets(assets fs.FS) http.Handler {
	fileServer := http.FileServerFS(assets)
	injected := []byte(`<script>window.__SPINOZA_WS_BASE__="ws://` + desktopAddr + `";</script></head>`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
