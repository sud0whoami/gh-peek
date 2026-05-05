package run

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

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/testutil"
)

// fakeClient is a file-local fake implementing githubapi.ActionsClient.
// Only GetRun and ListJobs are exercised; the other methods return zero.
type fakeClient struct {
	mu sync.Mutex

	runResults []domain.WorkflowRun
	runErrs    []error
	runCalls   int

	jobsResults [][]domain.WorkflowJob
	jobsErrs    []error
	jobsCalls   int
}

func (f *fakeClient) pushRun(r domain.WorkflowRun, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runResults = append(f.runResults, r)
	f.runErrs = append(f.runErrs, err)
}

func (f *fakeClient) pushJobs(j []domain.WorkflowJob, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobsResults = append(f.jobsResults, j)
	f.jobsErrs = append(f.jobsErrs, err)
}

func (f *fakeClient) ListRuns(context.Context, domain.RepoRef, githubapi.ListRunsFilter) (githubapi.ListRunsResult, error) {
	return githubapi.ListRunsResult{}, nil
}

func (f *fakeClient) GetRun(_ context.Context, _ domain.RepoRef, _ int64) (domain.WorkflowRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runCalls++
	if len(f.runResults) == 0 {
		return domain.WorkflowRun{}, nil
	}
	r, e := f.runResults[0], f.runErrs[0]
	f.runResults = f.runResults[1:]
	f.runErrs = f.runErrs[1:]
	return r, e
}

func (f *fakeClient) ListJobs(_ context.Context, _ domain.RepoRef, _ int64) ([]domain.WorkflowJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobsCalls++
	if len(f.jobsResults) == 0 {
		return nil, nil
	}
	j, e := f.jobsResults[0], f.jobsErrs[0]
	f.jobsResults = f.jobsResults[1:]
	f.jobsErrs = f.jobsErrs[1:]
	return j, e
}

func (f *fakeClient) DownloadJobLog(context.Context, domain.RepoRef, int64) ([]byte, error) {
	return nil, nil
}

func (f *fakeClient) runCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runCalls
}

func (f *fakeClient) jobsCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobsCalls
}

var fixedNow = time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

func testRepo() domain.RepoRef {
	return domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"}
}

func makeRun(id int64, status, conclusion string) domain.WorkflowRun {
	started := fixedNow.Add(-3 * time.Minute)
	return domain.WorkflowRun{
		ID:           id,
		Name:         "CI",
		WorkflowName: "CI",
		DisplayTitle: "Add feature",
		Event:        "push",
		HeadBranch:   "main",
		HeadSHA:      "feedbeef",
		Status:       status,
		Conclusion:   conclusion,
		Attempt:      1,
		CreatedAt:    fixedNow.Add(-5 * time.Minute),
		StartedAt:    &started,
		UpdatedAt:    fixedNow.Add(-1 * time.Minute),
		URL:          "https://github.com/octo/demo/actions/runs/777",
	}
}

func makeStep(n int, name, status, conclusion string) domain.WorkflowStep {
	started := fixedNow.Add(-2 * time.Minute)
	completed := fixedNow.Add(-1 * time.Minute)
	return domain.WorkflowStep{
		Number:      n,
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		StartedAt:   &started,
		CompletedAt: &completed,
	}
}

func makeJob(id int64, name, status, conclusion string, steps []domain.WorkflowStep) domain.WorkflowJob {
	started := fixedNow.Add(-3 * time.Minute)
	completed := fixedNow.Add(-30 * time.Second)
	j := domain.WorkflowJob{
		ID:          id,
		RunID:       777,
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		StartedAt:   &started,
		CompletedAt: &completed,
		Steps:       steps,
		URL:         "https://github.com/octo/demo/actions/runs/777/jobs/" + itoa(id),
	}
	return j
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

func newModel(t *testing.T, fc *fakeClient) *Model {
	t.Helper()
	return New(Params{
		Repo:         testRepo(),
		RunID:        777,
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		AutoRefresh:  true,
		TickInterval: time.Millisecond,
	})
}

// drainInit runs Init and feeds back any cmds until both load messages
// have been delivered (or no more cmds remain).
func drainInit(t *testing.T, m *Model) *Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	// Flatten by issuing the commands as a slice.
	cmds := []tea.Cmd{cmd}
	for len(cmds) > 0 {
		c := cmds[0]
		cmds = cmds[1:]
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		// Bubble Tea v2 batch messages: unpack.
		if bm, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range bm {
				cmds = append(cmds, sub)
			}
			continue
		}
		next, nc := m.Update(msg)
		m = next.(*Model)
		if nc != nil {
			cmds = append(cmds, nc)
		}
		// Stop once initial load is complete; otherwise the
		// auto-refresh tick keeps the queue non-empty.
		if !m.IsLoading() {
			break
		}
	}
	return m
}

func TestInitialLoadReady(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(1, "build", "completed", "success", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))

	if m.IsLoading() {
		t.Fatal("expected ready state after both loads")
	}
	v := m.View().Content
	if !strings.Contains(v, "Add feature") {
		t.Errorf("view missing run title: %q", v)
	}
	if !strings.Contains(v, "build") {
		t.Errorf("view missing job name: %q", v)
	}
}

func TestInitialLoadOnlyOneArrives(t *testing.T) {
	fc := &fakeClient{}
	m := newModel(t, fc)
	// Don't run Init's commands; deliver our own.
	_ = m.Init()
	next, _ := m.Update(RunLoadedMsg{Run: makeRun(777, "completed", "success"), Err: nil})
	m = next.(*Model)
	if !m.IsLoading() {
		t.Fatal("expected still loading after only run arrived")
	}
	next, _ = m.Update(JobsLoadedMsg{Jobs: []domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, Err: nil})
	m = next.(*Model)
	if m.IsLoading() {
		t.Fatal("expected ready after both arrived")
	}
}

func TestInitialLoadError(t *testing.T) {
	fc := &fakeClient{}
	m := newModel(t, fc)
	_ = m.Init()
	next, _ := m.Update(RunLoadedMsg{Err: errors.New("boom")})
	m = next.(*Model)
	v := m.View().Content
	if !strings.Contains(v, "boom") {
		t.Errorf("view missing error text: %q", v)
	}
}

func TestSentinelErrorRendering(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		fc := &fakeClient{}
		m := newModel(t, fc)
		_ = m.Init()
		next, _ := m.Update(RunLoadedMsg{Err: githubapi.ErrUnauthorized})
		m = next.(*Model)
		v := m.View().Content
		if !strings.Contains(v, "gh auth login") {
			t.Errorf("expected unauthorized hint, got %q", v)
		}
	})
	t.Run("not_found", func(t *testing.T) {
		fc := &fakeClient{}
		m := newModel(t, fc)
		_ = m.Init()
		next, _ := m.Update(RunLoadedMsg{Err: githubapi.ErrNotFound})
		m = next.(*Model)
		v := m.View().Content
		if !strings.Contains(v, "Run not found") {
			t.Errorf("expected not-found hint, got %q", v)
		}
	})
	t.Run("rate_limited", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()
		c := githubapi.New(githubapi.WithBaseURL(srv.URL))
		_, apiErr := c.ListRuns(context.Background(), testRepo(), githubapi.ListRunsFilter{})
		if !errors.Is(apiErr, githubapi.ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited, got %v", apiErr)
		}
		fc := &fakeClient{}
		m := newModel(t, fc)
		_ = m.Init()
		next, _ := m.Update(RunLoadedMsg{Err: apiErr})
		m = next.(*Model)
		v := m.View().Content
		if !strings.Contains(v, "Rate limited") {
			t.Errorf("expected rate-limited hint, got %q", v)
		}
	})
}

func TestJobSelection(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(1, "build", "completed", "success", nil),
		makeJob(2, "test", "completed", "success", nil),
		makeJob(3, "lint", "completed", "success", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	for _, k := range []string{"j", "j", "k"} {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(*Model)
	}
	if got := m.JobIndex(); got != 1 {
		t.Errorf("JobIndex = %d, want 1", got)
	}
}

func TestTabFocus(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(1, "build", "completed", "success", []domain.WorkflowStep{
			makeStep(1, "Set up", "completed", "success"),
		}),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	if !m.FocusJobs() {
		t.Fatal("default focus should be jobs")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	if m.FocusJobs() {
		t.Fatal("expected steps focus after tab")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	if !m.FocusJobs() {
		t.Fatal("expected jobs focus after second tab")
	}
}

func TestStepSelectionResetsOnJobChange(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	steps := []domain.WorkflowStep{
		makeStep(1, "Set up", "completed", "success"),
		makeStep(2, "Build", "completed", "success"),
		makeStep(3, "Upload", "completed", "success"),
	}
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(1, "build", "completed", "success", steps),
		makeJob(2, "test", "completed", "success", steps),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	// Tab to steps, move down twice.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	for _, k := range []string{"j", "j"} {
		next, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(*Model)
	}
	if got := m.StepIndex(); got != 2 {
		t.Errorf("StepIndex = %d, want 2", got)
	}
	// Tab back to jobs, move job cursor; step cursor must reset.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	if got := m.StepIndex(); got != 0 {
		t.Errorf("StepIndex after job change = %d, want 0", got)
	}
	// Tab back to steps; still 0.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	if got := m.StepIndex(); got != 0 {
		t.Errorf("StepIndex after tab back = %d, want 0", got)
	}
}

func TestEnterEmitsOpenJobLogMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "success", nil),
		makeJob(22, "test", "completed", "success", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter returned nil cmd")
	}
	msg := cmd()
	op, ok := msg.(OpenJobLogMsg)
	if !ok {
		t.Fatalf("expected OpenJobLogMsg, got %T", msg)
	}
	if op.JobID != 22 || op.RunID != 777 || op.Repo.Owner != "octo" {
		t.Errorf("unexpected OpenJobLogMsg: %+v", op)
	}
}

func TestEnterEmitsOpenJobLogMsg_ForwardsJobName(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "success", nil),
		makeJob(22, "deploy", "completed", "success", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	op := cmd().(OpenJobLogMsg)
	if op.JobName != "deploy" {
		t.Errorf("OpenJobLogMsg.JobName = %q, want %q", op.JobName, "deploy")
	}
}

func TestEnterEmitsOpenJobLogMsg_RunActiveTrueWhenInProgress(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "in_progress", "", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	op := cmd().(OpenJobLogMsg)
	if !op.RunActive {
		t.Errorf("OpenJobLogMsg.RunActive = false, want true for in_progress run")
	}
}

func TestEnterEmitsOpenJobLogMsg_RunActiveFalseWhenCompleted(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "success", nil),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	op := cmd().(OpenJobLogMsg)
	if op.RunActive {
		t.Errorf("OpenJobLogMsg.RunActive = true, want false for completed run")
	}
}

func TestEnterEmitsOpenJobLogMsg_ForwardsSteps(t *testing.T) {
	steps := []domain.WorkflowStep{
		makeStep(1, "Set up job", "completed", "success"),
		makeStep(2, "Run build", "completed", "failure"),
	}
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "failure"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "failure", steps),
	}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	op := cmd().(OpenJobLogMsg)
	if len(op.Steps) != 2 {
		t.Fatalf("OpenJobLogMsg.Steps len = %d, want 2", len(op.Steps))
	}
	if op.Steps[0].Name != "Set up job" || op.Steps[1].Name != "Run build" {
		t.Errorf("OpenJobLogMsg.Steps = %v, unexpected names", op.Steps)
	}
}

func TestOpenInBrowser(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "success", []domain.WorkflowStep{makeStep(1, "Set up", "completed", "success")}),
	}, nil)
	m := drainInit(t, newModel(t, fc))

	// Jobs pane focused → job URL.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatal("o returned nil cmd (jobs)")
	}
	op, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if !strings.Contains(op.URL, "/jobs/11") {
		t.Errorf("expected job URL, got %q", op.URL)
	}

	// Tab to steps; o emits run URL.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	m = next.(*Model)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	op2, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg (steps), got %T", cmd())
	}
	if !strings.HasSuffix(op2.URL, "/runs/777") {
		t.Errorf("expected run URL, got %q", op2.URL)
	}
}

func TestEscEmitsBackMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if cmd == nil {
		t.Fatal("esc returned nil cmd")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Fatalf("expected BackMsg, got %T", cmd())
	}
}

func TestBKeyEmitsBackMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil {
		t.Fatal("b returned nil cmd")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Fatalf("expected BackMsg, got %T", cmd())
	}
}

func TestRRefetches(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	beforeRun := fc.runCallCount()
	beforeJobs := fc.jobsCallCount()

	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	// Drain cmd: it should be a Batch of two fetches.
	msg := cmd()
	if bm, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range bm {
			if sub != nil {
				_ = sub()
			}
		}
	}
	if fc.runCallCount() != beforeRun+1 {
		t.Errorf("runCalls = %d, want %d", fc.runCallCount(), beforeRun+1)
	}
	if fc.jobsCallCount() != beforeJobs+1 {
		t.Errorf("jobsCalls = %d, want %d", fc.jobsCallCount(), beforeJobs+1)
	}
}

func TestAutoRefreshToggle(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "in_progress", "", nil)}, nil)
	m := drainInit(t, newModel(t, fc))

	// Toggle OFF.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	if m.AutoRefreshOn() {
		t.Fatal("expected auto-refresh off after R")
	}
	beforeRun := fc.runCallCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		_ = cmd()
	}
	if fc.runCallCount() != beforeRun {
		t.Errorf("tick fetched while auto-refresh off: %d -> %d", beforeRun, fc.runCallCount())
	}

	// Toggle ON; tick should fetch (run is in_progress).
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(*Model)
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "in_progress", "", nil)}, nil)
	beforeRun = fc.runCallCount()
	_, cmd = m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("tick returned nil cmd")
	}
	if bm, ok := cmd().(tea.BatchMsg); ok {
		for _, sub := range bm {
			if sub != nil {
				_ = sub()
			}
		}
	}
	if fc.runCallCount() != beforeRun+1 {
		t.Errorf("expected fetch after toggle-on tick: %d -> %d", beforeRun, fc.runCallCount())
	}
}

func TestTickSkippedWhenCompleted(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	beforeRun := fc.runCallCount()
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		_ = cmd()
	}
	if fc.runCallCount() != beforeRun {
		t.Errorf("tick fetched on completed run: %d -> %d", beforeRun, fc.runCallCount())
	}
}

func TestTickFiresWhenActive(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "in_progress", "", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "in_progress", "", nil)}, nil)
	beforeRun := fc.runCallCount()
	beforeJobs := fc.jobsCallCount()
	_, cmd := m.Update(TickMsg{})
	if cmd == nil {
		t.Fatal("tick returned nil cmd")
	}
	if bm, ok := cmd().(tea.BatchMsg); ok {
		for _, sub := range bm {
			if sub != nil {
				_ = sub()
			}
		}
	}
	if fc.runCallCount() != beforeRun+1 {
		t.Errorf("expected GetRun fetch on tick: %d -> %d", beforeRun, fc.runCallCount())
	}
	if fc.jobsCallCount() != beforeJobs+1 {
		t.Errorf("expected ListJobs fetch on tick: %d -> %d", beforeJobs, fc.jobsCallCount())
	}
}

func TestWindowResizeLayoutSwitch(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(1, "build", "completed", "success", []domain.WorkflowStep{
			makeStep(1, "Set up", "completed", "success"),
		}),
	}, nil)
	m := drainInit(t, newModel(t, fc))

	// Stacked.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = next.(*Model)
	stacked := m.View().Content
	if !strings.Contains(stacked, "build") || !strings.Contains(stacked, "Set up") {
		t.Errorf("stacked view missing content: %q", stacked)
	}

	// Side-by-side.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = next.(*Model)
	side := m.View().Content
	if !strings.Contains(side, "build") || !strings.Contains(side, "Set up") {
		t.Errorf("side-by-side view missing content: %q", side)
	}
}

func TestQuitOnQ(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q returned nil cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestRefreshErrorKeepsPriorData(t *testing.T) {
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "completed", "success"), nil)
	fc.pushJobs([]domain.WorkflowJob{makeJob(1, "build", "completed", "success", nil)}, nil)
	m := drainInit(t, newModel(t, fc))

	fc.pushRun(domain.WorkflowRun{}, errors.New("network down"))
	fc.pushJobs(nil, errors.New("network down"))

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	if bm, ok := cmd().(tea.BatchMsg); ok {
		for _, sub := range bm {
			if sub != nil {
				next, _ := m.Update(sub())
				m = next.(*Model)
			}
		}
	}
	if !m.HasRefreshErr() {
		t.Error("expected refresh error banner")
	}
	v := m.View().Content
	if !strings.Contains(v, "build") {
		t.Errorf("expected job name preserved, got %q", v)
	}
	if !strings.Contains(v, "network down") {
		t.Errorf("expected error in banner, got %q", v)
	}
}

// TestGoldenRunReady locks color profile and asserts a stable rendering.
//
//	UPDATE_GOLDEN=1 go test ./internal/ui/screens/run/... -run TestGoldenRunReady -count=1
func TestGoldenRunReady(t *testing.T) {
	testutil.LockColorProfile(t)
	fc := &fakeClient{}
	fc.pushRun(makeRun(777, "in_progress", ""), nil)
	fc.pushJobs([]domain.WorkflowJob{
		makeJob(11, "build", "completed", "success", []domain.WorkflowStep{
			makeStep(1, "Checkout", "completed", "success"),
			makeStep(2, "Compile", "completed", "success"),
			makeStep(3, "Upload artifact", "completed", "skipped"),
		}),
		makeJob(22, "deploy", "in_progress", "", []domain.WorkflowStep{
			makeStep(1, "Authenticate", "completed", "success"),
			makeStep(2, "Push image", "in_progress", ""),
			makeStep(3, "Notify", "queued", ""),
		}),
	}, nil)
	m := New(Params{
		Repo:        testRepo(),
		RunID:       777,
		Client:      fc,
		Now:         func() time.Time { return fixedNow },
		Width:       100,
		Height:      24,
		AutoRefresh: true,
	})
	m = drainInit(t, m)
	rendered := m.View().Content
	normalized := testutil.NormalizeTimestamps(rendered)
	testutil.AssertGolden(t, "run_ready", normalized)
}
