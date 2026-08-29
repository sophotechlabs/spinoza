package main

import (
	_ "embed"
	"io"

	"github.com/sophotechlabs/spinoza/internal/version"
)

//go:embed LICENSE
var licenseText string

func printedNotice(out io.Writer, opts settings) bool {
	if opts.showVersion {
		_, _ = io.WriteString(out, version.String()+"\n")
		return true
	}
	if opts.showLicense {
		_, _ = io.WriteString(out, licenseText)
		return true
	}
	return false
}
