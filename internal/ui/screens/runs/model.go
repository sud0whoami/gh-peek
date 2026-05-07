// Package runs implements the workflow run list screen.
package runs

import (
	"time"

	textinput "charm.land/bubbles/v2/textinput"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// State is the screen's loading/data state.
type State int

const (
	StateLoading State = iota
	StateReady
	StateEmpty
	StateError
)

// viewMode controls which subset of runs is shown; cycled by 'b'.
type viewMode int

const (
	viewModeBranch viewMode = iota
	viewModePR
	viewModeAll
)

func (v viewMode) String() string {
	switch v {
	case viewModeBranch:
		return "branch"
	case viewModePR:
		return "pr"
	case viewModeAll:
		return "all"
	default:
		return "?"
	}
}

// Params holds the configuration for New.
type Params struct {
	Startup     domain.StartupContext
	Client      githubapi.ActionsClient
	Now         func() time.Time
	Width       int
	Height      int
	AutoRefresh bool
	// TickInterval overrides the default 7s polling cadence. Zero means default.
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the runs list screen.
type Model struct {
	params Params
	keys   keymap.Runs
	theme  styles.Theme
	width  int
	height int

	state         State
	runs          []domain.WorkflowRun
	cursor        int
	input         textinput.Model
	activeOnly    bool
	showHelp      bool
	autoRefresh   bool
	tickInterval  time.Duration
	lastETag      string
	lastRefreshed time.Time
	loading       bool
	loadErr       error // set on first-load failure
	refreshErr    error // set on refresh failure when prior data exists
	viewMode      viewMode
}

// New returns a Model ready to fetch runs.
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
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "filter runs"
	in.CharLimit = 128
	interval := p.TickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}
	return &Model{
		params:       p,
		keys:         keymap.DefaultRuns(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		state:        StateLoading,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		input:        in,
		loading:      true,
		viewMode:     initialViewMode(p.Startup),
	}
}

func initialViewMode(sc domain.StartupContext) viewMode {
	switch sc.Kind {
	case domain.StartContextPR:
		return viewModePR
	case domain.StartContextBranch:
		return viewModeBranch
	default:
		return viewModeAll
	}
}

type tickMsg struct{}

// runsLoadedMsg carries a fetch result.
type runsLoadedMsg struct {
	Result githubapi.ListRunsResult
	Err    error
}

// OpenRunMsg asks the parent to open run detail.
type OpenRunMsg struct {
	RunID int64
	Repo  domain.RepoRef
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}
