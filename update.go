package main

import (
	"github.com/sophotechlabs/spinoza/internal/update"
	"github.com/sophotechlabs/spinoza/internal/version"
)

func updateChecker() *update.Checker {
	return update.New(version.String(), "")
}
