package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/mcp"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, mcp.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "spinoza-mcp: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	opts, err := mcp.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if opts.UnsafeRawOutput {
		slog.Warn("unsafe raw MCP output is enabled; arbitrary log lines and Helm values can contain credentials that redaction misses")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clusters, err := cluster.New(ctx, cluster.Options{
		Kubeconfig:  opts.Kubeconfig,
		PromSpec:    opts.PromSpec,
		SyncTimeout: opts.SyncWait,
		NodeShell:   func() bool { return false },
	})
	if err != nil {
		return err
	}
	if opts.Context != "" {
		if err := clusters.Use(api.ContextRef{Name: opts.Context}); err != nil {
			return err
		}
	}
	backend := clusters.Manager("")
	if backend == nil {
		return errors.New("no cluster answered; name a context with -context")
	}
	server := mcp.New(logReader{Backend: backend}, optionsFor(clusters, opts, mcp.PromFor(clusters.Current(), opts)))
	return server.Dispatch(ctx, opts, os.Stdin, os.Stdout)
}
