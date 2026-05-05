package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
)

func TestRunWithDeps_FriendlyBootstrapErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantLine string
	}{
		{"not-in-git-repo", bootstrap.ErrNotInGitRepo, "not inside a git repository"},
		{"detached-bootstrap", bootstrap.ErrDetachedHEAD, "detached HEAD"},
		{"detached-gitctx", gitctx.ErrDetachedHEAD, "detached HEAD"},
		{"no-github-repo", ghctx.ErrNoGitHubRepo, "no GitHub remote"},
		{"gh-unavailable", ghctx.ErrGHUnavailable, "gh CLI not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			deps := runDeps{
				bootstrap: func(context.Context) (domain.StartupContext, error) {
					return domain.StartupContext{}, tc.err
				},
				runProgram: func(tea.Model, io.Writer) error {
					t.Fatal("runProgram should not be called when bootstrap fails")
					return nil
				},
			}
			code := runWithDeps(&stdout, &stderr, true, deps)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.wantLine) {
				t.Errorf("stderr = %q, want contains %q", stderr.String(), tc.wantLine)
			}
		})
	}
}

func TestRunWithDeps_GenericBootstrapError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return domain.StartupContext{}, errors.New("boom")
		},
		runProgram: func(tea.Model, io.Writer) error { return nil },
	}
	code := runWithDeps(&stdout, &stderr, true, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want contains %q", stderr.String(), "boom")
	}
}

func TestRunWithDeps_HappyPathReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := 0
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return domain.StartupContext{
				Kind: domain.StartContextAll,
				Repo: domain.RepoContext{
					Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
					CurrentBranch: "main",
					DefaultBranch: "main",
					IsDefault:     true,
				},
			}, nil
		},
		runProgram: func(tea.Model, io.Writer) error {
			called++
			return nil
		},
	}
	code := runWithDeps(&stdout, &stderr, true, deps)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if called != 1 {
		t.Errorf("runProgram call count = %d, want 1", called)
	}
}

func TestRunWithDeps_ProgramErrorReturnsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return domain.StartupContext{
				Kind: domain.StartContextAll,
				Repo: domain.RepoContext{
					Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
					CurrentBranch: "main",
					DefaultBranch: "main",
					IsDefault:     true,
				},
			}, nil
		},
		runProgram: func(tea.Model, io.Writer) error {
			return errors.New("program crashed")
		},
	}
	code := runWithDeps(&stdout, &stderr, true, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "program crashed") {
		t.Errorf("stderr = %q, want contains %q", stderr.String(), "program crashed")
	}
}

func TestRunWithDeps_NonTTYExitsWithCode2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			t.Fatal("bootstrap should not be called when stdout is not a TTY")
			return domain.StartupContext{}, nil
		},
		runProgram: func(tea.Model, io.Writer) error { return nil },
	}
	code := runWithDeps(&stdout, &stderr, false, deps)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "interactive") {
		t.Errorf("stderr = %q, want contains %q", stderr.String(), "interactive")
	}
}
