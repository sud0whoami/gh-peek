package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/browser"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// runDeps bundles the swappable dependencies of Run for testing.
type runDeps struct {
	bootstrap     func(ctx context.Context) (domain.StartupContext, error)
	runProgram    func(root tea.Model, stdout io.Writer) error
	browserOpener browser.Opener
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
		browserOpener: browser.OSOpener{},
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
		writeBootstrapError(stderr, err)
		return 1
	}

	client := githubapi.New()
	root := NewRouter(RootParams{
		Startup:        startup,
		Client:         client,
		ReleasesClient: client,
		Now:            time.Now,
		Width:          100,
		Height:         30,
		AutoRefresh:    true,
	})

	if err := deps.runProgram(root, stdout); err != nil {
		fmt.Fprintln(stderr, "gh-peek:", err) //nolint:errcheck // stderr write; no recovery
		return 1
	}
	return 0
}

// RunWeb resolves the current repo context and opens the repository's
// GitHub Actions page in the user's browser.
func RunWeb(stdout, stderr io.Writer) int {
	return runWebWithDeps(stdout, stderr, defaultRunDeps())
}

func runWebWithDeps(stdout, stderr io.Writer, deps runDeps) int {
	_ = stdout // success is silent
	ctx := context.Background()
	startup, err := deps.bootstrap(ctx)
	if err != nil {
		writeBootstrapError(stderr, err)
		return 1
	}

	repo := startup.Repo.Repo
	url := actionsWebURL(repo)

	opener := deps.browserOpener
	if opener == nil {
		opener = browser.OSOpener{}
	}
	if err := opener.Open(ctx, url); err != nil {
		fmt.Fprintln(stderr, "gh-peek: failed to open browser:", err) //nolint:errcheck // stderr write; no recovery
		return 1
	}
	return 0
}

// actionsWebURL builds the web URL for the repo's Actions tab.
func actionsWebURL(r domain.RepoRef) string {
	return fmt.Sprintf("https://%s/%s/%s/actions", r.Host, r.Owner, r.Name)
}

// writeBootstrapError prints the friendly user-facing message for a
// bootstrap failure.
func writeBootstrapError(stderr io.Writer, err error) {
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
}
