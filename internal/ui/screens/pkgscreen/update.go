package pkgscreen

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// defaultTickInterval is the default refresh cadence for package detail.
const defaultTickInterval = 60 * time.Second

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.fetchCmd()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case versionsLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()
	case tickMsg:
		return m.handleTick()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg versionsLoadedMsg) *Model {
	if msg.Err != nil {
		if m.state == stateLoading && len(m.versions) == 0 {
			m.loadErr = msg.Err
			m.state = stateError
		} else {
			m.refreshErr = msg.Err
		}
		return m
	}
	m.refreshErr = nil
	m.lastRefreshed = m.params.Now()
	if msg.Result.NotModified {
		if msg.Result.ETag != "" {
			m.lastETag = msg.Result.ETag
		}
		m.state = stateReady
		return m
	}
	m.versions = msg.Result.Versions
	if msg.Result.ETag != "" {
		m.lastETag = msg.Result.ETag
	}
	if m.cursor >= len(m.versions) {
		m.cursor = 0
	}
	m.state = stateReady
	return m
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh {
		return m, m.tickCmd()
	}
	return m, m.fetchCmd()
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "b":
		return m, func() tea.Msg { return BackMsg{} }
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.versions)-1 {
			m.cursor++
		}
		return m, nil
	case "pgup":
		m.cursor -= 10
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil
	case "pgdown":
		m.cursor += 10
		if m.cursor > len(m.versions)-1 {
			m.cursor = len(m.versions) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil
	case "o":
		if len(m.versions) == 0 {
			return m, nil
		}
		url := m.versions[m.cursor].PackageHTMLURL
		if url == "" {
			return m, nil
		}
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "O":
		if m.pkg.URL == "" {
			return m, nil
		}
		url := m.pkg.URL
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "r":
		return m, m.fetchCmd()
	case "R":
		m.autoRefresh = !m.autoRefresh
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	}
	return m, nil
}

// fetchCmd issues a single ListPackageVersions call.
func (m *Model) fetchCmd() tea.Cmd {
	repo := m.params.Repo
	pkg := m.pkg
	client := m.params.Client
	f := githubapi.ListPackageVersionsFilter{IfNoneMatch: m.lastETag, PerPage: 50}
	return func() tea.Msg {
		r, err := client.ListPackageVersions(context.Background(), repo, pkg, f)
		return versionsLoadedMsg{Result: r, Err: err}
	}
}

func (m *Model) scheduleTick() tea.Cmd {
	if !m.autoRefresh {
		return nil
	}
	return m.tickCmd()
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}
