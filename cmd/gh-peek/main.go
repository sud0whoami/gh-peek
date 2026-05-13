// Package main is the gh-peek executable entrypoint.
//
// The binary is named gh-peek so it works both as a standalone
// executable and as a `gh` extension (`gh peek`).
package main

import (
	"flag"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/sud0whoami/gh-peek/internal/app"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, term.IsTerminal(int(os.Stdout.Fd()))))
}

// run is the testable entrypoint: parses flags from args and dispatches
// either to the interactive TUI or the non-interactive --web path.
func run(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("gh-peek", flag.ContinueOnError)
	fs.SetOutput(stderr)
	web := fs.Bool("web", false, "open the repository's GitHub Actions page in a browser and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *web {
		return app.RunWeb(stdout, stderr)
	}
	return app.Run(stdout, stderr, isTTY)
}
