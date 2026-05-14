package releases

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

// Params holds the configuration for New.
type Params struct {
	Repo        domain.RepoRef
	Client      githubapi.ReleasesClient
	Now         func() time.Time
	Width       int
	Height      int
	AutoRefresh bool
	// TickInterval overrides the default polling cadence. Zero means default.
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the releases list screen.
type Model struct {
	params Params
	keys   keymap.Releases
	theme  styles.Theme
	width  int
	height int

	state         State
	releases      []domain.Release
	cursor        int
	input         textinput.Model
	showHelp      bool
	autoRefresh   bool
	tickInterval  time.Duration
	lastETag      string
	lastRefreshed time.Time
	loading       bool
	loadErr       error
	refreshErr    error
}

// New returns a Model ready to fetch releases.
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
	in.Placeholder = "filter releases"
	in.CharLimit = 128
	interval := p.TickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}
	return &Model{
		params:       p,
		keys:         keymap.DefaultReleases(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		state:        StateLoading,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		input:        in,
		loading:      true,
	}
}

type tickMsg struct{}

// releasesLoadedMsg carries a fetch result.
type releasesLoadedMsg struct {
	Result githubapi.ListReleasesResult
	Err    error
}

// OpenReleaseMsg asks the parent to open release detail.
type OpenReleaseMsg struct {
	ReleaseID int64
	Repo      domain.RepoRef
	// Release is included so the detail screen can render immediately
	// without an extra fetch when the list payload already has the data.
	Release domain.Release
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}

// BackToRunsMsg asks the parent to return to the runs screen.
type BackToRunsMsg struct{}

// OpenPackagesMsg asks the parent to switch to the packages list screen.
type OpenPackagesMsg struct{}
