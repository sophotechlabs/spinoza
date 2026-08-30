package main

import (
	"context"
	"log/slog"

	"github.com/sophotechlabs/spinoza/internal/baseline"
	"github.com/sophotechlabs/spinoza/internal/history"
	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
)

func settingsStore() *settingsstore.Store {
	path, err := settingsstore.DefaultPath()
	if err != nil {
		slog.Warn("settings will not be kept", "error", err)
		return settingsstore.Memory()
	}
	store, openErr := settingsstore.Open(path)
	if openErr != nil {
		slog.Warn("the stored settings could not be read", "error", openErr)
	}
	return store
}

func baselineStore() *baseline.Store {
	dir, err := baseline.DefaultDir()
	if err != nil {
		slog.Warn("audit baselines will not be kept", "error", err)
		return baseline.Open("")
	}
	return baseline.Open(dir)
}

func historyStore(ctx context.Context) *history.Store {
	path, err := history.DefaultPath()
	if err != nil {
		slog.Warn("spinoza will not record what it does", "error", err)
	}
	store, openErr := history.Open(ctx, path)
	if openErr != nil {
		slog.Warn("spinoza will not record what it does", "error", openErr)
	}
	return store
}

func allowNodeShell(flagged bool, store *settingsstore.Store) func() bool {
	if flagged {
		return func() bool { return true }
	}
	return func() bool {
		return store.On(settingsstore.NodeShellKey)
	}
}
