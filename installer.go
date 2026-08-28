//go:build !desktop

package main

import (
	"github.com/sophotechlabs/spinoza/internal/update"
	"github.com/sophotechlabs/spinoza/internal/version"
)

// The desktop build wires none: an app bundle cannot replace itself while it is
// the thing running.
func updateInstaller() *update.Installer {
	return update.NewInstaller(version.String(), "")
}
