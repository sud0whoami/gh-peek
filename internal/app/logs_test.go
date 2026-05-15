package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/bootstrap"
	"github.com/sud0whoami/gh-peek/internal/clipboard"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/ghctx"
	"github.com/sud0whoami/gh-peek/internal/gitctx"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// logsTestClient is a configurable fake implementing githubapi.ActionsClient
// for logs tests.
type logsTestClient struct {
	listRunsResult githubapi.ListRunsResult
	listRunsErr    error
	getRun         domain.WorkflowRun
	getRunErr      error
	listJobs       []domain.WorkflowJob
	listJobsErr    error
	downloadLog    []byte
	downloadLogErr error
}

func (f *logsTestClient) ListRuns(_ context.Context, _ domain.RepoRef, _ githubapi.ListRunsFilter) (githubapi.ListRunsResult, error) {
	return f.listRunsResult, f.listRunsErr
}

func (f *logsTestClient) GetRun(_ context.Context, _ domain.RepoRef, _ int64) (domain.WorkflowRun, error) {
	return f.getRun, f.getRunErr
}

func (f *logsTestClient) ListJobs(_ context.Context, _ domain.RepoRef, _ int64) ([]domain.WorkflowJob, error) {
	return f.listJobs, f.listJobsErr
}

func (f *logsTestClient) DownloadJobLog(_ context.Context, _ domain.RepoRef, _ int64) ([]byte, error) {
	return f.downloadLog, f.downloadLogErr
}

func defaultLogsStartup() domain.StartupContext {
	return domain.StartupContext{
		Kind: domain.StartContextAll,
		Repo: domain.RepoContext{
			Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
			CurrentBranch: "main",
			DefaultBranch: "main",
			IsDefault:     true,
		},
	}
}

func TestRunLogsWithDeps_BootstrapErrors(t *testing.T) {
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
			deps := logsDeps{
				bootstrap: func(context.Context) (domain.StartupContext, error) {
					return domain.StartupContext{}, tc.err
				},
				client: &logsTestClient{},
			}
			code := runLogsWithDeps(nil, &stdout, &stderr, deps)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.wantLine) {
				t.Errorf("stderr = %q, want contains %q", stderr.String(), tc.wantLine)
			}
		})
	}
}

func TestRunLogsWithDeps_ErrorsAndJSONMutuallyExclusive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{},
	}
	code := runLogsWithDeps([]string{"--errors", "--json"}, &stdout, &stderr, deps)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--errors") || !strings.Contains(stderr.String(), "--json") {
		t.Errorf("stderr = %q, want contains both --errors and --json", stderr.String())
	}
}

func TestRunLogsWithDeps_NoRunsFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{Runs: nil},
		},
	}
	code := runLogsWithDeps([]string{"--run", "0"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no runs found") {
		t.Errorf("stderr = %q, want contains %q", stderr.String(), "no runs found")
	}
}

func TestRunLogsWithDeps_SuccessfulTextOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1, Status: "completed", Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("line1\nline2\n"),
		},
	}
	code := runLogsWithDeps([]string{"--all"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "line1") {
		t.Errorf("stdout = %q, want contains %q", out, "line1")
	}
	if !strings.Contains(out, "line2") {
		t.Errorf("stdout = %q, want contains %q", out, "line2")
	}
	if !strings.Contains(out, "build") {
		t.Errorf("stdout = %q, want contains job name %q", out, "build")
	}
}

func TestRunLogsWithDeps_FailedOnlyDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "passing-job", Conclusion: "success"},
			},
			downloadLog: []byte("some log output\n"),
		},
	}
	// --failed-only is true by default; no failed jobs → exit 1 with a hint.
	code := runLogsWithDeps(nil, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (no failed jobs); stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--all") {
		t.Errorf("stderr = %q, want hint about --all", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty when no jobs match; got %q", stdout.String())
	}
}

func TestRunLogsWithDeps_AllFlagIncludesNonFailed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "success-job", Conclusion: "success"},
			},
			downloadLog: []byte("output line\n"),
		},
	}
	code := runLogsWithDeps([]string{"--all"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "success-job") {
		t.Errorf("stdout = %q, want contains %q", stdout.String(), "success-job")
	}
}

func TestRunLogsWithDeps_ErrorsOutputsSnippets(t *testing.T) {
	// --errors should call formatErrors and print error snippets.
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			// Log containing an ##[error] line.
			downloadLog: []byte("2024-01-01T00:00:00Z ##[group]step\n2024-01-01T00:00:00Z ##[error]something broke\n2024-01-01T00:00:00Z ##[endgroup]\n"),
		},
	}
	code := runLogsWithDeps([]string{"--errors"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "something broke") {
		t.Errorf("stdout = %q, want contains %q", out, "something broke")
	}
	if !strings.Contains(out, "build") {
		t.Errorf("stdout = %q, want contains job name %q", out, "build")
	}
}

func TestRunLogsWithDeps_JSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 42, Name: "CI", Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("line1\nline2\n"),
		},
	}
	code := runLogsWithDeps([]string{"--json"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}

	var envelope logsEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, stdout.String())
	}
	if envelope.Run.ID != 42 {
		t.Errorf("run.id = %d, want 42", envelope.Run.ID)
	}
	if len(envelope.Jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(envelope.Jobs))
	}
	if envelope.Jobs[0].Name != "build" {
		t.Errorf("jobs[0].name = %q, want %q", envelope.Jobs[0].Name, "build")
	}
}

func TestRunLogsWithDeps_LogTruncatedBanner(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog:    []byte("partial log\n"),
			downloadLogErr: githubapi.ErrLogTooLarge,
		},
	}
	code := runLogsWithDeps([]string{"--all"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (partial success); stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "LOG TRUNCATED") {
		t.Errorf("stdout = %q, want contains %q", out, "LOG TRUNCATED")
	}
	if !strings.Contains(out, "partial log") {
		t.Errorf("stdout = %q, want contains %q", out, "partial log")
	}
}

func TestRunLogsWithDeps_APIError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsErr: errors.New("api down"),
		},
	}
	code := runLogsWithDeps(nil, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

// fakeCopier is a test double for clipboard.Copier.
type fakeCopier struct {
	got []byte
	err error
}

func (f *fakeCopier) Copy(_ context.Context, b []byte) error {
	f.got = append([]byte(nil), b...)
	return f.err
}

func TestRunLogsWithDeps_CopyTextMode(t *testing.T) {
	fc := &fakeCopier{}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 42, Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("line1\nline2\n"),
		},
		copier: fc,
	}

	code := runLogsWithDeps([]string{"--copy", "--all"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty when --copy is set; got %q", stdout.String())
	}
	if !strings.Contains(string(fc.got), "line1") {
		t.Errorf("clipboard content = %q, want contains %q", fc.got, "line1")
	}
	if !strings.Contains(stderr.String(), "copied") || !strings.Contains(stderr.String(), "bytes") {
		t.Errorf("stderr = %q, want contains 'copied' and 'bytes'", stderr.String())
	}
}

func TestRunLogsWithDeps_CopyErrorsMode(t *testing.T) {
	fc := &fakeCopier{}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 42, Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("2024-01-01T00:00:00Z ##[group]step\n2024-01-01T00:00:00Z ##[error]something broke\n2024-01-01T00:00:00Z ##[endgroup]\n"),
		},
		copier: fc,
	}

	code := runLogsWithDeps([]string{"--copy", "--errors"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty when --copy is set; got %q", stdout.String())
	}
	if len(fc.got) == 0 {
		t.Errorf("clipboard content should be non-empty for --errors mode")
	}
}

func TestRunLogsWithDeps_CopyJSONMode(t *testing.T) {
	fc := &fakeCopier{}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 42, Name: "CI", Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("line1\n"),
		},
		copier: fc,
	}

	code := runLogsWithDeps([]string{"--copy", "--json"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty when --copy is set; got %q", stdout.String())
	}

	var envelope logsEnvelope
	if err := json.Unmarshal(fc.got, &envelope); err != nil {
		t.Fatalf("clipboard content is not valid JSON: %v\ncontent: %s", err, fc.got)
	}
	if envelope.Run.ID != 42 {
		t.Errorf("envelope.run.id = %d, want 42", envelope.Run.ID)
	}
}

func TestRunLogsWithDeps_CopyNoTool(t *testing.T) {
	fc := &fakeCopier{err: clipboard.ErrNoClipboardTool}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1, Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("log line\n"),
		},
		copier: fc,
	}

	code := runLogsWithDeps([]string{"--copy", "--all"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "xclip") && !strings.Contains(errOut, "xsel") {
		t.Errorf("stderr = %q, want hint about xclip or xsel", errOut)
	}
}

func TestRunLogsWithDeps_CopyGenericError(t *testing.T) {
	fc := &fakeCopier{err: errors.New("pipe broke")}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1, Conclusion: "failure"}},
			},
			listJobs: []domain.WorkflowJob{
				{ID: 10, Name: "build", Conclusion: "failure"},
			},
			downloadLog: []byte("log line\n"),
		},
		copier: fc,
	}

	code := runLogsWithDeps([]string{"--copy", "--all"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to copy to clipboard") {
		t.Errorf("stderr = %q, want contains 'failed to copy to clipboard'", stderr.String())
	}
}

func TestRunLogsWithDeps_CopySkippedWhenNoJobs(t *testing.T) {
	fc := &fakeCopier{}
	var stdout, stderr bytes.Buffer
	deps := logsDeps{
		bootstrap: func(context.Context) (domain.StartupContext, error) {
			return defaultLogsStartup(), nil
		},
		client: &logsTestClient{
			listRunsResult: githubapi.ListRunsResult{
				Runs: []domain.WorkflowRun{{ID: 1, Conclusion: "success"}},
			},
			listJobs: []domain.WorkflowJob{
				// Only a success job; --failed-only (default) filters it out.
				{ID: 10, Name: "passing-job", Conclusion: "success"},
			},
			downloadLog: []byte("some log\n"),
		},
		copier: fc,
	}

	// --failed-only is true by default; no failed jobs → no-jobs-match guard fires before clipboard.
	code := runLogsWithDeps([]string{"--copy"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (no-jobs-match guard)", code)
	}
	if fc.got != nil {
		t.Errorf("Copy was called but should not have been; fc.got = %q", fc.got)
	}
}
