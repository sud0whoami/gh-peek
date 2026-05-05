// Package run implements the run-detail screen.
//
// Scope (Milestone 4):
//   - Initial parallel fetch of GetRun + ListJobs.
//   - States: loading | ready | error. Refresh failures keep prior data.
//   - Two-pane (jobs/steps) when width >= 100, stacked otherwise.
//   - Auto-refresh tick (default 7s, injectable) only fires while the
//     run is still active (Pending/Running) and auto-refresh is on.
//   - Emits OpenJobLogMsg / OpenInBrowserMsg / BackMsg for the parent
//     to route. The screen does not navigate itself.
package run

import (
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// state enumerates high-level view modes.
type state int

const (
	stateLoading state = iota
	stateReady
	stateError
)

// focusKind tracks which pane has the active selection cursor.
type focusKind int

const (
	focusJobs focusKind = iota
	focusSteps
)

// Params bundles the dependencies required to construct a Model.
type Params struct {
	Repo         domain.RepoRef
	RunID        int64
	Client       githubapi.ActionsClient
	Now          func() time.Time
	Width        int
	Height       int
	AutoRefresh  bool
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the run-detail screen.
type Model struct {
	params Params
	keys   keymap.RunDetail
	theme  styles.Theme
	width  int
	height int

	state state
	run   domain.WorkflowRun
	jobs  []domain.WorkflowJob

	runLoaded  bool
	jobsLoaded bool

	jobCursor  int
	stepCursor int
	focus      focusKind

	autoRefresh   bool
	tickInterval  time.Duration
	lastRefreshed time.Time
	loadErr       error
	refreshErr    error
	showHelp      bool
}

// New constructs a Model from the given Params.
func New(p Params) *Model {
	if p.Now == nil {
		p.Now = time.Now
	}
	if p.Width <= 0 {
		p.Width = 100
	}
	if p.Height <= 0 {
		p.Height = 24
	}
	interval := p.TickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}
	return &Model{
		params:       p,
		keys:         keymap.DefaultRunDetail(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		state:        stateLoading,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		focus:        focusJobs,
	}
}

// tickMsg is the internal auto-refresh tick.
type tickMsg struct{}

// runLoadedMsg carries the GetRun result.
type runLoadedMsg struct {
	Run domain.WorkflowRun
	Err error
}

// jobsLoadedMsg carries the ListJobs result.
type jobsLoadedMsg struct {
	Jobs []domain.WorkflowJob
	Err  error
}

// OpenJobLogMsg requests that the parent open the job log viewer.
type OpenJobLogMsg struct {
	Repo      domain.RepoRef
	RunID     int64
	JobID     int64
	JobName   string
	RunActive bool
	Steps     []domain.WorkflowStep
}

// OpenInBrowserMsg requests that the parent open the given URL.
type OpenInBrowserMsg struct {
	URL string
}

// BackMsg requests that the parent return to the previous screen.
type BackMsg struct{}
