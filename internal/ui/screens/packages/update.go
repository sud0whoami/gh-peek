package packages

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// defaultTickInterval is the default auto-refresh polling cadence.
// Packages change rarely so we poll less frequently than runs.
const defaultTickInterval = 60 * time.Second

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return m.fetchCmd() }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case packagesLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()
	case tickMsg:
		return m.handleTick()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg packagesLoadedMsg) *Model {
	m.loading = false
	if msg.Err != nil {
		if len(m.packages) > 0 {
			m.refreshErr = msg.Err
			return m
		}
		m.loadErr = msg.Err
		m.state = StateError
		return m
	}
	m.refreshErr = nil
	if msg.Result.NotModified {
		m.lastRefreshed = m.params.Now()
		for k, v := range msg.Result.ETags {
			if v != "" {
				m.lastETags[k] = v
			}
		}
		return m
	}
	m.packages = msg.Result.Packages
	for k, v := range msg.Result.ETags {
		if v != "" {
			m.lastETags[k] = v
		}
	}
	m.lastRefreshed = m.params.Now()
	if m.cursor >= len(m.visible()) {
		m.cursor = 0
	}
	if len(m.packages) == 0 {
		m.state = StateEmpty
	} else {
		m.state = StateReady
	}
	return m
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh || m.input.Focused() {
		return m, m.tickCmd()
	}
	return m, m.fetchCmd()
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.input.Focused() {
		switch key {
		case "esc":
			m.input.Reset()
			m.input.Blur()
			m.cursor = 0
			return m, nil
		case "enter":
			m.input.Blur()
			m.cursor = 0
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if v := m.visible(); m.cursor >= len(v) {
			m.cursor = 0
		}
		return m, cmd
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "b", "P", "W":
		return m, func() tea.Msg { return BackToRunsMsg{} }
	case "L":
		return m, func() tea.Msg { return OpenReleasesMsg{} }
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.visible())-1 {
			m.cursor++
		}
		return m, nil
	case "enter":
		v := m.visible()
		if len(v) == 0 {
			return m, nil
		}
		pkg := v[m.cursor]
		repo := m.params.Repo
		return m, func() tea.Msg { return OpenPackageMsg{PackageID: pkg.ID, Repo: repo, Package: pkg} }
	case "o":
		v := m.visible()
		if len(v) == 0 {
			return m, nil
		}
		url := v[m.cursor].URL
		if url == "" {
			return m, nil
		}
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "/":
		cmd := m.input.Focus()
		m.cursor = 0
		return m, cmd
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

// fetchCmd returns a Cmd that performs a single ListPackages call.
func (m *Model) fetchCmd() tea.Cmd {
	repo := m.params.Repo
	client := m.params.Client
	inm := make(map[domain.PackageType]string, len(m.lastETags))
	for k, v := range m.lastETags {
		inm[k] = v
	}
	f := githubapi.ListPackagesFilter{IfNoneMatch: inm, PerPage: 50}
	return func() tea.Msg {
		r, err := client.ListPackages(context.Background(), repo, f)
		return packagesLoadedMsg{Result: r, Err: err}
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

// visible applies the search filter to the package list.
func (m *Model) visible() []domain.Package {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	out := make([]domain.Package, 0, len(m.packages))
	for _, p := range m.packages {
		if q != "" {
			hay := strings.ToLower(p.Name + " " + string(p.Type) + " " + p.Visibility)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}
