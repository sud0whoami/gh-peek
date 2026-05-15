// Package main is the gh-peek executable entrypoint.
//
// The binary is named gh-peek so it works both as a standalone
// executable and as a `gh` extension (`gh peek`).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/sud0whoami/gh-peek/internal/app"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, term.IsTerminal(int(os.Stdout.Fd()))))
}

// run is the testable entrypoint: parses flags from args and dispatches
// to the interactive TUI, the non-interactive --web path, or the logs
// subcommand.
//
// Subcommands:
//
//	logs [flags]  stream job logs to stdout (non-interactive)
func run(args []string, stdout, stderr io.Writer, isTTY bool) int {
	// Dispatch subcommands before top-level flag parsing so that
	// subcommand flags are not misinterpreted by the top-level FlagSet.
	if len(args) > 0 && args[0] == "logs" {
		return app.RunLogs(args[1:], stdout, stderr)
	}

	fs := flag.NewFlagSet("gh-peek", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, `gh-peek — browse GitHub Actions from your terminal

USAGE
  gh peek [--web]          open the interactive TUI (requires a TTY)
  gh peek logs [flags]     stream job logs to stdout (non-interactive)
  gh peek --help           show this help

TUI
  Starts in the most relevant view for the current git context:
    default branch  → all-runs list
    PR branch       → PR run list
    other branch    → branch run list

  Key bindings (selected):
    enter  open run detail     o  open in browser
    b      cycle branch/PR/all P  packages
    R      toggle auto-refresh ?  full help

FLAGS
`)
		fs.PrintDefaults()
		fmt.Fprintf(stderr, `
SUBCOMMANDS
  logs  Download and print job logs. Run 'gh peek logs --help' for details.
`)
	}
	web := fs.Bool("web", false, "open the repository's GitHub Actions page in a browser and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *web {
		return app.RunWeb(stdout, stderr)
	}
	return app.Run(stdout, stderr, isTTY)
}
