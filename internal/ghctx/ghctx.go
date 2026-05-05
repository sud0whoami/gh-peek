// Package ghctx integrates with the GitHub CLI / go-gh to resolve the
// current repository, the repo's default branch, and the PR (if any)
// for the current branch.
//
// All external calls are routed through injectable function-typed
// fields so tests can fake gh / go-gh entirely.
package ghctx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	gh "github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// Sentinel errors for friendly bootstrap behavior.
var (
	// ErrNoGitHubRepo signals that the current directory is not a
	// recognizable GitHub repository (no remote, no GH_REPO override).
	ErrNoGitHubRepo = errors.New("no GitHub repository detected")
	// ErrGHUnavailable signals that the gh CLI is not installed or
	// not on PATH.
	ErrGHUnavailable = errors.New("gh CLI not available")
)

// RepoResolver is the interface used by callers (e.g. bootstrap).
type RepoResolver interface {
	CurrentRepo(ctx context.Context) (domain.RepoRef, error)
	DefaultBranch(ctx context.Context, repo domain.RepoRef) (string, error)
	CurrentBranchPR(ctx context.Context, repo domain.RepoRef, branch string) (*domain.PullRequestContext, error)
}

// RepoLike is the minimal shape required from a go-gh
// repository.Repository value. It is satisfied by repoAdapter below.
type RepoLike interface {
	Host() string
	Owner() string
	Name() string
}

// GHExecFunc runs the gh CLI and returns its stdout, stderr, and
// process error.
type GHExecFunc func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error)

// GH is the default RepoResolver. Both fields can be replaced in tests.
type GH struct {
	// Exec runs `gh ...`. If nil, gh.ExecContext from go-gh is used.
	Exec GHExecFunc
	// Repo resolves the current repository. If nil,
	// repository.Current() from go-gh is used.
	Repo func() (RepoLike, error)
}

// New constructs a GH wired up to the real go-gh defaults.
func New() *GH {
	return &GH{
		Exec: defaultExec,
		Repo: defaultRepo,
	}
}

func defaultExec(ctx context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	return gh.ExecContext(ctx, args...)
}

func defaultRepo() (RepoLike, error) {
	r, err := repository.Current()
	if err != nil {
		return nil, err
	}
	return repoAdapter{r: r}, nil
}

// repoAdapter wraps go-gh's struct-of-fields shape into the
// method-based RepoLike interface.
type repoAdapter struct{ r repository.Repository }

func (a repoAdapter) Host() string  { return a.r.Host }
func (a repoAdapter) Owner() string { return a.r.Owner }
func (a repoAdapter) Name() string  { return a.r.Name }

// CurrentRepo resolves the GitHub repository for the working directory.
func (g *GH) CurrentRepo(_ context.Context) (domain.RepoRef, error) {
	if g.Repo == nil {
		return domain.RepoRef{}, fmt.Errorf("ghctx: Repo function not configured")
	}
	r, err := g.Repo()
	if err != nil {
		return domain.RepoRef{}, fmt.Errorf("%w: %v", ErrNoGitHubRepo, err)
	}
	return domain.RepoRef{Host: r.Host(), Owner: r.Owner(), Name: r.Name()}, nil
}

// DefaultBranch returns the repository's default branch name via
// `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`.
func (g *GH) DefaultBranch(ctx context.Context, repo domain.RepoRef) (string, error) {
	target := repo.Owner + "/" + repo.Name
	stdout, stderr, err := g.exec(ctx,
		"repo", "view", target,
		"--json", "defaultBranchRef",
		"-q", ".defaultBranchRef.name",
	)
	if err != nil {
		if isGHNotFound(err) {
			return "", fmt.Errorf("%w: %v", ErrGHUnavailable, err)
		}
		return "", fmt.Errorf("gh repo view: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// prJSON mirrors the JSON shape returned by `gh pr view --json ...`.
type prJSON struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	BaseRefName string `json:"baseRefName"`
}

// CurrentBranchPR returns the PR for the given branch, or (nil, nil)
// if no PR exists.
func (g *GH) CurrentBranchPR(ctx context.Context, repo domain.RepoRef, branch string) (*domain.PullRequestContext, error) {
	stdout, stderr, err := g.exec(ctx,
		"pr", "view", branch,
		"--repo", repo.Owner+"/"+repo.Name,
		"--json", "number,title,url,headRefName,headRefOid,baseRefName",
	)
	if err != nil {
		if isGHNotFound(err) {
			return nil, fmt.Errorf("%w: %v", ErrGHUnavailable, err)
		}
		combined := stdout.String() + "\n" + stderr.String()
		if strings.Contains(strings.ToLower(combined), "no pull requests found") {
			return nil, nil
		}
		return nil, fmt.Errorf("gh pr view: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	var raw prJSON
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("gh pr view: parse json: %w", err)
	}
	return &domain.PullRequestContext{
		Number:      raw.Number,
		Title:       raw.Title,
		URL:         raw.URL,
		HeadRefName: raw.HeadRefName,
		HeadRefOID:  raw.HeadRefOid,
		BaseRefName: raw.BaseRefName,
	}, nil
}

func (g *GH) exec(ctx context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	if g.Exec == nil {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("ghctx: Exec function not configured")
	}
	return g.Exec(ctx, args...)
}

// isGHNotFound reports whether err indicates that the gh binary is
// missing from PATH.
func isGHNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var e *exec.Error
	if errors.As(err, &e) && errors.Is(e.Err, exec.ErrNotFound) {
		return true
	}
	return false
}

// Compile-time interface check.
var _ RepoResolver = (*GH)(nil)
