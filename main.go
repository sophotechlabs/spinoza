//go:build !desktop

package main

import (
	"context"
	"errors"
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

	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("spinoza: %v", err)
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

	addrErr := server.CheckLoopback(opts.addr)
	if addrErr != nil {
		return addrErr
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clusters, err := cluster.New(ctx, opts.cluster)
	if err != nil {
		return err
	}

	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	srv := server.New(clusters, assets)
	httpServer := &http.Server{
		Addr:              opts.addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := "http://" + opts.addr
	log.Printf("spinoza listening on %s  (open it in a browser)", url)
	if opts.openBrowser {
		openURL(ctx, url)
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	}()

	err = httpServer.ListenAndServe()
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
	_ = exec.CommandContext(ctx, name, args...).Start()
}
