package pkgscreen

import (
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// state is the screen's loading/data state for the versions fetch.
type state int

const (
	stateLoading state = iota
	stateReady
	stateError
)

// Params holds the configuration for New.
type Params struct {
	Repo      domain.RepoRef
	PackageID int64
	// Initial may be provided by the list screen so the header renders
	// immediately without an extra fetch.
	Initial      domain.Package
	Client       githubapi.PackagesClient
	Now          func() time.Time
	Width        int
	Height       int
	AutoRefresh  bool
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the package detail screen.
type Model struct {
	params Params
	keys   keymap.PackageDetail
	theme  styles.Theme
	width  int
	height int

	state    state
	pkg      domain.Package
	versions []domain.PackageVersion

	cursor int

	autoRefresh   bool
	tickInterval  time.Duration
	lastETag      string
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
		keys:         keymap.DefaultPackageDetail(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		pkg:          p.Initial,
		state:        stateLoading,
	}
}

type tickMsg struct{}

// versionsLoadedMsg carries the ListPackageVersions result.
type versionsLoadedMsg struct {
	Result githubapi.ListPackageVersionsResult
	Err    error
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}

// BackMsg asks the parent to return to the packages list.
type BackMsg struct{}
