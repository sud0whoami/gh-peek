package release

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
	focusNotes focusKind = iota
	focusAssets
)

// Params holds the configuration for New.
type Params struct {
	Repo      domain.RepoRef
	ReleaseID int64
	// Initial may be provided by the list screen so the detail screen
	// can render immediately without an extra fetch.
	Initial      domain.Release
	Client       githubapi.ReleasesClient
	Now          func() time.Time
	Width        int
	Height       int
	AutoRefresh  bool
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the release-detail screen.
type Model struct {
	params Params
	keys   keymap.ReleaseDetail
	theme  styles.Theme
	width  int
	height int

	state state
	rel   domain.Release

	notesScroll int
	assetCursor int
	focus       focusKind

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
	m := &Model{
		params:       p,
		keys:         keymap.DefaultReleaseDetail(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		focus:        focusNotes,
	}
	if p.Initial.ID != 0 {
		m.rel = p.Initial
		m.state = stateReady
	} else {
		m.state = stateLoading
	}
	return m
}

type tickMsg struct{}

// releaseLoadedMsg carries the GetRelease result.
type releaseLoadedMsg struct {
	Release domain.Release
	Err     error
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}

// BackMsg asks the parent to return to the releases list.
type BackMsg struct{}
