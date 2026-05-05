package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// runDeps bundles the swappable dependencies of Run for testing.
type runDeps struct {
	bootstrap  func(ctx context.Context) (domain.StartupContext, error)
	runProgram func(root tea.Model, stdout io.Writer) error
}

// Run is the testable entrypoint shared with cmd/gh-peek.
//
// If isTTY is false it prints the documented non-TTY message to stderr
// and returns exit code 2. Otherwise it resolves the startup context,
// constructs the routed root Model, and runs it with a Bubble Tea
// program. Bootstrap and program errors are reported on stderr and
// yield exit code 1.
func Run(stdout, stderr io.Writer, isTTY bool) int {
	return runWithDeps(stdout, stderr, isTTY, defaultRunDeps())
}

func defaultRunDeps() runDeps {
	return runDeps{
		bootstrap: func(ctx context.Context) (domain.StartupContext, error) {
			boot := bootstrap.New(gitctx.New(""), ghctx.New())
			return boot.Resolve(ctx)
		},
		runProgram: func(root tea.Model, stdout io.Writer) error {
			p := tea.NewProgram(root, tea.WithOutput(stdout))
			_, err := p.Run()
			return err
		},
	}
}

func runWithDeps(stdout, stderr io.Writer, isTTY bool, deps runDeps) int {
	if !isTTY {
		fmt.Fprintln(stderr, "gh-peek is interactive; run it from a terminal") //nolint:errcheck // stderr write; no recovery
		return 2
	}

	ctx := context.Background()
	startup, err := deps.bootstrap(ctx)
	if err != nil {
		switch {
		case errors.Is(err, bootstrap.ErrNotInGitRepo):
			fmt.Fprintln(stderr, "gh-peek: not inside a git repository") //nolint:errcheck
		case errors.Is(err, bootstrap.ErrDetachedHEAD), errors.Is(err, gitctx.ErrDetachedHEAD):
			fmt.Fprintln(stderr, "gh-peek: detached HEAD; check out a branch first") //nolint:errcheck
		case errors.Is(err, ghctx.ErrNoGitHubRepo):
			fmt.Fprintln(stderr, "gh-peek: no GitHub remote detected for this repository") //nolint:errcheck
		case errors.Is(err, ghctx.ErrGHUnavailable):
			fmt.Fprintln(stderr, "gh-peek: gh CLI not found on PATH; install GitHub CLI to continue") //nolint:errcheck
		default:
			fmt.Fprintln(stderr, "gh-peek:", err) //nolint:errcheck
		}
		return 1
	}

	client := githubapi.New()
	root := NewRouter(RootParams{
		Startup:     startup,
		Client:      client,
		Now:         time.Now,
		Width:       100,
		Height:      30,
		AutoRefresh: true,
	})

	if err := deps.runProgram(root, stdout); err != nil {
		fmt.Fprintln(stderr, "gh-peek:", err) //nolint:errcheck // stderr write; no recovery
		return 1
	}
	return 0
}
