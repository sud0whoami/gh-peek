// Package main is the gh-peek executable entrypoint.
//
// The binary is named gh-peek so it works both as a standalone
// executable and as a `gh` extension (`gh actions-tui`).
package main

import (
	"os"

	"golang.org/x/term"

	"github.com/sud0whoami/gh-peek/internal/app"
)

func main() {
	os.Exit(app.Run(os.Stdout, os.Stderr, term.IsTerminal(int(os.Stdout.Fd()))))
}
