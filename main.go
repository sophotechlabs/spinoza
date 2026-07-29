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

	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("spinoza: %v", err)
	}
}

func run() error {
	flags := flag.NewFlagSet("spinoza", flag.ExitOnError)
	addr := flags.String("addr", "127.0.0.1:34115", "listen address")
	openBrowser := flags.Bool("open", false, "open the default browser on start")
	debugImage := flags.String("debug-image", debugcontainer.DefaultImage, "image used for debug containers")
	kubectlBinary := flags.String("kubectl", debugcontainer.DefaultBinary, "kubectl binary used to create debug containers")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr, err := makeManager(ctx, *debugImage, *kubectlBinary)
	if err != nil {
		return err
	}

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
