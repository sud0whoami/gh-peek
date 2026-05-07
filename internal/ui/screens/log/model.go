package log

import (
	"time"

	textinput "charm.land/bubbles/v2/textinput"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/logs"
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

// Params holds the configuration for New.
type Params struct {
	Repo          domain.RepoRef
	RunID         int64
	JobID         int64
	JobName       string
	Client        githubapi.ActionsClient
	Now           func() time.Time
	Width, Height int
	AutoRefresh   bool
	TickInterval  time.Duration

	// RunActive tells the screen whether to keep auto-refreshing.
	RunActive bool

	// ViewMode sets the initial view mode (default: ViewModeOutline).
	ViewMode ViewMode

	// Steps from the GitHub API, used to enrich outline headers with status badges and duration.
	Steps []domain.WorkflowStep
}

// Model is the Bubble Tea model for the job log viewer screen.
type Model struct {
	params Params
	keys   keymap.LogViewer
	theme  styles.Theme
	width  int
	height int

	state state
	buf   *logs.Buffer

	// outline is rebuilt from buf on each successful load; nil until then.
	outline *logs.Outline

	// expanded maps stable node-path keys to true (outline/compact only).
	expanded map[string]bool

	// visibleRows is the flattened visible-row slice for outline/compact.
	// nil in ViewModeRaw.
	visibleRows []row

	// cursor is the focused row index into visibleRows (outline/compact only).
	cursor int

	viewMode ViewMode

	// showTimestamps toggles the leading timestamp display in outline/compact.
	showTimestamps bool

	top  int // top-of-viewport index: into buf.Lines() (raw) or visibleRows (outline/compact)
	wrap bool

	input            textinput.Model
	matches          []int
	matchCursor      int // 0-based; -1 when no matches
	autoRefresh      bool
	tickInterval     time.Duration
	runActive        bool
	lastRefreshed    time.Time
	loadErr          error
	refreshErr       error
	truncatedFromAPI bool
	showHelp         bool
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
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "search log"
	in.CharLimit = 128
	return &Model{
		params:       p,
		keys:         keymap.DefaultLogViewer(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		state:        stateLoading,
		buf:          logs.New(),
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		runActive:    p.RunActive,
		viewMode:     p.ViewMode,
		input:        in,
		matchCursor:  -1,
	}
}

// tickMsg is the internal auto-refresh tick.
type tickMsg struct{}

// logLoadedMsg carries the result of a DownloadJobLog call.
type logLoadedMsg struct {
	Bytes []byte
	Err   error
}

// BackMsg requests that the parent return to the previous screen.
type BackMsg struct{}

// OpenInBrowserMsg requests that the parent open the given URL.
type OpenInBrowserMsg struct {
	URL string
}
