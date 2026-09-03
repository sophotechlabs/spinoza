//go:build !desktop

package main

import (
	"log/slog"

	"github.com/sophotechlabs/spinoza/internal/update"
	"github.com/sophotechlabs/spinoza/internal/version"
)

func updateInstaller() *update.Installer {
	return update.NewInstaller(version.String(), selfUpdateScript())
}

func selfUpdateScript() string {
	if !envBool("SPINOZA_UNSAFE_SELF_UPDATE") {
		return ""
	}
	slog.Warn("unsafe self-update enabled; a mutable remote script can execute code with this user's privileges")
	return update.Script
}
