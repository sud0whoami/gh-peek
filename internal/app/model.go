package app

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/browser"
	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	logscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/log"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/packages"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/pkgscreen"
	releasescreen "github.com/sud0whoami/gh-peek/internal/ui/screens/release"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/releases"
	runscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/run"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/runs"
)

// activeScreen identifies which child screen is currently visible.
type activeScreen int

const (
	activeNone activeScreen = iota
	activeRuns
	activeRunDetail
	activeLogViewer
	activeReleasesList
	activeReleaseDetail
	activePackagesList
	activePackageDetail
)

// RootParams holds dependencies for NewRouter.
type RootParams struct {
	Startup domain.StartupContext
	Client  githubapi.ActionsClient
	// ReleasesClient is optional; if nil, the same concrete client passed
	// as Client is used when it also implements ReleasesClient.
	ReleasesClient githubapi.ReleasesClient
	// PackagesClient is optional; if nil, the same concrete Client is used
	// when it implements PackagesClient.
	PackagesClient githubapi.PackagesClient
	Now            func() time.Time
	Width          int
	Height         int
	AutoRefresh    bool
	// TickInterval overrides the 7s auto-refresh cadence in child screens.
	TickInterval time.Duration
	// BrowserOpener opens URLs. Defaults to browser.OSOpener{}.
	BrowserOpener browser.Opener
}

// Model is the root Bubble Tea model for gh-peek.
// New() returns a minimal placeholder; NewRouter() wires up the full screen stack.
type Model struct {
	params              *RootParams
	width, height       int
	runsScreen          *runs.Model
	detailScreen        *runscreen.Model
	logScreen           *logscreen.Model
	releasesScreen      *releases.Model
	releaseDetailScreen *releasescreen.Model
	packagesScreen      *packages.Model
	packageDetailScreen *pkgscreen.Model
	active              activeScreen
	browserOpener       browser.Opener
}

// New returns a minimal placeholder model used in tests.
func New() *Model {
	return &Model{}
}

// NewRouter returns a Model wired to the runs-list screen.
func NewRouter(p RootParams) *Model {
	if p.Now == nil {
		p.Now = time.Now
	}
	if p.BrowserOpener == nil {
		p.BrowserOpener = browser.OSOpener{}
	}
	rs := runs.New(runs.Params{
		Startup:      p.Startup,
		Client:       p.Client,
		Now:          p.Now,
		Width:        p.Width,
		Height:       p.Height,
		AutoRefresh:  p.AutoRefresh,
		TickInterval: p.TickInterval,
	})
	return &Model{
		params:        &p,
		width:         p.Width,
		height:        p.Height,
		runsScreen:    rs,
		active:        activeRuns,
		browserOpener: p.BrowserOpener,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	if m.active == activeRuns && m.runsScreen != nil {
		return m.runsScreen.Init()
	}
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Global quit keys.
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	// 2. Window size: remember and propagate to active child.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		return m.delegate(ws)
	}

	// 3. Intercept navigation messages from child screens before delegation.
	switch msg := msg.(type) {
	case runs.OpenRunMsg:
		if m.params == nil {
			return m, nil
		}
		detail := runscreen.New(runscreen.Params{
			Repo:         msg.Repo,
			RunID:        msg.RunID,
			Client:       m.params.Client,
			Now:          m.params.Now,
			Width:        m.width,
			Height:       m.height,
			AutoRefresh:  true,
			TickInterval: m.params.TickInterval,
		})
		m.detailScreen = detail
		m.active = activeRunDetail
		return m, detail.Init()

	case runs.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case runscreen.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case runscreen.OpenJobLogMsg:
		if m.params == nil {
			return m, nil
		}
		lv := logscreen.New(logscreen.Params{
			Repo:         msg.Repo,
			RunID:        msg.RunID,
			JobID:        msg.JobID,
			JobName:      msg.JobName,
			Client:       m.params.Client,
			Now:          m.params.Now,
			Width:        m.width,
			Height:       m.height,
			AutoRefresh:  true,
			TickInterval: m.params.TickInterval,
			RunActive:    msg.RunActive,
			Steps:        msg.Steps,
		})
		m.logScreen = lv
		m.active = activeLogViewer
		return m, lv.Init()

	case logscreen.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case logscreen.BackMsg:
		m.logScreen = nil
		if m.detailScreen != nil {
			m.active = activeRunDetail
		} else {
			m.active = activeRuns
		}
		return m, nil

	case runscreen.BackMsg:
		m.detailScreen = nil
		m.active = activeRuns
		return m, nil

	case runs.OpenReleasesMsg:
		if m.params == nil {
			return m, nil
		}
		rc := m.releasesClient()
		if rc == nil {
			return m, nil
		}
		if m.releasesScreen == nil {
			m.releasesScreen = releases.New(releases.Params{
				Repo:         m.params.Startup.Repo.Repo,
				Client:       rc,
				Now:          m.params.Now,
				Width:        m.width,
				Height:       m.height,
				AutoRefresh:  m.params.AutoRefresh,
				TickInterval: m.params.TickInterval,
			})
			m.active = activeReleasesList
			return m, m.releasesScreen.Init()
		}
		m.active = activeReleasesList
		return m, nil

	case releases.OpenReleaseMsg:
		if m.params == nil {
			return m, nil
		}
		rc := m.releasesClient()
		if rc == nil {
			return m, nil
		}
		detail := releasescreen.New(releasescreen.Params{
			Repo:         msg.Repo,
			ReleaseID:    msg.ReleaseID,
			Initial:      msg.Release,
			Client:       rc,
			Now:          m.params.Now,
			Width:        m.width,
			Height:       m.height,
			AutoRefresh:  m.params.AutoRefresh,
			TickInterval: m.params.TickInterval,
		})
		m.releaseDetailScreen = detail
		m.active = activeReleaseDetail
		return m, detail.Init()

	case releases.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case releases.BackToRunsMsg:
		m.active = activeRuns
		return m, nil

	case releasescreen.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case releasescreen.BackMsg:
		m.releaseDetailScreen = nil
		m.active = activeReleasesList
		return m, nil

	case runs.OpenPackagesMsg:
		return m.openPackagesList()

	case releases.OpenPackagesMsg:
		return m.openPackagesList()

	case packages.OpenPackageMsg:
		if m.params == nil {
			return m, nil
		}
		pc := m.packagesClient()
		if pc == nil {
			return m, nil
		}
		detail := pkgscreen.New(pkgscreen.Params{
			Repo:         msg.Repo,
			PackageID:    msg.PackageID,
			Initial:      msg.Package,
			Client:       pc,
			Now:          m.params.Now,
			Width:        m.width,
			Height:       m.height,
			AutoRefresh:  m.params.AutoRefresh,
			TickInterval: m.params.TickInterval,
		})
		m.packageDetailScreen = detail
		m.active = activePackageDetail
		return m, detail.Init()

	case packages.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case packages.BackToRunsMsg:
		m.active = activeRuns
		return m, nil

	case pkgscreen.OpenInBrowserMsg:
		return m, m.openBrowserCmd(msg.URL)

	case pkgscreen.BackMsg:
		m.packageDetailScreen = nil
		m.active = activePackagesList
		return m, nil
	}

	// 4. Delegate to active child.
	return m.delegate(msg)
}

// delegate sends msg to the active child screen.
func (m *Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.active {
	case activeRuns:
		if m.runsScreen != nil {
			updated, cmd := m.runsScreen.Update(msg)
			if rm, ok := updated.(*runs.Model); ok {
				m.runsScreen = rm
			}
			return m, cmd
		}
	case activeRunDetail:
		if m.detailScreen != nil {
			updated, cmd := m.detailScreen.Update(msg)
			if rm, ok := updated.(*runscreen.Model); ok {
				m.detailScreen = rm
			}
			return m, cmd
		}
	case activeLogViewer:
		if m.logScreen != nil {
			updated, cmd := m.logScreen.Update(msg)
			if lm, ok := updated.(*logscreen.Model); ok {
				m.logScreen = lm
			}
			return m, cmd
		}
	case activeReleasesList:
		if m.releasesScreen != nil {
			updated, cmd := m.releasesScreen.Update(msg)
			if rm, ok := updated.(*releases.Model); ok {
				m.releasesScreen = rm
			}
			return m, cmd
		}
	case activeReleaseDetail:
		if m.releaseDetailScreen != nil {
			updated, cmd := m.releaseDetailScreen.Update(msg)
			if rm, ok := updated.(*releasescreen.Model); ok {
				m.releaseDetailScreen = rm
			}
			return m, cmd
		}
	case activePackagesList:
		if m.packagesScreen != nil {
			updated, cmd := m.packagesScreen.Update(msg)
			if pm, ok := updated.(*packages.Model); ok {
				m.packagesScreen = pm
			}
			return m, cmd
		}
	case activePackageDetail:
		if m.packageDetailScreen != nil {
			updated, cmd := m.packageDetailScreen.Update(msg)
			if pm, ok := updated.(*pkgscreen.Model); ok {
				m.packageDetailScreen = pm
			}
			return m, cmd
		}
	}
	return m, nil
}

// openBrowserCmd returns a tea.Cmd that opens the URL in the user's
// browser. Failures are logged via slog and not surfaced in the UI.
func (m *Model) openBrowserCmd(url string) tea.Cmd {
	opener := m.browserOpener
	if opener == nil {
		opener = browser.OSOpener{}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := opener.Open(ctx, url); err != nil {
			slog.Warn("browser open failed", "url", url, "err", err)
		}
		return nil
	}
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	switch m.active {
	case activeRuns:
		if m.runsScreen != nil {
			return m.runsScreen.View()
		}
	case activeRunDetail:
		if m.detailScreen != nil {
			return m.detailScreen.View()
		}
	case activeLogViewer:
		if m.logScreen != nil {
			return m.logScreen.View()
		}
	case activeReleasesList:
		if m.releasesScreen != nil {
			return m.releasesScreen.View()
		}
	case activeReleaseDetail:
		if m.releaseDetailScreen != nil {
			return m.releaseDetailScreen.View()
		}
	case activePackagesList:
		if m.packagesScreen != nil {
			return m.packagesScreen.View()
		}
	case activePackageDetail:
		if m.packageDetailScreen != nil {
			return m.packageDetailScreen.View()
		}
	}
	return tea.NewView("gh-peek — initializing…")
}

// releasesClient returns the configured ReleasesClient, falling back to
// the Client if it also implements ReleasesClient.
func (m *Model) releasesClient() githubapi.ReleasesClient {
	if m.params == nil {
		return nil
	}
	if m.params.ReleasesClient != nil {
		return m.params.ReleasesClient
	}
	if rc, ok := m.params.Client.(githubapi.ReleasesClient); ok {
		return rc
	}
	return nil
}

// packagesClient returns the configured PackagesClient, falling back to
// the Client if it also implements PackagesClient.
func (m *Model) packagesClient() githubapi.PackagesClient {
	if m.params == nil {
		return nil
	}
	if m.params.PackagesClient != nil {
		return m.params.PackagesClient
	}
	if pc, ok := m.params.Client.(githubapi.PackagesClient); ok {
		return pc
	}
	return nil
}

// openPackagesList lazily constructs (or restores) the packages list
// screen and switches to it.
func (m *Model) openPackagesList() (tea.Model, tea.Cmd) {
	if m.params == nil {
		return m, nil
	}
	pc := m.packagesClient()
	if pc == nil {
		return m, nil
	}
	if m.packagesScreen == nil {
		m.packagesScreen = packages.New(packages.Params{
			Repo:         m.params.Startup.Repo.Repo,
			Client:       pc,
			Now:          m.params.Now,
			Width:        m.width,
			Height:       m.height,
			AutoRefresh:  m.params.AutoRefresh,
			TickInterval: m.params.TickInterval,
		})
		m.active = activePackagesList
		return m, m.packagesScreen.Init()
	}
	m.active = activePackagesList
	return m, nil
}
