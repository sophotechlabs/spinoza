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
	"github.com/sophotechlabs/spinoza/internal/version"
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
	server := mcp.New(logReader{Backend: backend}, mcp.Options{
		Version:    version.String(),
		Context:    clusters.Current().Name,
		Protected:  clusters.Protected(clusters.ID()),
		AllowWrite: opts.AllowWrite,
		Prometheus: mcp.PromFor(clusters.Current(), opts),
		LogLines:   opts.LogLines,
		CallBudget: opts.CallBudget,
	})
	return server.Dispatch(ctx, opts, os.Stdin, os.Stdout)
}
