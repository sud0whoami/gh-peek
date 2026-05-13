package ghctx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

type fakeRepo struct{ host, owner, name string }

func (f fakeRepo) Host() string  { return f.host }
func (f fakeRepo) Owner() string { return f.owner }
func (f fakeRepo) Name() string  { return f.name }

func TestCurrentRepo_Happy(t *testing.T) {
	t.Parallel()
	g := &GH{
		Repo: func() (RepoLike, error) {
			return fakeRepo{host: "github.com", owner: "octocat", name: "hello"}, nil
		},
		Exec: func(_ context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
			// Return org owner type for the owner-type call.
			var stdout bytes.Buffer
			stdout.WriteString(`{"owner":{"type":"Organization"}}`)
			return stdout, bytes.Buffer{}, nil
		},
	}
	got, err := g.CurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := domain.RepoRef{Host: "github.com", Owner: "octocat", Name: "hello", OwnerType: "Organization"}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCurrentRepo_NoRepo(t *testing.T) {
	t.Parallel()
	g := &GH{
		Repo: func() (RepoLike, error) {
			return nil, errors.New("no remotes configured")
		},
	}
	_, err := g.CurrentRepo(context.Background())
	if !errors.Is(err, ErrNoGitHubRepo) {
		t.Fatalf("err = %v, want wrapped ErrNoGitHubRepo", err)
	}
}

func TestCurrentRepo_OwnerTypeFallback(t *testing.T) {
	t.Parallel()
	// When the owner-type fetch fails, OwnerType is empty but no error is returned.
	g := &GH{
		Repo: func() (RepoLike, error) {
			return fakeRepo{host: "github.com", owner: "octocat", name: "hello"}, nil
		},
		Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
			return bytes.Buffer{}, bytes.Buffer{}, errors.New("fetch failed")
		},
	}
	got, err := g.CurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.OwnerType != "" {
		t.Errorf("OwnerType = %q, want empty on failure", got.OwnerType)
	}
}

func TestCurrentRepo_UserOwnerType(t *testing.T) {
	t.Parallel()
	g := &GH{
		Repo: func() (RepoLike, error) {
			return fakeRepo{host: "github.com", owner: "user1", name: "repo"}, nil
		},
		Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
			var stdout bytes.Buffer
			stdout.WriteString(`{"owner":{"type":"User"}}`)
			return stdout, bytes.Buffer{}, nil
		},
	}
	got, err := g.CurrentRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.OwnerType != "User" {
		t.Errorf("OwnerType = %q, want User", got.OwnerType)
	}
}

func TestDefaultBranch_Happy(t *testing.T) {
	t.Parallel()
	g := &GH{
		Exec: func(_ context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
			var stdout bytes.Buffer
			stdout.WriteString("main\n")
			return stdout, bytes.Buffer{}, nil
		},
	}
	got, err := g.DefaultBranch(context.Background(), domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "main" {
		t.Fatalf("got %q, want main", got)
	}
}

func TestDefaultBranch_PropagatesError(t *testing.T) {
	t.Parallel()
	g := &GH{
		Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
			var stderr bytes.Buffer
			stderr.WriteString("boom")
			return bytes.Buffer{}, stderr, errors.New("exit 1")
		},
	}
	_, err := g.DefaultBranch(context.Background(), domain.RepoRef{Owner: "o", Name: "r"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCurrentBranchPR_Happy(t *testing.T) {
	t.Parallel()
	const body = `{"number":42,"title":"Fix flaky test","url":"https://github.com/o/r/pull/42","headRefName":"feature/foo","headRefOid":"deadbeef","baseRefName":"main"}`
	g := &GH{
		Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
			var stdout bytes.Buffer
			stdout.WriteString(body)
			return stdout, bytes.Buffer{}, nil
		},
	}
	pr, err := g.CurrentBranchPR(context.Background(), domain.RepoRef{Owner: "o", Name: "r"}, "feature/foo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if pr == nil {
		t.Fatalf("pr is nil")
	}
	want := domain.PullRequestContext{
		Number: 42, Title: "Fix flaky test",
		URL:         "https://github.com/o/r/pull/42",
		HeadRefName: "feature/foo", HeadRefOID: "deadbeef", BaseRefName: "main",
	}
	if *pr != want {
		t.Fatalf("got %#v, want %#v", *pr, want)
	}
}

func TestCurrentBranchPR_NoPR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		stdout, stderr string
	}{
		{"on stderr", "", "no pull requests found for branch \"feature/foo\""},
		{"on stdout", "no pull requests found", ""},
		{"mixed case", "", "No Pull Requests Found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &GH{
				Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
					var so, se bytes.Buffer
					so.WriteString(tc.stdout)
					se.WriteString(tc.stderr)
					return so, se, errors.New("exit 1")
				},
			}
			pr, err := g.CurrentBranchPR(context.Background(), domain.RepoRef{Owner: "o", Name: "r"}, "feature/foo")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if pr != nil {
				t.Fatalf("pr = %#v, want nil", pr)
			}
		})
	}
}

func TestCurrentBranchPR_GHNotInstalled(t *testing.T) {
	t.Parallel()
	g := &GH{
		Exec: func(_ context.Context, _ ...string) (bytes.Buffer, bytes.Buffer, error) {
			return bytes.Buffer{}, bytes.Buffer{}, &exec.Error{Name: "gh", Err: exec.ErrNotFound}
		},
	}
	_, err := g.CurrentBranchPR(context.Background(), domain.RepoRef{Owner: "o", Name: "r"}, "feature/foo")
	if !errors.Is(err, ErrGHUnavailable) {
		t.Fatalf("err = %v, want wrapped ErrGHUnavailable", err)
	}
}
