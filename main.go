//go:build !desktop

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sophotechlabs/spinoza/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("spinoza: %v", err)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:34115", "listen address")
	openBrowser := flag.Bool("open", false, "open the default browser on start")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr := makeManager(ctx)

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	srv := server.New(mgr, assets)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
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

	err = httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
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
