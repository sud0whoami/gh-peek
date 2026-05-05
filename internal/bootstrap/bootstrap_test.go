package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
)

type fakeGit struct {
	inside     bool
	insideErr  error
	root       string
	rootErr    error
	branch     string
	branchErr  error
	headSHA    string
	headSHAErr error
}

func (f *fakeGit) IsInsideWorkTree(context.Context) (bool, error) { return f.inside, f.insideErr }
func (f *fakeGit) RepoRoot(context.Context) (string, error)       { return f.root, f.rootErr }
func (f *fakeGit) CurrentBranch(context.Context) (string, error)  { return f.branch, f.branchErr }
func (f *fakeGit) HeadSHA(context.Context) (string, error)        { return f.headSHA, f.headSHAErr }

type fakeGH struct {
	repo    domain.RepoRef
	repoErr error
	def     string
	defErr  error
	pr      *domain.PullRequestContext
	prErr   error
}

func (f *fakeGH) CurrentRepo(context.Context) (domain.RepoRef, error) { return f.repo, f.repoErr }
func (f *fakeGH) DefaultBranch(context.Context, domain.RepoRef) (string, error) {
	return f.def, f.defErr
}
func (f *fakeGH) CurrentBranchPR(_ context.Context, _ domain.RepoRef, _ string) (*domain.PullRequestContext, error) {
	return f.pr, f.prErr
}

func okGit() *fakeGit {
	return &fakeGit{inside: true, root: "/tmp/r", branch: "main", headSHA: "sha1"}
}
func okGH() *fakeGH {
	return &fakeGH{repo: domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, def: "main"}
}

func TestResolve_NotInGitRepo(t *testing.T) {
	t.Parallel()
	b := New(&fakeGit{inside: false}, okGH())
	_, err := b.Resolve(context.Background())
	if !errors.Is(err, ErrNotInGitRepo) {
		t.Fatalf("err = %v, want ErrNotInGitRepo", err)
	}
}

func TestResolve_DetachedHEAD(t *testing.T) {
	t.Parallel()
	g := okGit()
	g.branch = ""
	g.branchErr = gitctx.ErrDetachedHEAD
	b := New(g, okGH())
	_, err := b.Resolve(context.Background())
	if !errors.Is(err, ErrDetachedHEAD) {
		t.Fatalf("err = %v, want ErrDetachedHEAD", err)
	}
}

func TestResolve_NoGitHubRepo(t *testing.T) {
	t.Parallel()
	gh := okGH()
	gh.repoErr = ghctx.ErrNoGitHubRepo
	b := New(okGit(), gh)
	_, err := b.Resolve(context.Background())
	if !errors.Is(err, ghctx.ErrNoGitHubRepo) {
		t.Fatalf("err = %v, want ErrNoGitHubRepo", err)
	}
}

func TestResolve_OnDefaultBranch_AllRuns(t *testing.T) {
	t.Parallel()
	b := New(okGit(), okGH())
	sc, err := b.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sc.Kind != domain.StartContextAll {
		t.Fatalf("Kind = %q, want %q", sc.Kind, domain.StartContextAll)
	}
	if !sc.Repo.IsDefault {
		t.Fatalf("Repo.IsDefault = false, want true")
	}
	if sc.PR != nil {
		t.Fatalf("PR = %#v, want nil", sc.PR)
	}
	if sc.Repo.Repo.Owner != "o" || sc.Repo.RootDir != "/tmp/r" || sc.Repo.HeadSHA != "sha1" {
		t.Fatalf("unexpected RepoContext: %#v", sc.Repo)
	}
}

func TestResolve_FeatureBranchWithPR(t *testing.T) {
	t.Parallel()
	g := okGit()
	g.branch = "feature/foo"
	gh := okGH()
	gh.pr = &domain.PullRequestContext{Number: 42, HeadRefName: "feature/foo"}
	b := New(g, gh)
	sc, err := b.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sc.Kind != domain.StartContextPR {
		t.Fatalf("Kind = %q, want %q", sc.Kind, domain.StartContextPR)
	}
	if sc.Repo.IsDefault {
		t.Fatalf("Repo.IsDefault = true, want false")
	}
	if sc.PR == nil || sc.PR.Number != 42 {
		t.Fatalf("PR = %#v, want number 42", sc.PR)
	}
}

func TestResolve_FeatureBranchNoPR(t *testing.T) {
	t.Parallel()
	g := okGit()
	g.branch = "feature/foo"
	b := New(g, okGH()) // pr nil, prErr nil
	sc, err := b.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sc.Kind != domain.StartContextBranch {
		t.Fatalf("Kind = %q, want %q", sc.Kind, domain.StartContextBranch)
	}
	if sc.PR != nil {
		t.Fatalf("PR = %#v, want nil", sc.PR)
	}
}

func TestResolve_PRLookup_GHUnavailable(t *testing.T) {
	t.Parallel()
	g := okGit()
	g.branch = "feature/foo"
	gh := okGH()
	gh.prErr = ghctx.ErrGHUnavailable
	b := New(g, gh)
	_, err := b.Resolve(context.Background())
	if !errors.Is(err, ghctx.ErrGHUnavailable) {
		t.Fatalf("err = %v, want ErrGHUnavailable", err)
	}
}
