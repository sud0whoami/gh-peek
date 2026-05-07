package runs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/testutil"
)

// fakeClient is a file-local fake implementing githubapi.ActionsClient.
type fakeClient struct {
	mu      sync.Mutex
	calls   int
	filters []githubapi.ListRunsFilter
	results []githubapi.ListRunsResult
	errs    []error
}

func (f *fakeClient) push(r githubapi.ListRunsResult, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	f.errs = append(f.errs, err)
}

func (f *fakeClient) ListRuns(_ context.Context, _ domain.RepoRef, filter githubapi.ListRunsFilter) (githubapi.ListRunsResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.filters = append(f.filters, filter)
	if len(f.results) == 0 {
		return githubapi.ListRunsResult{}, nil
	}
	r := f.results[0]
	e := f.errs[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
		f.errs = f.errs[1:]
	}
	return r, e
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeClient) lastFilter() githubapi.ListRunsFilter {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.filters) == 0 {
		return githubapi.ListRunsFilter{}
	}
	return f.filters[len(f.filters)-1]
}

func (f *fakeClient) GetRun(context.Context, domain.RepoRef, int64) (domain.WorkflowRun, error) {
	return domain.WorkflowRun{}, nil
}
func (f *fakeClient) ListJobs(context.Context, domain.RepoRef, int64) ([]domain.WorkflowJob, error) {
	return nil, nil
}
func (f *fakeClient) DownloadJobLog(context.Context, domain.RepoRef, int64) ([]byte, error) {
	return nil, nil
}

// fixedNow returns a constant time used by tests to make output stable.
var fixedNow = time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

// startupAll constructs a baseline StartupContext for All-runs mode.
func startupAll() domain.StartupContext {
	return domain.StartupContext{
		Kind: domain.StartContextAll,
		Repo: domain.RepoContext{
			Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
			CurrentBranch: "main",
			DefaultBranch: "main",
			HeadSHA:       "deadbeef",
			IsDefault:     true,
		},
	}
}

func startupBranch() domain.StartupContext {
	return domain.StartupContext{
		Kind: domain.StartContextBranch,
		Repo: domain.RepoContext{
			Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
			CurrentBranch: "feature/x",
			DefaultBranch: "main",
			HeadSHA:       "abc123",
			IsDefault:     false,
		},
	}
}

func startupPR() domain.StartupContext {
	return domain.StartupContext{
		Kind: domain.StartContextPR,
		Repo: domain.RepoContext{
			Repo:          domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
			CurrentBranch: "feature/x",
			DefaultBranch: "main",
			HeadSHA:       "abc123",
			IsDefault:     false,
		},
		PR: &domain.PullRequestContext{
			Number:      42,
			Title:       "fix flaky test",
			HeadRefName: "feature/x",
			HeadRefOID:  "abc123",
		},
	}
}

// makeRun constructs a sample WorkflowRun.
func makeRun(id int64, name, title, branch, status, conclusion string) domain.WorkflowRun {
	t := fixedNow.Add(-5 * time.Minute)
	return domain.WorkflowRun{
		ID:           id,
		Name:         name,
		WorkflowName: name,
		DisplayTitle: title,
		Event:        "push",
		HeadBranch:   branch,
		HeadSHA:      "feedbeef",
		Status:       status,
		Conclusion:   conclusion,
		Attempt:      1,
		CreatedAt:    t,
		UpdatedAt:    t,
		URL:          "https://github.com/octo/demo/actions/runs/" + itoa(id),
	}
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func newModel(t *testing.T, fc *fakeClient, sc domain.StartupContext) *Model {
	t.Helper()
	m := New(Params{
		Startup:      sc,
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		AutoRefresh:  true,
		TickInterval: time.Millisecond,
	})
	return m
}

// drainInit runs the initial Init() command and feeds its output into Update.
func drainInit(t *testing.T, m *Model) *Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(*Model)
		if _, ok := msg.(RunsLoadedMsg); ok {
			break
		}
	}
	return m
}

func TestInitialLoadReady(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "Add feature", "main", "completed", "success"),
	}}, nil)
	m := newModel(t, fc, startupAll())
	m = drainInit(t, m)

	v := m.View().Content
	if !strings.Contains(v, "CI") {
		t.Errorf("view missing workflow name: %q", v)
	}
	if !strings.Contains(v, "main") {
		t.Errorf("view missing branch: %q", v)
	}
	if !strings.Contains(v, "success") {
		t.Errorf("view missing status badge text: %q", v)
	}
}

func TestInitialLoadEmpty(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	m := newModel(t, fc, startupAll())
	m = drainInit(t, m)
	v := m.View().Content
	if !strings.Contains(v, "No runs match the current filter. Press 'r' to refresh.") {
		t.Errorf("view missing empty message: %q", v)
	}
}

func TestInitialLoadError(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{}, errors.New("boom"))
	m := newModel(t, fc, startupAll())
	m = drainInit(t, m)
	v := m.View().Content
	if !strings.Contains(v, "boom") {
		t.Errorf("view missing error text: %q", v)
	}
}

func TestSentinelErrorRendering(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		fc := &fakeClient{}
		fc.push(githubapi.ListRunsResult{}, githubapi.ErrUnauthorized)
		m := drainInit(t, newModel(t, fc, startupAll()))
		v := m.View().Content
		if !strings.Contains(v, "Run `gh auth login`") {
			t.Errorf("expected unauthorized hint, got %q", v)
		}
	})
	t.Run("not_found", func(t *testing.T) {
		fc := &fakeClient{}
		fc.push(githubapi.ListRunsResult{}, githubapi.ErrNotFound)
		m := drainInit(t, newModel(t, fc, startupAll()))
		v := m.View().Content
		if !strings.Contains(v, "Repository not found") {
			t.Errorf("expected not-found hint, got %q", v)
		}
	})
	t.Run("rate_limited_with_retry_after", func(t *testing.T) {
		// Build a real *APIError via a tiny httptest server returning 429.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()
		c := githubapi.New(githubapi.WithBaseURL(srv.URL))
		_, apiErr := c.ListRuns(context.Background(),
			domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
			githubapi.ListRunsFilter{})
		if !errors.Is(apiErr, githubapi.ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited, got %v", apiErr)
		}
		fc := &fakeClient{}
		fc.push(githubapi.ListRunsResult{}, apiErr)
		m := drainInit(t, newModel(t, fc, startupAll()))
		v := m.View().Content
		if !strings.Contains(v, "Rate limited") {
			t.Errorf("expected rate-limited hint, got %q", v)
		}
	})
}

func TestSelection(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
		makeRun(2, "CI", "b", "main", "completed", "success"),
		makeRun(3, "CI", "c", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	for _, k := range []string{"j", "j", "k"} {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(*Model)
	}
	if got := m.SelectedIndex(); got != 1 {
		t.Errorf("SelectedIndex = %d, want 1", got)
	}
}

func TestEnterEmitsOpenRunMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(11, "CI", "a", "main", "completed", "success"),
		makeRun(22, "CI", "b", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter returned nil cmd")
	}
	msg := cmd()
	op, ok := msg.(OpenRunMsg)
	if !ok {
		t.Fatalf("expected OpenRunMsg, got %T", msg)
	}
	if op.RunID != 22 {
		t.Errorf("RunID = %d, want 22", op.RunID)
	}
	if op.Repo.Owner != "octo" || op.Repo.Name != "demo" {
		t.Errorf("Repo = %+v", op.Repo)
	}
}

func TestSearchToggle(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	if m.IsSearching() {
		t.Fatal("expected not searching initially")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	if !m.IsSearching() {
		t.Fatal("expected searching after /")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 27, Text: "esc"})
	m = next.(*Model)
	if m.IsSearching() {
		t.Fatal("expected not searching after esc")
	}
}

func TestActiveOnlyFilter(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "done", "main", "completed", "success"),
		makeRun(2, "CI", "running", "main", "in_progress", ""),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	if got := m.VisibleRunCount(); got != 2 {
		t.Fatalf("VisibleRunCount = %d, want 2", got)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(*Model)
	if got := m.VisibleRunCount(); got != 1 {
		t.Errorf("VisibleRunCount after 'a' = %d, want 1", got)
	}
}

func TestRefreshFetchesAgain(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
	}}, nil)
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	before := fc.callCount()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("expected cmd from refresh")
	}
	msg := cmd()
	if _, ok := msg.(RunsLoadedMsg); !ok {
		t.Fatalf("expected RunsLoadedMsg from refresh, got %T", msg)
	}
	if fc.callCount() != before+1 {
		t.Errorf("call count = %d, want %d", fc.callCount(), before+1)
	}
}

func TestTickPausedWhileTyping(t *testing.T) {
	fc := &fakeClient{}
	// Active run so ticks are not suppressed by all-completed rule.
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "running", "main", "in_progress", ""),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	// Focus the input.
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	before := fc.callCount()
	next, cmd := m.Update(TickMsg{})
	m = next.(*Model)
	// Tick should reschedule but NOT fetch.
	if cmd != nil {
		// Run cmd; should be a tick reschedule, not a fetch.
		msg := cmd()
		if _, ok := msg.(RunsLoadedMsg); ok {
			t.Fatalf("tick while typing should not fetch")
		}
	}
	if fc.callCount() != before {
		t.Errorf("expected no fetch while typing; got %d -> %d", before, fc.callCount())
	}
	// Cancel input.
	next, _ = m.Update(tea.KeyPressMsg{Code: 27, Text: "esc"})
	m = next.(*Model)
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "running", "main", "in_progress", ""),
	}}, nil)
	before = fc.callCount()
	_, cmd = m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("tick after esc returned nil cmd")
	}
	// Drain commands until we observe a fetch or run out.
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		if _, ok := msg.(RunsLoadedMsg); ok {
			break
		}
		// If it was a tickMsg reschedule, stop to avoid recursion.
		if _, ok := msg.(TickMsg); ok {
			break
		}
		cmd = nil
	}
	if fc.callCount() != before+1 {
		t.Errorf("expected fetch after esc; got %d -> %d", before, fc.callCount())
	}
}

func TestTickStopsWhenAllCompleted(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(RunsLoadedMsg); ok {
			t.Fatalf("tick should not fetch when only completed runs")
		}
	}
	if fc.callCount() != before {
		t.Errorf("call count changed: %d -> %d", before, fc.callCount())
	}
}

func TestAutoRefreshToggle(t *testing.T) {
	fc := &fakeClient{}
	// Active run keeps ticks alive.
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "in_progress", ""),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	// Toggle OFF.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(RunsLoadedMsg); ok {
			t.Fatal("tick should not fetch while auto-refresh off")
		}
	}
	if fc.callCount() != before {
		t.Errorf("fetched while auto-refresh off: %d -> %d", before, fc.callCount())
	}
	// Toggle back ON.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "in_progress", ""),
	}}, nil)
	before = fc.callCount()
	_, cmd = m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("expected cmd from tick after re-enable")
	}
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		if _, ok := msg.(RunsLoadedMsg); ok {
			break
		}
		if _, ok := msg.(TickMsg); ok {
			break
		}
		cmd = nil
	}
	if fc.callCount() != before+1 {
		t.Errorf("expected fetch after re-enable: %d -> %d", before, fc.callCount())
	}
}

func TestETagRoundTrip(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{
		Runs: []domain.WorkflowRun{makeRun(1, "CI", "a", "main", "completed", "success")},
		ETag: `"abc"`,
	}, nil)
	fc.push(githubapi.ListRunsResult{
		Runs: []domain.WorkflowRun{makeRun(1, "CI", "a", "main", "completed", "success")},
		ETag: `"abc"`,
	}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	if got := m.LastETag(); got != `"abc"` {
		t.Errorf("LastETag = %q, want \"abc\"", got)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(*Model)
	_ = m
	if cmd == nil {
		t.Fatal("refresh returned nil cmd")
	}
	_ = cmd()
	if got := fc.lastFilter().IfNoneMatch; got != `"abc"` {
		t.Errorf("IfNoneMatch on refresh = %q, want \"abc\"", got)
	}
}

func TestNotModifiedKeepsData(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{
		Runs: []domain.WorkflowRun{
			makeRun(1, "CI", "a", "main", "completed", "success"),
			makeRun(2, "CI", "b", "main", "completed", "success"),
		},
		ETag: `"x"`,
	}, nil)
	fc.push(githubapi.ListRunsResult{
		Runs:        nil,
		NotModified: true,
		ETag:        `"x"`,
	}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	if got := m.VisibleRunCount(); got != 2 {
		t.Fatalf("VisibleRunCount before = %d, want 2", got)
	}
	t1, _ := m.LastRefreshed()
	// Bump fixedNow by advancing through a refresh.
	prevNow := fixedNow
	fixedNow = fixedNow.Add(10 * time.Second)
	defer func() { fixedNow = prevNow }()

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("refresh nil cmd")
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(*Model)

	if got := m.VisibleRunCount(); got != 2 {
		t.Errorf("VisibleRunCount after NotModified = %d, want 2", got)
	}
	t2, ok := m.LastRefreshed()
	if !ok {
		t.Fatal("LastRefreshed not set")
	}
	if !t2.After(t1) {
		t.Errorf("LastRefreshed did not advance: %v -> %v", t1, t2)
	}
}

func TestWindowResizeWidth(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI-with-a-very-long-workflow-name", "a long display title that should be truncated", "main", "completed", "success"),
		makeRun(2, "Another", "b", "feature/long-branch-name-here", "completed", "failure"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(*Model)
	v := m.View().Content
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line width %d > 60: %q", w, line)
		}
	}
}

func TestQuitOnQ(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "a", "main", "completed", "success"),
	}}, nil)
	m := drainInit(t, newModel(t, fc, startupAll()))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q returned nil cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg")
	}
}

func TestFilterFromStartup(t *testing.T) {
	cases := []struct {
		name    string
		sc      domain.StartupContext
		wantBr  string
		wantSHA string
	}{
		{"all", startupAll(), "", ""},
		{"branch", startupBranch(), "feature/x", ""},
		{"pr", startupPR(), "", "abc123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{}
			fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
			_ = drainInit(t, newModel(t, fc, tc.sc))
			f := fc.lastFilter()
			if f.Branch != tc.wantBr {
				t.Errorf("Branch = %q, want %q", f.Branch, tc.wantBr)
			}
			if f.HeadSHA != tc.wantSHA {
				t.Errorf("HeadSHA = %q, want %q", f.HeadSHA, tc.wantSHA)
			}
		})
	}
}

// TestGoldenRunsReady locks the lipgloss color profile and asserts a
// stable rendering of a populated runs view.
//
// To regenerate the golden file:
//
//	UPDATE_GOLDEN=1 go test ./internal/ui/screens/runs/... -run TestGoldenRunsReady -count=1
func TestGoldenRunsReady(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: []domain.WorkflowRun{
		makeRun(1, "CI", "Add feature", "main", "completed", "success"),
		makeRun(2, "CI", "Refactor module", "main", "completed", "failure"),
		makeRun(3, "CI", "Wire bootstrap", "main", "in_progress", ""),
	}}, nil)
	m := New(Params{
		Startup:     startupAll(),
		Client:      fc,
		Now:         func() time.Time { return fixedNow },
		Width:       100,
		Height:      24,
		AutoRefresh: true,
	})
	m = drainInit(t, m)
	rendered := m.View().Content
	normalized := testutil.NormalizeTimestamps(rendered)
	testutil.AssertGolden(t, "runs_ready", normalized)
}

// pressKey delivers a single keypress to the model and returns the new model.
func pressKey(t *testing.T, m *Model, s string) *Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: rune(s[0]), Text: s})
	return next.(*Model)
}

func TestRunsScreen_BCycle_BranchAndPR(t *testing.T) {
	fc := &fakeClient{}
	// initial fetch + three more after each `b`.
	for i := 0; i < 4; i++ {
		fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	}
	m := newModel(t, fc, startupPR())
	m = drainInit(t, m)
	if got := m.ViewMode(); got != "pr" {
		t.Fatalf("initial ViewMode = %q, want pr", got)
	}

	// pr -> all
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "all" {
		t.Errorf("after 1x b: ViewMode = %q, want all", got)
	}
	// drive the fetch cmd
	cmd := m.fetchCmd()
	if cmd != nil {
		next, _ := m.Update(cmd())
		m = next.(*Model)
	}

	// all -> branch
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "branch" {
		t.Errorf("after 2x b: ViewMode = %q, want branch", got)
	}
	// branch -> pr (wraps)
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "pr" {
		t.Errorf("after 3x b: ViewMode = %q, want pr", got)
	}
}

func TestRunsScreen_BCycle_BranchOnly(t *testing.T) {
	fc := &fakeClient{}
	for i := 0; i < 3; i++ {
		fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	}
	m := newModel(t, fc, startupBranch())
	m = drainInit(t, m)
	if got := m.ViewMode(); got != "branch" {
		t.Fatalf("initial ViewMode = %q, want branch", got)
	}
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "all" {
		t.Errorf("after 1x b: ViewMode = %q, want all", got)
	}
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "branch" {
		t.Errorf("after 2x b: ViewMode = %q, want branch (toggle)", got)
	}
}

func TestRunsScreen_BCycle_NoOpOnDefaultBranch(t *testing.T) {
	// startupAll has CurrentBranch=main (default) and no PR.
	// After applicable filtering: only "all" mode is selectable when
	// branch matches default? In our impl we treat any non-empty
	// CurrentBranch as "branch" applicable, so this case actually
	// has both branch and all available. Document by switching to
	// a no-branch context to assert true no-op.
	fc := &fakeClient{}
	fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	sc := startupAll()
	sc.Repo.CurrentBranch = ""
	m := newModel(t, fc, sc)
	m = drainInit(t, m)
	if got := m.ViewMode(); got != "all" {
		t.Fatalf("initial ViewMode = %q, want all", got)
	}
	m = pressKey(t, m, "b")
	if got := m.ViewMode(); got != "all" {
		t.Errorf("after b: ViewMode = %q, want all (no-op)", got)
	}
}

func TestRunsScreen_BCycle_FilterReflectsMode(t *testing.T) {
	fc := &fakeClient{}
	for i := 0; i < 4; i++ {
		fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	}
	m := newModel(t, fc, startupPR())
	m = drainInit(t, m)

	// Starting view: PR -> filter has HeadSHA, no branch.
	if f := fc.lastFilter(); f.HeadSHA != "abc123" || f.Branch != "" {
		t.Errorf("initial filter = %+v, want HeadSHA=abc123 Branch=\"\"", f)
	}

	// b -> all
	m = pressKey(t, m, "b")
	// Drive the fetch the keypress queued.
	cmd := m.fetchCmd()
	if cmd != nil {
		next, _ := m.Update(cmd())
		m = next.(*Model)
	}
	if f := fc.lastFilter(); f.HeadSHA != "" || f.Branch != "" {
		t.Errorf("after b->all filter = %+v, want both empty", f)
	}

	// b -> branch
	m = pressKey(t, m, "b")
	cmd = m.fetchCmd()
	if cmd != nil {
		m.Update(cmd()) //nolint:errcheck
	}
	if f := fc.lastFilter(); f.Branch != "feature/x" || f.HeadSHA != "" {
		t.Errorf("after b->branch filter = %+v, want Branch=feature/x HeadSHA=\"\"", f)
	}
}

func TestBannerReflectsViewMode(t *testing.T) {
	// Start in PR view, then cycle to all, then branch.
	fc := &fakeClient{}
	for i := 0; i < 4; i++ {
		fc.push(githubapi.ListRunsResult{Runs: nil}, nil)
	}
	m := newModel(t, fc, startupPR())
	m = drainInit(t, m)

	// Initial view = PR
	view := m.View().Content
	if !strings.Contains(view, "PR #42") {
		t.Errorf("initial banner: want PR #42, got:\n%s", view)
	}
	if !strings.Contains(view, "pr:42") {
		t.Errorf("initial filter line: want pr:42, got:\n%s", view)
	}

	// b -> all
	m = pressKey(t, m, "b")
	view = m.View().Content
	if !strings.Contains(view, "All runs") {
		t.Errorf("after b->all banner: want All runs, got:\n%s", view)
	}
	if strings.Contains(view, "PR #") {
		t.Errorf("after b->all banner: should not contain PR #, got:\n%s", view)
	}

	// b -> branch
	m = pressKey(t, m, "b")
	view = m.View().Content
	if !strings.Contains(view, "Branch runs") {
		t.Errorf("after b->branch banner: want Branch runs, got:\n%s", view)
	}
	if !strings.Contains(view, "branch:feature/x") {
		t.Errorf("after b->branch filter line: want branch:feature/x, got:\n%s", view)
	}
}
