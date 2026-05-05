// Package bootstrap resolves the startup context for gh-peek:
// repo + PR + branch + default-branch facts, combined into a single
// domain.StartupContext that the UI layer consumes.
package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
)

// Sentinel errors. ghctx and gitctx sentinels are re-surfaced via
// errors.Is rather than redefined here.
var (
	// ErrNotInGitRepo is returned when the working directory is not
	// inside a git worktree.
	ErrNotInGitRepo = errors.New("not inside a git repository")
	// ErrDetachedHEAD is returned when the worktree is in a
	// detached-HEAD state (no branch checked out).
	ErrDetachedHEAD = errors.New("detached HEAD; gh-peek needs a checked-out branch")
)

// Bootstrapper orchestrates the startup-context resolution algorithm.
type Bootstrapper struct {
	Git gitctx.GitInspector
	GH  ghctx.RepoResolver
}

// New constructs a Bootstrapper from the given git + gh adapters.
func New(g gitctx.GitInspector, gh ghctx.RepoResolver) *Bootstrapper {
	return &Bootstrapper{Git: g, GH: gh}
}

// Resolve runs the startup algorithm and returns the resolved
// StartupContext. See package doc for the algorithm.
func (b *Bootstrapper) Resolve(ctx context.Context) (domain.StartupContext, error) {
	var sc domain.StartupContext

	inside, err := b.Git.IsInsideWorkTree(ctx)
	if err != nil {
		return sc, fmt.Errorf("bootstrap: check worktree: %w", err)
	}
	if !inside {
		return sc, ErrNotInGitRepo
	}

	root, err := b.Git.RepoRoot(ctx)
	if err != nil {
		return sc, fmt.Errorf("bootstrap: repo root: %w", err)
	}

	branch, err := b.Git.CurrentBranch(ctx)
	if err != nil {
		if errors.Is(err, gitctx.ErrDetachedHEAD) {
			return sc, ErrDetachedHEAD
		}
		return sc, fmt.Errorf("bootstrap: current branch: %w", err)
	}

	headSHA, err := b.Git.HeadSHA(ctx)
	if err != nil {
		return sc, fmt.Errorf("bootstrap: head sha: %w", err)
	}

	repo, err := b.GH.CurrentRepo(ctx)
	if err != nil {
		if errors.Is(err, ghctx.ErrNoGitHubRepo) {
			return sc, fmt.Errorf("current directory is not a GitHub repository: %w", err)
		}
		return sc, fmt.Errorf("bootstrap: current repo: %w", err)
	}

	defaultBranch, err := b.GH.DefaultBranch(ctx, repo)
	if err != nil {
		return sc, fmt.Errorf("bootstrap: default branch: %w", err)
	}

	rc := domain.RepoContext{
		Repo:          repo,
		RootDir:       root,
		CurrentBranch: branch,
		DefaultBranch: defaultBranch,
		HeadSHA:       headSHA,
		IsDefault:     branch == defaultBranch,
	}

	if rc.IsDefault {
		return domain.StartupContext{Kind: domain.StartContextAll, Repo: rc}, nil
	}

	pr, err := b.GH.CurrentBranchPR(ctx, repo, branch)
	if err != nil {
		return sc, fmt.Errorf("bootstrap: current-branch PR: %w", err)
	}
	if pr != nil {
		return domain.StartupContext{Kind: domain.StartContextPR, Repo: rc, PR: pr}, nil
	}
	return domain.StartupContext{Kind: domain.StartContextBranch, Repo: rc}, nil
}
