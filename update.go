package main

import (
	"github.com/sophotechlabs/spinoza/internal/update"
	"github.com/sophotechlabs/spinoza/internal/version"
)

// updateChecker is what answers /api/update. A run started with the check off
// still has one, so that the answer says it was turned off rather than leaving
// the browser to guess.
func updateChecker(opts settings) *update.Checker {
	if !opts.updateCheck {
		return update.Off(version.String())
	}
	return update.New(version.String(), opts.updateFrom)
}
