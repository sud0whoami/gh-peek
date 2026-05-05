package gitctx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records the last call and returns the configured stdout/err.
type fakeRunner struct {
	stdout []byte
	stderr string
	err    error

	lastName string
	lastArgs []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.lastName = name
	f.lastArgs = args
	if f.err != nil {
		return nil, &exec.ExitError{Stderr: []byte(f.stderr)} // simulate exit error w/ stderr
	}
	return f.stdout, nil
}

// errRunner returns a custom error (with stderr) for negative paths.
type errRunner struct {
	stderr string
	err    error
}

func (r *errRunner) run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if r.err != nil {
		return nil, &runnerError{stderr: r.stderr, wrapped: r.err}
	}
	return nil, nil
}

// runnerError is a minimal error type that satisfies the stderr-extraction
// path used by the implementation. The implementation reads stderr out of
// *exec.ExitError; here we provide an alternative that has a Stderr method
// to ensure the implementation also handles that shape gracefully.
type runnerError struct {
	stderr  string
	wrapped error
}

func (e *runnerError) Error() string  { return e.wrapped.Error() }
func (e *runnerError) Unwrap() error  { return e.wrapped }
func (e *runnerError) Stderr() []byte { return []byte(e.stderr) }

func TestIsInsideWorkTree_True(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{stdout: []byte("true\n")}
	g := &Git{Run: r.run}
	got, err := g.IsInsideWorkTree(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got {
		t.Fatalf("got false, want true")
	}
	if r.lastName != "git" || len(r.lastArgs) == 0 || r.lastArgs[0] != "rev-parse" {
		t.Fatalf("unexpected call: %s %v", r.lastName, r.lastArgs)
	}
}

func TestIsInsideWorkTree_FalseWhenNotARepo(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{
		err:    errors.New("exit"),
		stderr: "fatal: not a git repository (or any of the parent directories): .git",
	}
	g := &Git{Run: r.run}
	got, err := g.IsInsideWorkTree(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got {
		t.Fatalf("got true, want false")
	}
}

func TestRepoRoot_StripsTrailingNewline(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{stdout: []byte("/tmp/repo\n")}
	g := &Git{Run: r.run}
	got, err := g.RepoRoot(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/tmp/repo" {
		t.Fatalf("got %q, want %q", got, "/tmp/repo")
	}
}

func TestCurrentBranch_StripsTrailingNewline(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{stdout: []byte("feature/foo\n")}
	g := &Git{Run: r.run}
	got, err := g.CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "feature/foo" {
		t.Fatalf("got %q, want %q", got, "feature/foo")
	}
}

func TestCurrentBranch_DetachedHEAD(t *testing.T) {
	t.Parallel()
	r := &errRunner{
		err:    errors.New("exit status 128"),
		stderr: "fatal: ref HEAD is not a symbolic ref",
	}
	g := &Git{Run: r.run}
	got, err := g.CurrentBranch(context.Background())
	if !errors.Is(err, ErrDetachedHEAD) {
		t.Fatalf("err = %v, want ErrDetachedHEAD", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestHeadSHA_StripsTrailingNewline(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{stdout: []byte("abc123\n")}
	g := &Git{Run: r.run}
	got, err := g.HeadSHA(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("got %q, want %q", got, "abc123")
	}
}

func TestUnexpectedErrorIncludesStderr(t *testing.T) {
	t.Parallel()
	r := &errRunner{
		err:    errors.New("exit status 1"),
		stderr: "some catastrophic failure",
	}
	g := &Git{Run: r.run}
	_, err := g.RepoRoot(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "some catastrophic failure") {
		t.Fatalf("error %q does not contain stderr", err.Error())
	}
}

func TestUnexpectedErrorOmitsEmptyStderr(t *testing.T) {
	t.Parallel()
	r := &errRunner{
		err:    errors.New("exit status 1"),
		stderr: "",
	}
	g := &Git{Run: r.run}
	_, err := g.HeadSHA(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if strings.Contains(err.Error(), "stderr:") {
		t.Fatalf("error %q should not mention stderr when empty", err.Error())
	}
}

// Integration: exercises the real git binary in a tempdir.
func TestRealGitBinary_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	// macOS resolves /var/folders/... to /private/var/folders/...; resolve up front.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	cmd := exec.Command("git", "init", "-q", "-b", "main", dir)
	cmd.Env = append(os.Environ(), "GIT_TEMPLATE_DIR=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	g := New(dir)
	ctx := context.Background()
	inside, err := g.IsInsideWorkTree(ctx)
	if err != nil {
		t.Fatalf("IsInsideWorkTree: %v", err)
	}
	if !inside {
		t.Fatalf("IsInsideWorkTree = false, want true")
	}
	root, err := g.RepoRoot(ctx)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root): %v", err)
	}
	if rootResolved != resolved {
		t.Fatalf("RepoRoot = %q, want %q", rootResolved, resolved)
	}
}
