// Package gitctx inspects the local git worktree the user invoked
// gh-peek inside of: worktree check, repo root, current branch, and
// HEAD SHA.
//
// All commands run through an injectable CommandRunner so tests can
// fake git without depending on the real binary.
package gitctx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrDetachedHEAD is returned by CurrentBranch when the worktree is
// in a detached-HEAD state.
var ErrDetachedHEAD = errors.New("detached HEAD")

// GitInspector is the interface used by callers (e.g. bootstrap).
type GitInspector interface {
	IsInsideWorkTree(ctx context.Context) (bool, error)
	RepoRoot(ctx context.Context) (string, error)
	CurrentBranch(ctx context.Context) (string, error)
	HeadSHA(ctx context.Context) (string, error)
}

// CommandRunner runs an external command and returns its stdout.
// On non-zero exit, the returned error should expose stderr either
// via *exec.ExitError or via a Stderr() []byte method.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Git is the default GitInspector implementation backed by `git`.
type Git struct {
	// Run executes a command. If nil, a default runner using
	// exec.CommandContext is used.
	Run CommandRunner
	// Dir is the working directory for git invocations. Empty means
	// the current process directory.
	Dir string
}

// New constructs a Git that runs the real git binary in workingDir.
func New(workingDir string) *Git {
	g := &Git{Dir: workingDir}
	g.Run = g.defaultRunner
	return g
}

func (g *Git) defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if g.Dir != "" {
		cmd.Dir = g.Dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Attach stderr to *exec.ExitError so callers can recover it.
		if ee, ok := err.(*exec.ExitError); ok {
			ee.Stderr = stderr.Bytes()
		}
		return out, err
	}
	return out, nil
}

// stderrFrom extracts stderr bytes from an error produced by a
// CommandRunner. It supports *exec.ExitError and any error that
// exposes a Stderr() []byte method.
func stderrFrom(err error) string {
	if err == nil {
		return ""
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(ee.Stderr)
	}
	type stderrer interface{ Stderr() []byte }
	var se stderrer
	if errors.As(err, &se) {
		return string(se.Stderr())
	}
	return ""
}

// wrap wraps an exec failure with subcommand context plus stderr (when
// non-empty).
func wrap(subcmd string, err error) error {
	stderr := strings.TrimSpace(stderrFrom(err))
	if stderr == "" {
		return fmt.Errorf("git %s: %w", subcmd, err)
	}
	return fmt.Errorf("git %s: %w (stderr: %s)", subcmd, err, stderr)
}

func (g *Git) run(ctx context.Context, args ...string) ([]byte, string, error) {
	out, err := g.Run(ctx, "git", args...)
	if err != nil {
		return out, stderrFrom(err), err
	}
	return out, "", nil
}

// IsInsideWorkTree reports whether the working directory is inside a
// git worktree. A "not a git repository" failure is reported as
// (false, nil); other errors propagate.
func (g *Git) IsInsideWorkTree(ctx context.Context) (bool, error) {
	out, stderr, err := g.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if strings.Contains(stderr, "not a git repository") {
			return false, nil
		}
		return false, wrap("rev-parse --is-inside-work-tree", err)
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// RepoRoot returns the absolute path of the repository root.
func (g *Git) RepoRoot(ctx context.Context) (string, error) {
	out, _, err := g.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", wrap("rev-parse --show-toplevel", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// CurrentBranch returns the short name of the currently checked-out
// branch. Detached HEAD returns ErrDetachedHEAD.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	out, stderr, err := g.run(ctx, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		if strings.Contains(stderr, "not a symbolic ref") {
			return "", ErrDetachedHEAD
		}
		return "", wrap("symbolic-ref --short HEAD", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// HeadSHA returns the full SHA of HEAD.
func (g *Git) HeadSHA(ctx context.Context) (string, error) {
	out, _, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", wrap("rev-parse HEAD", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Compile-time interface check.
var _ GitInspector = (*Git)(nil)
