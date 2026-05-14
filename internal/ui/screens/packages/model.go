package packages

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
	Repo         domain.RepoRef
	Client       githubapi.PackagesClient
	Now          func() time.Time
	Width        int
	Height       int
	AutoRefresh  bool
	TickInterval time.Duration
}

// Model is the Bubble Tea model for the packages list screen.
type Model struct {
	params Params
	keys   keymap.Packages
	theme  styles.Theme
	width  int
	height int

	state         State
	packages      []domain.Package
	cursor        int
	input         textinput.Model
	showHelp      bool
	autoRefresh   bool
	tickInterval  time.Duration
	lastETags     map[domain.PackageType]string
	lastRefreshed time.Time
	loading       bool
	loadErr       error
	refreshErr    error
}

// New returns a Model ready to fetch packages.
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
	in.Placeholder = "filter packages"
	in.CharLimit = 128
	interval := p.TickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}
	return &Model{
		params:       p,
		keys:         keymap.DefaultPackages(),
		theme:        styles.DefaultTheme(),
		width:        p.Width,
		height:       p.Height,
		state:        StateLoading,
		autoRefresh:  p.AutoRefresh,
		tickInterval: interval,
		input:        in,
		loading:      true,
		lastETags:    map[domain.PackageType]string{},
	}
}

type tickMsg struct{}

// packagesLoadedMsg carries a fetch result.
type packagesLoadedMsg struct {
	Result githubapi.ListPackagesResult
	Err    error
}

// OpenPackageMsg asks the parent to open package detail.
type OpenPackageMsg struct {
	PackageID int64
	Repo      domain.RepoRef
	Package   domain.Package
}

// OpenInBrowserMsg asks the parent to open a URL in the browser.
type OpenInBrowserMsg struct {
	URL string
}

// BackToRunsMsg asks the parent to return to the runs screen.
type BackToRunsMsg struct{}
