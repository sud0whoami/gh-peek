package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/browser"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	logscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/log"
	runscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/run"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/runs"
)

// recordingOpener captures the URLs passed to Open. It implements
// browser.Opener so the router tests don't actually launch a browser.
type recordingOpener struct {
	mu   sync.Mutex
	URLs []string
	err  error
}

func (r *recordingOpener) Open(_ context.Context, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.URLs = append(r.URLs, url)
	return r.err
}

func (r *recordingOpener) urls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.URLs))
	copy(out, r.URLs)
	return out
}

var _ browser.Opener = (*recordingOpener)(nil)

func newRouterWithOpener(o browser.Opener) *Model {
	return NewRouter(RootParams{
		Startup:       routerStartup(),
		Client:        routerFakeClient{},
		Now:           func() time.Time { return time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) },
		Width:         100,
		Height:        30,
		AutoRefresh:   true,
		TickInterval:  time.Millisecond,
		BrowserOpener: o,
	})
}

// routerFakeClient is a no-op ActionsClient for routing tests.
type routerFakeClient struct{}

func (routerFakeClient) ListRuns(context.Context, domain.RepoRef, githubapi.ListRunsFilter) (githubapi.ListRunsResult, error) {
	return githubapi.ListRunsResult{}, nil
}
func (routerFakeClient) GetRun(context.Context, domain.RepoRef, int64) (domain.WorkflowRun, error) {
	return domain.WorkflowRun{}, nil
}
func (routerFakeClient) ListJobs(context.Context, domain.RepoRef, int64) ([]domain.WorkflowJob, error) {
	return nil, nil
}
func (routerFakeClient) DownloadJobLog(context.Context, domain.RepoRef, int64) ([]byte, error) {
	return nil, nil
}

func routerStartup() domain.StartupContext {
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

func newRouterForTest() *Model {
	return NewRouter(RootParams{
		Startup:      routerStartup(),
		Client:       routerFakeClient{},
		Now:          func() time.Time { return time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) },
		Width:        100,
		Height:       30,
		AutoRefresh:  true,
		TickInterval: time.Millisecond,
	})
}

func TestRouter_StartsOnRunsScreen(t *testing.T) {
	m := newRouterForTest()
	if got := m.ActiveScreenName(); got != "runs" {
		t.Fatalf("active screen = %q, want %q", got, "runs")
	}
	if m.RunsScreen() == nil {
		t.Fatal("RunsScreen() = nil; want non-nil instance after NewRouter")
	}
}

func TestRouter_OpenRunMsgSwapsToDetail(t *testing.T) {
	m := newRouterForTest()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 42, Repo: repo})
	mm := updated.(*Model)
	if got := mm.ActiveScreenName(); got != "run-detail" {
		t.Fatalf("active screen = %q, want %q", got, "run-detail")
	}
	if mm.DetailScreen() == nil {
		t.Fatal("DetailScreen() = nil after OpenRunMsg")
	}
	if got := mm.DetailScreen().RunID(); got != 42 {
		t.Fatalf("detail RunID = %d, want 42", got)
	}
}

func TestRouter_BackMsgReturnsToRunsScreen(t *testing.T) {
	m := newRouterForTest()
	before := m.RunsScreen()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 7, Repo: repo})
	mm := updated.(*Model)
	if mm.ActiveScreenName() != "run-detail" {
		t.Fatalf("precondition: expected to be on run-detail, got %q", mm.ActiveScreenName())
	}
	updated, _ = mm.Update(runscreen.BackMsg{})
	mm = updated.(*Model)
	if got := mm.ActiveScreenName(); got != "runs" {
		t.Fatalf("active screen = %q, want %q", got, "runs")
	}
	if mm.RunsScreen() != before {
		t.Fatal("RunsScreen instance changed across BackMsg; expected the same pointer (state preserved)")
	}
	if mm.DetailScreen() != nil {
		t.Fatal("DetailScreen() should be nil after BackMsg")
	}
}

func TestRouter_WindowSizeForwardedAndRemembered(t *testing.T) {
	m := newRouterForTest()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mm := updated.(*Model)
	if mm.Width() != 80 {
		t.Fatalf("router width = %d, want 80", mm.Width())
	}
	repo := routerStartup().Repo.Repo
	updated, _ = mm.Update(runs.OpenRunMsg{RunID: 1, Repo: repo})
	mm = updated.(*Model)
	if got := mm.DetailScreen().Width(); got != 80 {
		t.Fatalf("detail screen width = %d, want 80 (inherited from router)", got)
	}
}

func TestRouter_QuitStillWorks(t *testing.T) {
	m := newRouterForTest()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected quit cmd, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestRouter_OpenInBrowser_FromRuns(t *testing.T) {
	op := &recordingOpener{}
	m := newRouterWithOpener(op)
	_, cmd := m.Update(runs.OpenInBrowserMsg{URL: "https://example.com/runs"})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd that opens the URL")
	}
	_ = cmd()
	got := op.urls()
	if len(got) != 1 || got[0] != "https://example.com/runs" {
		t.Fatalf("opener URLs = %v, want [https://example.com/runs]", got)
	}
}

func TestRouter_OpenInBrowser_FromRun(t *testing.T) {
	op := &recordingOpener{}
	m := newRouterWithOpener(op)
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 9, Repo: repo})
	mm := updated.(*Model)
	prevDetail := mm.DetailScreen()
	_, cmd := mm.Update(runscreen.OpenInBrowserMsg{URL: "https://example.com/run"})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd that opens the URL")
	}
	_ = cmd()
	got := op.urls()
	if len(got) != 1 || got[0] != "https://example.com/run" {
		t.Fatalf("opener URLs = %v, want [https://example.com/run]", got)
	}
	if mm.ActiveScreenName() != "run-detail" || mm.DetailScreen() != prevDetail {
		t.Fatal("run.OpenInBrowserMsg should not change the active screen")
	}
}

func TestRouter_OpenInBrowser_FromLog(t *testing.T) {
	op := &recordingOpener{}
	m := newRouterWithOpener(op)
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 9, Repo: repo})
	mm := updated.(*Model)
	updated, _ = mm.Update(runscreen.OpenJobLogMsg{Repo: repo, RunID: 9, JobID: 1})
	mm = updated.(*Model)
	prevLog := mm.LogScreen()
	_, cmd := mm.Update(logscreen.OpenInBrowserMsg{URL: "https://example.com/log"})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd that opens the URL")
	}
	_ = cmd()
	got := op.urls()
	if len(got) != 1 || got[0] != "https://example.com/log" {
		t.Fatalf("opener URLs = %v, want [https://example.com/log]", got)
	}
	if mm.ActiveScreenName() != "log-viewer" || mm.LogScreen() != prevLog {
		t.Fatal("log.OpenInBrowserMsg should not change the active screen")
	}
}

func TestRouter_LegacyNewStillBehavesLikeM0(t *testing.T) {
	m := New()
	if got := m.ActiveScreenName(); got != "none" {
		t.Fatalf("legacy active screen = %q, want %q", got, "none")
	}
	view := m.View()
	if !strings.Contains(view.Content, "gh-peek — initializing…") {
		t.Fatalf("legacy view missing placeholder; got %q", view.Content)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("legacy quit cmd nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestRouter_OpenJobLogMsgSwapsToLogViewer(t *testing.T) {
	m := newRouterForTest()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 7, Repo: repo})
	mm := updated.(*Model)
	if mm.ActiveScreenName() != "run-detail" {
		t.Fatalf("precondition: expected run-detail, got %q", mm.ActiveScreenName())
	}
	updated, cmd := mm.Update(runscreen.OpenJobLogMsg{Repo: repo, RunID: 7, JobID: 100})
	mm = updated.(*Model)
	if mm.ActiveScreenName() != "log-viewer" {
		t.Fatalf("active screen = %q, want %q", mm.ActiveScreenName(), "log-viewer")
	}
	if mm.LogScreen() == nil {
		t.Fatal("LogScreen() = nil after OpenJobLogMsg")
	}
	if cmd == nil {
		t.Fatal("expected Init cmd from log screen swap")
	}
}

func TestRouter_LogBackMsgReturnsToRunDetail(t *testing.T) {
	m := newRouterForTest()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 7, Repo: repo})
	mm := updated.(*Model)
	beforeDetail := mm.DetailScreen()
	updated, _ = mm.Update(runscreen.OpenJobLogMsg{Repo: repo, RunID: 7, JobID: 100})
	mm = updated.(*Model)
	updated, _ = mm.Update(logscreen.BackMsg{})
	mm = updated.(*Model)
	if mm.ActiveScreenName() != "run-detail" {
		t.Fatalf("active screen = %q, want %q", mm.ActiveScreenName(), "run-detail")
	}
	if mm.DetailScreen() != beforeDetail {
		t.Fatal("expected the same DetailScreen instance preserved across log Back")
	}
	if mm.LogScreen() != nil {
		t.Fatal("LogScreen should be cleared after BackMsg")
	}
}

func TestRouter_OpenJobLogMsgPropagatesJobNameAndRunActive(t *testing.T) {
	m := newRouterForTest()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 7, Repo: repo})
	mm := updated.(*Model)
	updated, _ = mm.Update(runscreen.OpenJobLogMsg{
		Repo: repo, RunID: 7, JobID: 100,
		JobName: "deploy", RunActive: true,
	})
	mm = updated.(*Model)
	lv := mm.LogScreen()
	if lv == nil {
		t.Fatal("LogScreen() = nil after OpenJobLogMsg")
	}
	if got := lv.JobName(); got != "deploy" {
		t.Errorf("LogScreen JobName = %q, want %q", got, "deploy")
	}
	if !lv.RunActive() {
		t.Error("LogScreen RunActive = false, want true")
	}
}

func TestRouter_OpenJobLogMsgPropagatesSteps(t *testing.T) {
	m := newRouterForTest()
	repo := routerStartup().Repo.Repo
	updated, _ := m.Update(runs.OpenRunMsg{RunID: 7, Repo: repo})
	mm := updated.(*Model)
	steps := []domain.WorkflowStep{
		{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success"},
		{Number: 2, Name: "Run tests", Status: "completed", Conclusion: "failure"},
	}
	updated, _ = mm.Update(runscreen.OpenJobLogMsg{
		Repo: repo, RunID: 7, JobID: 100,
		JobName: "ci", Steps: steps,
	})
	mm = updated.(*Model)
	lv := mm.LogScreen()
	if lv == nil {
		t.Fatal("LogScreen() = nil after OpenJobLogMsg")
	}
	got := lv.Steps()
	if len(got) != 2 {
		t.Fatalf("LogScreen Steps len = %d, want 2", len(got))
	}
	if got[0].Name != "Set up job" || got[1].Name != "Run tests" {
		t.Errorf("LogScreen Steps = %v, unexpected names", got)
	}
}

// Compile-time use of githubapi & domain to keep imports stable in this test file.
var _ = githubapi.ErrNotFound
var _ domain.RepoRef
