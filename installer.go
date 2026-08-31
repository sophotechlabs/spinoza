//go:build !desktop

package main

import (
	"github.com/sophotechlabs/spinoza/internal/update"
	"github.com/sophotechlabs/spinoza/internal/version"
)

func updateInstaller() *update.Installer {
	return update.NewInstaller(version.String(), "")
}
