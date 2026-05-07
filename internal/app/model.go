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
)

// RootParams holds dependencies for NewRouter.
type RootParams struct {
	Startup     domain.StartupContext
	Client      githubapi.ActionsClient
	Now         func() time.Time
	Width       int
	Height      int
	AutoRefresh bool
	// TickInterval overrides the 7s auto-refresh cadence in child screens.
	TickInterval time.Duration
	// BrowserOpener opens URLs. Defaults to browser.OSOpener{}.
	BrowserOpener browser.Opener
}

// Model is the root Bubble Tea model for gh-peek.
// New() returns a minimal placeholder; NewRouter() wires up the full screen stack.
type Model struct {
	params        *RootParams
	width, height int
	runsScreen    *runs.Model
	detailScreen  *runscreen.Model
	logScreen     *logscreen.Model
	active        activeScreen
	browserOpener browser.Opener
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
	}
	return tea.NewView("gh-peek — initializing…")
}
