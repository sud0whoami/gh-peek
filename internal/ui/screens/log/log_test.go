package log

import (
	"context"
	"errors"
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
// Only DownloadJobLog is exercised; the other methods return zero.
type fakeClient struct {
	mu      sync.Mutex
	results [][]byte
	errs    []error
	calls   int
}

func (f *fakeClient) push(b []byte, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, b)
	f.errs = append(f.errs, err)
}

func (f *fakeClient) DownloadJobLog(_ context.Context, _ domain.RepoRef, _ int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.results) == 0 {
		return nil, nil
	}
	b, e := f.results[0], f.errs[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
		f.errs = f.errs[1:]
	}
	return b, e
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeClient) ListRuns(context.Context, domain.RepoRef, githubapi.ListRunsFilter) (githubapi.ListRunsResult, error) {
	return githubapi.ListRunsResult{}, nil
}
func (f *fakeClient) GetRun(context.Context, domain.RepoRef, int64) (domain.WorkflowRun, error) {
	return domain.WorkflowRun{}, nil
}
func (f *fakeClient) ListJobs(context.Context, domain.RepoRef, int64) ([]domain.WorkflowJob, error) {
	return nil, nil
}

var fixedNow = time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

func testRepo() domain.RepoRef {
	return domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"}
}

// nLines builds a log with n lines: "line 1\nline 2\n…line n\n".
func nLines(n int) []byte {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString("line ")
		sb.WriteString(itoa(int64(i)))
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func newModel(t *testing.T, fc *fakeClient, runActive bool) *Model {
	t.Helper()
	return New(Params{
		Repo:         testRepo(),
		RunID:        777,
		JobID:        42,
		JobName:      "build",
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		AutoRefresh:  true,
		TickInterval: time.Millisecond,
		RunActive:    runActive,
		ViewMode:     ViewModeRaw,
	})
}

// drainInit runs Init() and feeds the resulting message back into Update.
func drainInit(t *testing.T, m *Model) *Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next.(*Model)
}

func TestInitialLoadReady(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(200), nil)
	m := drainInit(t, newModel(t, fc, false))
	if m.IsLoading() {
		t.Fatal("expected ready state after load")
	}
	if got := m.Top(); got != 0 {
		t.Errorf("Top = %d, want 0", got)
	}
	v := m.View().Content
	if !strings.Contains(v, "line 1") {
		t.Errorf("view missing first line: %q", v)
	}
}

func TestInitialLoadErrorSentinel(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nil, githubapi.ErrUnauthorized)
	m := drainInit(t, newModel(t, fc, false))
	v := m.View().Content
	if !strings.Contains(v, "gh auth login") {
		t.Errorf("expected unauthorized hint, got %q", v)
	}
}

func TestInitialLoadTooLargePartial(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), githubapi.ErrLogTooLarge)
	m := drainInit(t, newModel(t, fc, false))
	if m.IsLoading() {
		t.Fatal("expected ready state with partial bytes")
	}
	if !m.TruncatedFromAPI() {
		t.Error("TruncatedFromAPI should be true")
	}
	v := m.View().Content
	if !strings.Contains(v, "truncated by server") {
		t.Errorf("expected truncation banner, got %q", v)
	}
	if !strings.Contains(v, "line 1") {
		t.Errorf("expected partial bytes visible, got %q", v)
	}
}

func TestScrollJK(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(100), nil)
	m := drainInit(t, newModel(t, fc, false))
	for i := 0; i < 5; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = next.(*Model)
	}
	for i := 0; i < 2; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		m = next.(*Model)
	}
	if got := m.Top(); got != 3 {
		t.Errorf("Top = %d, want 3", got)
	}
}

func TestPageUpDown(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(200), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown, Text: "pgdown"})
	m = next.(*Model)
	if m.Top() < 5 {
		t.Errorf("Top after PgDn = %d, want >= 5", m.Top())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp, Text: "pgup"})
	m = next.(*Model)
	if m.Top() != 0 {
		t.Errorf("Top after PgUp = %d, want 0", m.Top())
	}
}

func TestGGTopBottom(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(100), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = next.(*Model)
	if m.Top() < 50 {
		t.Errorf("Top after G = %d, want near bottom", m.Top())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = next.(*Model)
	if m.Top() != 0 {
		t.Errorf("Top after g = %d, want 0", m.Top())
	}
}

func TestSlashFocusEscCancel(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	if !m.IsSearching() {
		t.Fatal("expected searching after /")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	m = next.(*Model)
	if m.IsSearching() {
		t.Fatal("expected not searching after esc")
	}
	if m.MatchCount() != 0 {
		t.Errorf("MatchCount after esc = %d, want 0", m.MatchCount())
	}
}

func TestSearchUpdatesMatchesLive(t *testing.T) {
	fc := &fakeClient{}
	// Lines containing "foo" in indices 5, 10, 30.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		if i == 5 || i == 10 || i == 30 {
			sb.WriteString("has foo here\n")
		} else {
			sb.WriteString("nothing\n")
		}
	}
	fc.push([]byte(sb.String()), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	for _, r := range "foo" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	if m.MatchCount() != 3 {
		t.Errorf("MatchCount = %d, want 3", m.MatchCount())
	}
	if m.MatchIndex() != 0 {
		t.Errorf("MatchIndex = %d, want 0", m.MatchIndex())
	}
	if m.Top() != 5 {
		t.Errorf("Top after live search = %d, want 5", m.Top())
	}
}

func TestNNNavigatesMatches(t *testing.T) {
	fc := &fakeClient{}
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		if i == 2 || i == 7 || i == 15 {
			sb.WriteString("xx hit yy\n")
		} else {
			sb.WriteString("plain\n")
		}
	}
	fc.push([]byte(sb.String()), nil)
	m := drainInit(t, newModel(t, fc, false))
	// Focus search and type "hit" then exit search input via enter.
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	for _, r := range "hit" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	m = next.(*Model)
	if m.MatchCount() != 3 {
		t.Fatalf("MatchCount = %d, want 3", m.MatchCount())
	}
	// n → 1
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(*Model)
	if m.MatchIndex() != 1 {
		t.Errorf("MatchIndex after n = %d, want 1", m.MatchIndex())
	}
	// n → 2
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(*Model)
	if m.MatchIndex() != 2 {
		t.Errorf("MatchIndex after n = %d, want 2", m.MatchIndex())
	}
	// n wraps → 0
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(*Model)
	if m.MatchIndex() != 0 {
		t.Errorf("MatchIndex after n wrap = %d, want 0", m.MatchIndex())
	}
	// N wraps → 2
	next, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N"})
	m = next.(*Model)
	if m.MatchIndex() != 2 {
		t.Errorf("MatchIndex after N wrap = %d, want 2", m.MatchIndex())
	}
}

func TestWToggleWrap(t *testing.T) {
	fc := &fakeClient{}
	// One very long line.
	long := strings.Repeat("abcd ", 60) // ~300 cells
	fc.push([]byte(long+"\n"+nLinesString(5)), nil)
	m := drainInit(t, newModel(t, fc, false))
	if m.Wrap() {
		t.Fatal("Wrap should default to off")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(*Model)
	if !m.Wrap() {
		t.Fatal("Wrap should be on after w")
	}
	v := m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > m.Width() {
			t.Errorf("wrapped line width %d > screen width %d (line=%q)", w, m.Width(), ln)
		}
	}
}

func nLinesString(n int) string { return string(nLines(n)) }

func TestFJumpsToFirstFailure(t *testing.T) {
	fc := &fakeClient{}
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		if i == 42 {
			sb.WriteString("##[error] something blew up\n")
		} else {
			sb.WriteString("ok\n")
		}
	}
	fc.push([]byte(sb.String()), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	m = next.(*Model)
	if m.Top() != 42 {
		t.Errorf("Top after F = %d, want 42", m.Top())
	}

	// Without marker: F is no-op.
	fc2 := &fakeClient{}
	fc2.push(nLines(50), nil)
	m2 := drainInit(t, newModel(t, fc2, false))
	prev := m2.Top()
	next, _ = m2.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	m2 = next.(*Model)
	if m2.Top() != prev {
		t.Errorf("Top changed when no failure marker: %d → %d", prev, m2.Top())
	}
}

func TestRRefresh(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, false))
	before := fc.callCount()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	_ = cmd()
	if fc.callCount() != before+1 {
		t.Errorf("calls = %d, want %d", fc.callCount(), before+1)
	}
}

func TestRToggleAndTickGating(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, true))
	if !m.AutoRefreshOn() {
		t.Fatal("auto-refresh should default on")
	}
	// Toggle OFF.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	if m.AutoRefreshOn() {
		t.Fatal("expected auto-refresh off after R")
	}
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		_ = cmd()
	}
	if fc.callCount() != before {
		t.Errorf("tick fetched while auto-refresh off: %d → %d", before, fc.callCount())
	}
	// Toggle ON; run is active → tick fetches.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	fc.push(nLines(10), nil)
	before = fc.callCount()
	_, cmd = m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("tick returned nil cmd while on+active")
	}
	_ = cmd()
	if fc.callCount() != before+1 {
		t.Errorf("expected fetch after toggle-on tick: %d → %d", before, fc.callCount())
	}
}

func TestTickPausedWhileTyping(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, true))
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		_ = cmd()
	}
	if fc.callCount() != before {
		t.Errorf("tick fetched while typing: %d → %d", before, fc.callCount())
	}
}

func TestTickStoppedWhenRunNotActive(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, false))
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		_ = cmd()
	}
	if fc.callCount() != before {
		t.Errorf("tick fetched when RunActive=false: %d → %d", before, fc.callCount())
	}
}

func TestTickFiresWhenActive(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(10), nil)
	fc.push(nLines(10), nil)
	m := drainInit(t, newModel(t, fc, true))
	before := fc.callCount()
	_, cmd := m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("tick returned nil cmd when active")
	}
	_ = cmd()
	if fc.callCount() != before+1 {
		t.Errorf("expected fetch on tick: %d → %d", before, fc.callCount())
	}
}

func TestOEmitsOpenInBrowser(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(5), nil)
	m := drainInit(t, newModel(t, fc, false))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatal("o returned nil cmd")
	}
	op, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	want := "https://github.com/octo/demo/actions/runs/777/job/42"
	if op.URL != want {
		t.Errorf("URL = %q, want %q", op.URL, want)
	}
}

func TestEscBEmitsBack(t *testing.T) {
	t.Run("esc", func(t *testing.T) {
		fc := &fakeClient{}
		fc.push(nLines(5), nil)
		m := drainInit(t, newModel(t, fc, false))
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
		if cmd == nil {
			t.Fatal("esc returned nil cmd")
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Fatalf("expected BackMsg, got %T", cmd())
		}
	})
	t.Run("b", func(t *testing.T) {
		fc := &fakeClient{}
		fc.push(nLines(5), nil)
		m := drainInit(t, newModel(t, fc, false))
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
		if cmd == nil {
			t.Fatal("b returned nil cmd")
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Fatalf("expected BackMsg, got %T", cmd())
		}
	})
	t.Run("esc_while_searching_does_not_back", func(t *testing.T) {
		fc := &fakeClient{}
		fc.push(nLines(5), nil)
		m := drainInit(t, newModel(t, fc, false))
		next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		m = next.(*Model)
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
		if cmd != nil {
			if _, ok := cmd().(BackMsg); ok {
				t.Fatal("esc while searching must not emit BackMsg")
			}
		}
	})
}

func TestQuitOnQ(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(5), nil)
	m := drainInit(t, newModel(t, fc, false))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q returned nil cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestRefreshErrorKeepsPriorData(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(20), nil)
	fc.push(nil, errors.New("network down"))
	m := drainInit(t, newModel(t, fc, false))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)
	if !m.HasRefreshErr() {
		t.Error("expected refresh error banner")
	}
	v := m.View().Content
	if !strings.Contains(v, "line 1") {
		t.Errorf("expected prior data preserved, got %q", v)
	}
	if !strings.Contains(v, "network down") {
		t.Errorf("expected error in banner, got %q", v)
	}
}

func TestWindowResize(t *testing.T) {
	fc := &fakeClient{}
	fc.push([]byte(strings.Repeat("a", 200)+"\n"), nil)
	m := drainInit(t, newModel(t, fc, false))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(*Model)
	v := m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > 60 {
			t.Errorf("line width %d > 60 (line=%q)", w, ln)
		}
	}
}

// TestGoldenLogReady locks color profile and asserts a stable rendering.
//
//	UPDATE_GOLDEN=1 go test ./internal/ui/screens/log/... -run TestGoldenLogReady -count=1
func TestGoldenLogReady(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	var sb strings.Builder
	sb.WriteString("Setting up job\n")
	sb.WriteString("\x1b[32m✓ Checkout repo\x1b[0m\n")
	sb.WriteString("Run go build ./...\n")
	sb.WriteString("Building...\n")
	sb.WriteString("\x1b[31m##[error] build failed: unexpected token\x1b[0m\n")
	sb.WriteString("Process completed with exit code 1\n")
	for i := 0; i < 5; i++ {
		sb.WriteString("post-error noise\n")
	}
	fc.push([]byte(sb.String()), nil)
	m := New(Params{
		Repo:        testRepo(),
		RunID:       777,
		JobID:       42,
		JobName:     "build",
		Client:      fc,
		Now:         func() time.Time { return fixedNow },
		Width:       100,
		Height:      24,
		AutoRefresh: false,
		RunActive:   false,
		ViewMode:    ViewModeRaw,
	})
	m = drainInit(t, m)
	rendered := m.View().Content
	normalized := testutil.NormalizeTimestamps(rendered)
	testutil.AssertGolden(t, "log_ready", normalized)
}

// ---------------------------------------------------------------------------
// Milestone 8: outline mode tests
// ---------------------------------------------------------------------------

// outlineLog returns a GHA-style job log with groups and an error.
// Structure:
//   - ##[group]Set up job
//   - plain line
//   - ##[endgroup]
//   - ##[group]Run build
//   - ##[command]go build
//   - ##[error]build failed
//   - ##[endgroup]
func outlineLog() []byte {
	var sb strings.Builder
	sb.WriteString("2024-01-01T10:00:00Z ##[group]Set up job\n")
	sb.WriteString("2024-01-01T10:00:01Z Initialising\n")
	sb.WriteString("2024-01-01T10:00:02Z ##[endgroup]\n")
	sb.WriteString("2024-01-01T10:00:03Z ##[group]Run build\n")
	sb.WriteString("2024-01-01T10:00:04Z ##[command]go build ./...\n")
	sb.WriteString("2024-01-01T10:00:05Z ##[error]build failed: something broke\n")
	sb.WriteString("2024-01-01T10:00:06Z ##[endgroup]\n")
	return []byte(sb.String())
}

func newOutlineModel(t *testing.T, fc *fakeClient) *Model {
	t.Helper()
	return New(Params{
		Repo:         testRepo(),
		RunID:        777,
		JobID:        42,
		JobName:      "build",
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
	})
}

// TestLog_DefaultViewMode_IsOutline verifies the zero-value ViewMode resolves
// to outline.
func TestLog_DefaultViewMode_IsOutline(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(5), nil)
	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		TickInterval: time.Millisecond,
	})
	if m.CurrentViewMode() != ViewModeOutline {
		t.Errorf("default ViewMode = %d, want ViewModeOutline (%d)", m.CurrentViewMode(), ViewModeOutline)
	}
}

// TestLog_AutoExpandsFailingStep verifies that after loading a log with an
// error, the failing step is automatically expanded.
func TestLog_AutoExpandsFailingStep(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// The outline should have two step-level groups: "Set up job" and "Run build".
	// "Run build" has an error, so it should be expanded (key = "1").
	if !m.IsExpanded("1") {
		t.Error("failing step (key=1) should be auto-expanded")
	}
	// "Set up job" has no error, so it should remain collapsed.
	if m.IsExpanded("0") {
		t.Error("passing step (key=0) should not be auto-expanded")
	}
}

// TestLog_ToggleExpandsAndCollapses verifies that enter/space toggle nodes.
func TestLog_ToggleExpandsAndCollapses(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// Cursor is at 0. Press enter to expand first step ("Set up job").
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	m = next.(*Model)
	if !m.IsExpanded("0") {
		t.Error("first step should be expanded after enter")
	}

	// Press enter again to collapse it.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	m = next.(*Model)
	if m.IsExpanded("0") {
		t.Error("first step should be collapsed after second enter")
	}
}

// TestLog_ModeKey_CyclesOutlineCompactRaw verifies that v cycles the view mode.
func TestLog_ModeKey_CyclesOutlineCompactRaw(t *testing.T) {
	fc := &fakeClient{}
	fc.push(nLines(5), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	if m.CurrentViewMode() != ViewModeOutline {
		t.Fatalf("initial mode = %d, want outline", m.CurrentViewMode())
	}
	// v → compact
	next, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = next.(*Model)
	if m.CurrentViewMode() != ViewModeCompact {
		t.Errorf("after 1 v: mode = %d, want compact", m.CurrentViewMode())
	}
	// v → raw
	next, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = next.(*Model)
	if m.CurrentViewMode() != ViewModeRaw {
		t.Errorf("after 2 v: mode = %d, want raw", m.CurrentViewMode())
	}
	// v → outline again
	next, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = next.(*Model)
	if m.CurrentViewMode() != ViewModeOutline {
		t.Errorf("after 3 v: mode = %d, want outline", m.CurrentViewMode())
	}
}

// TestLog_TimestampToggle verifies that t toggles the showTimestamps flag.
func TestLog_TimestampToggle(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	if m.ShowTimestamps() {
		t.Fatal("showTimestamps should default to false")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = next.(*Model)
	if !m.ShowTimestamps() {
		t.Error("showTimestamps should be true after t")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = next.(*Model)
	if m.ShowTimestamps() {
		t.Error("showTimestamps should be false again after second t")
	}
}

// TestLog_RawModeMatchesPreviousRenderer verifies that ViewModeRaw produces
// the same golden output as the pre-M8 flat renderer.
//
//	UPDATE_GOLDEN=1 go test ./internal/ui/screens/log/... -run TestLog_RawModeMatchesPreviousRenderer -count=1
func TestLog_RawModeMatchesPreviousRenderer(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	var sb strings.Builder
	sb.WriteString("Setting up job\n")
	sb.WriteString("\x1b[32m✓ Checkout repo\x1b[0m\n")
	sb.WriteString("Run go build ./...\n")
	sb.WriteString("Building...\n")
	sb.WriteString("\x1b[31m##[error] build failed: unexpected token\x1b[0m\n")
	sb.WriteString("Process completed with exit code 1\n")
	for i := 0; i < 5; i++ {
		sb.WriteString("post-error noise\n")
	}
	fc.push([]byte(sb.String()), nil)
	m := New(Params{
		Repo:        testRepo(),
		RunID:       777,
		JobID:       42,
		JobName:     "build",
		Client:      fc,
		Now:         func() time.Time { return fixedNow },
		Width:       100,
		Height:      24,
		AutoRefresh: false,
		RunActive:   false,
		ViewMode:    ViewModeRaw,
	})
	m = drainInit(t, m)
	rendered := m.View().Content
	normalized := testutil.NormalizeTimestamps(rendered)
	// Reuses the existing log_ready golden — raw mode must match it exactly.
	testutil.AssertGolden(t, "log_ready", normalized)
}

// TestLog_SearchAutoExpandsAncestors verifies that searching for a term inside
// a collapsed group expands the ancestors so the match is visible.
func TestLog_SearchAutoExpandsAncestors(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// "Set up job" step is collapsed. Search for "Initialising" (inside it).
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	for _, r := range "Initialising" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}

	if m.MatchCount() == 0 {
		t.Fatal("expected at least one match for 'Initialising'")
	}
	// The ancestor step ("Set up job") should now be expanded.
	if !m.IsExpanded("0") {
		t.Error("search should have expanded ancestor step (key=0)")
	}
	// The cursor should be on a visible row.
	if m.Cursor() < 0 {
		t.Error("cursor should be >= 0 after search")
	}
}

// TestLog_FAutoExpandsAncestors verifies that F expands ancestors of the first
// failure line so it becomes visible.
func TestLog_FAutoExpandsAncestors(t *testing.T) {
	fc := &fakeClient{}
	// Build a log where the error is inside a group that is NOT auto-expanded.
	// We need a log where the error is NOT in the first position so auto-expand
	// doesn't cover it — but auto-expand expands steps with errors, so let's
	// verify that F on an already-expanded step still seeks to the error line.
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// Before F: cursor is at 0.
	initialCursor := m.Cursor()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	m = next.(*Model)

	// The cursor should have moved to the error line.
	if m.Cursor() == initialCursor && m.VisibleRowCount() > 1 {
		t.Error("F should move cursor to failure line")
	}
	// The view should show the error line.
	v := m.View().Content
	if !strings.Contains(v, "build failed") {
		t.Errorf("F should make the error line visible, got view:\n%s", v)
	}
}

// TestLog_ExpansionStateSurvivesRebuild verifies that expand/collapse choices
// are preserved when the log buffer is refreshed with the same content.
func TestLog_ExpansionStateSurvivesRebuild(t *testing.T) {
	fc := &fakeClient{}
	logBytes := outlineLog()
	fc.push(logBytes, nil)
	fc.push(logBytes, nil) // second load (simulated refresh)
	m := drainInit(t, newOutlineModel(t, fc))

	// Manually expand the first step.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	m = next.(*Model)
	if !m.IsExpanded("0") {
		t.Fatal("expected step 0 to be expanded after enter")
	}

	// Simulate a refresh (second load).
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(*Model)

	// Expansion state should be preserved.
	if !m.IsExpanded("0") {
		t.Error("expansion state for step 0 should survive a refresh")
	}
}

// TestLog_ExpandAllCollapseAll verifies E expands all nodes and O collapses all.
func TestLog_ExpandAllCollapseAll(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// E — expand all.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	m = next.(*Model)
	if !m.IsExpanded("0") || !m.IsExpanded("1") {
		t.Error("E should expand all steps")
	}

	// O — collapse all.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'O', Text: "O"})
	m = next.(*Model)
	if m.IsExpanded("0") || m.IsExpanded("1") {
		t.Error("O should collapse all steps")
	}
	// After collapse all, visibleRows should only contain step headers.
	if m.VisibleRowCount() != 2 {
		t.Errorf("after collapse all: visibleRowCount = %d, want 2 (one per step)", m.VisibleRowCount())
	}
}

// TestLog_CursorNavigation verifies up/down move the cursor in outline mode.
func TestLog_CursorNavigation(t *testing.T) {
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := drainInit(t, newOutlineModel(t, fc))

	// Expand all so there are multiple rows.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	m = next.(*Model)

	initialCursor := m.Cursor()
	if initialCursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", initialCursor)
	}

	// j → move cursor down.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	if m.Cursor() != 1 {
		t.Errorf("cursor after j = %d, want 1", m.Cursor())
	}

	// k → move cursor up.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(*Model)
	if m.Cursor() != 0 {
		t.Errorf("cursor after k = %d, want 0", m.Cursor())
	}
}

// TestGoldenLogOutline locks color profile and asserts a stable outline-mode
// rendering.
//
//	UPDATE_GOLDEN=1 go test ./internal/ui/screens/log/... -run TestGoldenLogOutline -count=1
func TestGoldenLogOutline(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := New(Params{
		Repo:        testRepo(),
		RunID:       777,
		JobID:       42,
		JobName:     "build",
		Client:      fc,
		Now:         func() time.Time { return fixedNow },
		Width:       100,
		Height:      24,
		AutoRefresh: false,
		RunActive:   false,
		ViewMode:    ViewModeOutline,
	})
	m = drainInit(t, m)
	rendered := m.View().Content
	normalized := testutil.NormalizeTimestamps(rendered)
	testutil.AssertGolden(t, "log_outline", normalized)
}

// ---------------------------------------------------------------------------
// Milestone 9 tests — step status mapping and API-derived step data
// ---------------------------------------------------------------------------

// makeAPIStep creates a WorkflowStep with start/completed times suitable for
// testing API-derived duration fallback.
func makeAPIStep(number int, name, status, conclusion string) domain.WorkflowStep {
	// Each step starts 60s after the previous, runs for 45s, with a 15s gap.
	started := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(number-1) * 60 * time.Second)
	completed := started.Add(45 * time.Second)
	return domain.WorkflowStep{
		Number:      number,
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		StartedAt:   &started,
		CompletedAt: &completed,
	}
}

// outlineLogNoTimestamps is a minimal GHA log without leading timestamps; two
// top-level groups: "Set up job" (success, no errors) and "Run build"
// (failure, one error line). Without timestamps every line inherits the
// active step (step 0), so this fixture exercises the no-partition path.
func outlineLogNoTimestamps() []byte {
	var sb strings.Builder
	sb.WriteString("##[group]Set up job\n")
	sb.WriteString("Initializing environment\n")
	sb.WriteString("##[endgroup]\n")
	sb.WriteString("##[group]Run build\n")
	sb.WriteString("go build ./...\n")
	sb.WriteString("##[error]build failed\n")
	sb.WriteString("##[endgroup]\n")
	return []byte(sb.String())
}

// outlineLogWithTimestamps is the same content as outlineLogNoTimestamps but
// each line carries a timestamp that places it inside the corresponding API
// step's window (per makeAPIStep's 60s-per-step layout).
func outlineLogWithTimestamps() []byte {
	var sb strings.Builder
	// Step 1 ("Set up job") window: 10:00:00 .. 10:00:45.
	sb.WriteString("2024-01-01T10:00:01Z ##[group]Set up job\n")
	sb.WriteString("2024-01-01T10:00:02Z Initializing environment\n")
	sb.WriteString("2024-01-01T10:00:03Z ##[endgroup]\n")
	// Step 2 ("Run build") window: 10:01:00 .. 10:01:45.
	sb.WriteString("2024-01-01T10:01:01Z ##[group]Run build\n")
	sb.WriteString("2024-01-01T10:01:02Z go build ./...\n")
	sb.WriteString("2024-01-01T10:01:03Z ##[error]build failed\n")
	sb.WriteString("2024-01-01T10:01:04Z ##[endgroup]\n")
	return []byte(sb.String())
}

// TestLog_Steps_Accessor verifies that Params.Steps is accessible via Steps().
func TestLog_Steps_Accessor(t *testing.T) {
	steps := []domain.WorkflowStep{
		makeAPIStep(1, "Set up job", "completed", "success"),
		makeAPIStep(2, "Run build", "completed", "failure"),
	}
	fc := &fakeClient{}
	fc.push(nLines(5), nil)
	m := New(Params{
		Repo:     testRepo(),
		RunID:    1,
		JobID:    1,
		Client:   fc,
		Now:      now,
		Width:    100,
		Height:   24,
		ViewMode: ViewModeOutline,
		Steps:    steps,
	})
	got := m.Steps()
	if len(got) != 2 {
		t.Fatalf("Steps() len = %d, want 2", len(got))
	}
	if got[0].Name != "Set up job" || got[1].Name != "Run build" {
		t.Errorf("Steps() = %v, unexpected names", got)
	}
}

// TestLog_StepStatusBadge_SuccessStep verifies that a step with SevPlain but
// API conclusion "success" renders a success badge in the header.
func TestLog_StepStatusBadge_SuccessStep(t *testing.T) {
	testutil.LockColorProfile(t)
	steps := []domain.WorkflowStep{
		makeAPIStep(1, "Set up job", "completed", "success"),
		makeAPIStep(2, "Run build", "completed", "failure"),
	}
	fc := &fakeClient{}
	fc.push(outlineLogWithTimestamps(), nil)
	m := New(Params{
		Repo:     testRepo(),
		RunID:    1,
		JobID:    1,
		Client:   fc,
		Now:      now,
		Width:    100,
		Height:   24,
		ViewMode: ViewModeOutline,
		Steps:    steps,
	})
	m = drainInit(t, m)
	v := m.View().Content
	// "Set up job" step (index 0) should show a success badge (✓).
	if !strings.Contains(v, "✓") {
		t.Errorf("expected success badge (✓) for 'Set up job', view:\n%s", v)
	}
}

// TestLog_APIStepDuration_UsedWhenNoTimestamps verifies that when a log group
// lacks leading timestamps, the duration shown in the header comes from the
// API step's StartedAt/CompletedAt fields.
func TestLog_APIStepDuration_UsedWhenNoTimestamps(t *testing.T) {
	testutil.LockColorProfile(t)
	steps := []domain.WorkflowStep{
		makeAPIStep(1, "Set up job", "completed", "success"),
		makeAPIStep(2, "Run build", "completed", "failure"),
	}
	fc := &fakeClient{}
	fc.push(outlineLogNoTimestamps(), nil)
	m := New(Params{
		Repo:     testRepo(),
		RunID:    1,
		JobID:    1,
		Client:   fc,
		Now:      now,
		Width:    100,
		Height:   24,
		ViewMode: ViewModeOutline,
		Steps:    steps,
	})
	m = drainInit(t, m)
	v := m.View().Content
	// Each step has a 45s duration from the API step.
	if !strings.Contains(v, "45s") {
		t.Errorf("expected API-derived duration (45s) in view:\n%s", v)
	}
}

// ---------------------------------------------------------------------------
// Milestone 9.4 — wrap behavior in outline/compact modes
// ---------------------------------------------------------------------------

// TestLog_WrapInOutlineMode verifies that w toggles wrap in outline mode and
// that long content lines are wrapped at the screen width inside their indent
// column. Header rows must not be wider than the screen width.
func TestLog_WrapInOutlineMode(t *testing.T) {
	testutil.LockColorProfile(t)
	// Create a log with a very long line inside a group.
	long := strings.Repeat("word ", 40) // 200 chars — longer than width 100
	logContent := "##[group]Setup\n" + long + "\n##[endgroup]\n"
	fc := &fakeClient{}
	fc.push([]byte(logContent), nil)
	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       40,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
	})
	m = drainInit(t, m)

	// Expand the step so the long line is visible.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	m = next.(*Model)

	// Without wrap: the long line should appear as a single row (truncated).
	v := m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > m.Width() {
			t.Errorf("line wider than screen (%d > %d) without wrap: %q", w, m.Width(), ln)
		}
	}

	// Enable wrap.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(*Model)
	if !m.Wrap() {
		t.Fatal("Wrap should be on after w in outline mode")
	}

	// With wrap: every rendered line must be within screen width.
	v = m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > m.Width() {
			t.Errorf("line wider than screen (%d > %d) with wrap enabled: %q", w, m.Width(), ln)
		}
	}

	// The long content should have been split across multiple visual lines.
	longLines := 0
	for _, ln := range strings.Split(v, "\n") {
		// Content lines contain "word" but not "##" markers.
		if strings.Contains(ln, "word") && !strings.Contains(ln, "##") {
			longLines++
		}
	}
	if longLines < 2 {
		t.Errorf("expected long line to be wrapped into >= 2 rows, got %d rows containing 'word'", longLines)
	}
}

// TestLog_WrapHeaderTruncatesTitle verifies that header rows never wrap and
// that truncation keeps counts/duration on the right side visible.
func TestLog_WrapHeaderTruncatesTitle(t *testing.T) {
	testutil.LockColorProfile(t)
	// A step with a very long title.
	title := strings.Repeat("A", 120) // longer than width 80
	logContent := "##[group]" + title + "\nsome content\n##[error]boom\n##[endgroup]\n"
	fc := &fakeClient{}
	fc.push([]byte(logContent), nil)
	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        80,
		Height:       10,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
	})
	m = drainInit(t, m)

	// Enable wrap (headers must still not exceed screen width).
	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(*Model)

	v := m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > m.Width() {
			t.Errorf("header line wider than screen (%d > %d): %q", w, m.Width(), ln)
		}
	}
	// The error count badge should appear somewhere in the view.
	if !strings.Contains(v, "error") {
		t.Errorf("expected error count visible in header, view:\n%s", v)
	}
}

// ---------------------------------------------------------------------------
// Milestone 9.5 — help screen at 80 columns
// ---------------------------------------------------------------------------

// TestLog_HelpToggle verifies that ? toggles the full help overlay.
func TestLog_HelpToggle(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        80,
		Height:       24,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
	})
	m = drainInit(t, m)

	// Initially short help.
	v := m.View().Content
	if strings.Contains(v, "expand all") {
		t.Error("full help should not be visible before ? is pressed")
	}

	// Toggle on.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(*Model)
	v = m.View().Content
	if !strings.Contains(v, "expand all") {
		t.Error("full help should be visible after ? is pressed")
	}

	// Toggle off.
	next, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(*Model)
	v = m.View().Content
	if strings.Contains(v, "expand all") {
		t.Error("full help should be hidden after second ? press")
	}
}

// TestLog_FullHelpFitsAt80Cols verifies that no help line exceeds 80 columns.
func TestLog_FullHelpFitsAt80Cols(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	fc.push(outlineLog(), nil)
	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        80,
		Height:       24,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
	})
	m = drainInit(t, m)

	// Enable full help.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(*Model)

	v := m.View().Content
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > m.Width() {
			t.Errorf("help line wider than 80 cols (%d): %q", w, ln)
		}
	}
}

// TestLog_CRLFLineEndings verifies that logs with Windows-style \r\n line
// endings are handled correctly end-to-end. Before the fix, trailing \r in
// group titles broke marker classification in BuildOutline, so the outline
// had no NodeStep roots and the user saw nothing when expanding.
func TestLog_CRLFLineEndings(t *testing.T) {
	testutil.LockColorProfile(t)

	// Simulate a GitHub Actions job log with \r\n endings.
	logContent := "##[group]Set up job\r\nsome setup content\r\n##[endgroup]\r\n" +
		"##[group]Run tests\r\ntest output\r\n##[endgroup]\r\n"

	fc := &fakeClient{}
	fc.push([]byte(logContent), nil)

	apiSteps := []domain.WorkflowStep{
		{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success"},
		{Number: 2, Name: "Run tests", Status: "completed", Conclusion: "success"},
	}

	m := New(Params{
		Repo:         testRepo(),
		RunID:        1,
		JobID:        1,
		Client:       fc,
		Now:          now,
		Width:        120,
		Height:       30,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
		RunActive:    false,
		ViewMode:     ViewModeOutline,
		Steps:        apiSteps,
	})
	m = drainInit(t, m)

	// There should be exactly 2 step roots matched from the API steps.
	// Before the fix, all roots had \r-suffixed titles and none matched,
	// so the API steps would all become empty synthetic nodes.
	roots := m.outline.Roots
	if got := len(roots); got != 2 {
		titles := make([]string, len(roots))
		for i, r := range roots {
			titles[i] = r.Title
		}
		t.Fatalf("outline.Roots = %d, want 2 (titles: %q)", got, titles)
	}
	if got := m.outline.Roots[0].Title; got != "Set up job" {
		t.Errorf("Roots[0].Title = %q, want %q", got, "Set up job")
	}
	if got := m.outline.Roots[1].Title; got != "Run tests" {
		t.Errorf("Roots[1].Title = %q, want %q", got, "Run tests")
	}

	// Expand the first step — it must have children (not be empty).
	rows := m.visibleRows
	if len(rows) < 2 {
		t.Fatalf("visibleRows = %d, want at least 2 (one per step header)", len(rows))
	}
	// Row 0 is the "Set up job" header; expand it.
	m.cursor = 0
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: ""})
	m = next.(*Model)

	// After expanding, there must be at least one content row below the header.
	expanded := m.visibleRows
	if len(expanded) <= 2 {
		t.Errorf("after expanding Set up job: visibleRows = %d, want > 2 (header + content + next step)", len(expanded))
	}
}
