// Package run implements the run-detail screen.
package run

import (
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// state is the screen's loading/data state.
type state int

const (
	stateLoading state = iota
	stateReady
	stateError
)

// focusKind identifies the focused pane.
type focusKind int

const (
	focusJobs focusKind = iota
	focusSteps
)

// Params holds the configuration for New.
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

// OpenJobLogMsg asks the parent to open the job log viewer.
type OpenJobLogMsg struct {
	Repo      domain.RepoRef
	RunID     int64
	JobID     int64
	JobName   string
	RunActive bool
	Steps     []domain.WorkflowStep
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}

// BackMsg asks the parent to go back.
type BackMsg struct{}
