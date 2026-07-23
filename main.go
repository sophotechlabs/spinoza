package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sophotechlabs/spinoza/internal/broker"
	"github.com/sophotechlabs/spinoza/internal/server"
)

//go:embed all:web/dist
var embedded embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:34115", "listen address")
	openBrowser := flag.Bool("open", false, "open the default browser on start")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := broker.NewStub(ctx)

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		log.Fatalf("assets: %v", err)
	}

	srv := server.New(b, assets)
	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
	}

	url := "http://" + *addr
	log.Printf("spinoza listening on %s  (open it in a browser)", url)
	if *openBrowser {
		openURL(url)
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func openURL(url string) {
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
	_ = exec.Command(name, args...).Start()
}
