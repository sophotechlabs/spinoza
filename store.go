package main

import (
	"log/slog"

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

func allowNodeShell(flagged bool, store *settingsstore.Store) func() bool {
	if flagged {
		return func() bool { return true }
	}
	return func() bool {
		return store.On(settingsstore.NodeShellKey)
	}
}
