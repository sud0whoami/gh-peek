package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
)

// fakeOpener records URLs passed to Open and optionally returns an
// error. Safe for concurrent use.
type fakeOpener struct {
	mu     sync.Mutex
	urls   []string
	err    error
	called int
}

func (f *fakeOpener) Open(_ context.Context, url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	f.urls = append(f.urls, url)
	return f.err
}

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

func startupContextFor(host, owner, name string) domain.StartupContext {
	return domain.StartupContext{
		Kind: domain.StartContextAll,
		Repo: domain.RepoContext{
			Repo:          domain.RepoRef{Host: host, Owner: owner, Name: name},
			CurrentBranch: "main",
			DefaultBranch: "main",
			IsDefault:     true,
		},
	}
}

func TestRunWebWithDeps_HappyPath(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		owner   string
		repo    string
		wantURL string
	}{
		{"dotcom", "github.com", "foo", "bar", "https://github.com/foo/bar/actions"},
		{"ghes", "github.enterprise.example", "foo", "bar", "https://github.enterprise.example/foo/bar/actions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			opener := &fakeOpener{}
			deps := runDeps{
				bootstrap: func(context.Context) (domain.StartupContext, error) {
					return startupContextFor(tc.host, tc.owner, tc.repo), nil
				},
				runProgram: func(tea.Model, io.Writer) error {
					t.Fatal("runProgram must not be called for --web")
					return nil
				},
				browserOpener: opener,
			}
			code := runWebWithDeps(&stdout, &stderr, deps)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if opener.called != 1 {
				t.Fatalf("opener.called = %d, want 1", opener.called)
			}
			if got := opener.urls[0]; got != tc.wantURL {
				t.Errorf("url = %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestRunWebWithDeps_FriendlyBootstrapErrors(t *testing.T) {
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
			opener := &fakeOpener{}
			deps := runDeps{
				bootstrap: func(context.Context) (domain.StartupContext, error) {
					return domain.StartupContext{}, tc.err
				},
				runProgram: func(tea.Model, io.Writer) error {
					t.Fatal("runProgram must not be called for --web")
					return nil
				},
				browserOpener: opener,
			}
			code := runWebWithDeps(&stdout, &stderr, deps)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.wantLine) {
				t.Errorf("stderr = %q, want contains %q", stderr.String(), tc.wantLine)
			}
			if opener.called != 0 {
				t.Errorf("opener.called = %d, want 0 on bootstrap failure", opener.called)
			}
		})
	}
}

func TestRunWebWithDeps_OpenerFailureExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opener := &fakeOpener{err: errors.New("xdg-open not found")}
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return startupContextFor("github.com", "foo", "bar"), nil
		},
		runProgram: func(tea.Model, io.Writer) error {
			t.Fatal("runProgram must not be called for --web")
			return nil
		},
		browserOpener: opener,
	}
	code := runWebWithDeps(&stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to open browser") {
		t.Errorf("stderr = %q, want contains %q", stderr.String(), "failed to open browser")
	}
	if !strings.Contains(stderr.String(), "xdg-open not found") {
		t.Errorf("stderr = %q, want underlying error included", stderr.String())
	}
}

func TestRunWebWithDeps_DoesNotRequireTTY(t *testing.T) {
	// The --web path must work when stdout is not a TTY. We exercise it
	// here implicitly: runWebWithDeps never accepts an isTTY arg, so the
	// non-TTY guard cannot fire. This test documents the contract by
	// ensuring a successful exit even though no TTY is involved.
	var stdout, stderr bytes.Buffer
	opener := &fakeOpener{}
	deps := runDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return startupContextFor("github.com", "octo", "demo"), nil
		},
		runProgram: func(tea.Model, io.Writer) error {
			t.Fatal("runProgram must not be called for --web")
			return nil
		},
		browserOpener: opener,
	}
	if code := runWebWithDeps(&stdout, &stderr, deps); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if opener.called != 1 {
		t.Fatalf("opener.called = %d, want 1", opener.called)
	}
}
