package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/baseline"
	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func settingsStore(ctx context.Context) *settingsstore.Store {
	path, err := settingsstore.DefaultPath()
	if err != nil {
		slog.Warn("settings will not be kept", "error", err)
		return settingsstore.Memory()
	}
	held, openErr := settingsstore.Open(ctx, path)
	if openErr != nil {
		slog.Warn("the stored settings could not be read", "error", openErr)
	}
	return held
}

func baselineStore() *baseline.Store {
	dir, err := baseline.DefaultDir()
	if err != nil {
		slog.Warn("audit baselines will not be kept", "error", err)
		return baseline.Open("")
	}
	return baseline.Open(dir)
}

func historyStore(ctx context.Context) *store.Store {
	path, err := store.DefaultPath()
	if err != nil {
		slog.Warn("spinoza will not record what it does", "error", err)
	}
	held, openErr := store.Open(ctx, path)
	if openErr != nil {
		slog.Warn("spinoza will not record what it does", "error", openErr)
	}
	return held
}

func allowNodeShell(flagged bool, held *settingsstore.Store) func() bool {
	if flagged {
		return func() bool { return true }
	}
	return func() bool {
		return held.On(settingsstore.NodeShellKey)
	}
}

func readableNamespaces(held *settingsstore.Store) func() map[string][]string {
	return func() map[string][]string {
		return parseNamespaces(held.All()[settingsstore.NamespacesKey])
	}
}

func parseNamespaces(raw string) map[string][]string {
	if raw == "" {
		return nil
	}
	var held map[string][]string
	err := json.Unmarshal([]byte(raw), &held)
	if err != nil {
		return nil
	}
	return held
}

func customColumns(held *settingsstore.Store) func() map[string][]api.CustomColumn {
	return func() map[string][]api.CustomColumn {
		return api.ParseColumns(held.All()[settingsstore.ColumnsKey])
	}
}
