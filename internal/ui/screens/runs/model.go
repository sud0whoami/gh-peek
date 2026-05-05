// Package runs implements the workflow run list screen.
//
// Scope (Milestone 3):
//   - Initial fetch driven by StartupContext (All / Branch / PR).
//   - Client-side active-only and substring search filters.
//   - 7-second auto-refresh tick with ETag-based conditional GETs.
//   - Sentinel-error rendering (auth, not-found, rate-limited).
//   - Emits OpenRunMsg / OpenInBrowserMsg for the parent to route.
//
// Deferred:
//   - The `b` (cycle branch / PR / all) keybinding is intentionally
//     not wired in M3; it will return in a later milestone once the
//     parent owns view-state cycling.
package runs

import (
	"time"

	textinput "charm.land/bubbles/v2/textinput"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// State enumerates the high-level view modes of the runs screen.
type State int

// State values.
const (
	StateLoading State = iota
	StateReady
	StateEmpty
	StateError
)

// viewMode tracks which slice of runs the screen is currently
// requesting from the API. It is bound by `b` cycling through the
// applicable modes given the StartupContext.
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

// Params bundles the dependencies required to construct a Model.
type Params struct {
	Startup     domain.StartupContext
	Client      githubapi.ActionsClient
	Now         func() time.Time
	Width       int
	Height      int
	AutoRefresh bool
	// TickInterval overrides the auto-refresh polling cadence.
	// Zero means "use the default" (7s).
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

// initialViewMode picks the starting view mode from the StartupContext.
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

// tickMsg is the internal auto-refresh tick.
type tickMsg struct{}

// runsLoadedMsg is dispatched when a fetch completes (success or error).
type runsLoadedMsg struct {
	Result githubapi.ListRunsResult
	Err    error
}

// OpenRunMsg requests that the parent open the run-detail screen.
type OpenRunMsg struct {
	RunID int64
	Repo  domain.RepoRef
}

// OpenInBrowserMsg requests that the parent open the given URL.
type OpenInBrowserMsg struct {
	URL string
}
