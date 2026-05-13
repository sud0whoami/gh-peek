package releases

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// defaultTickInterval is the default auto-refresh polling cadence for releases.
// Releases change rarely so we poll less frequently than runs.
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

	case releasesLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()

	case tickMsg:
		return m.handleTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg releasesLoadedMsg) *Model {
	m.loading = false
	if msg.Err != nil {
		if len(m.releases) > 0 {
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
		if msg.Result.ETag != "" {
			m.lastETag = msg.Result.ETag
		}
		return m
	}
	m.releases = msg.Result.Releases
	m.lastETag = msg.Result.ETag
	m.lastRefreshed = m.params.Now()
	if m.cursor >= len(m.visible()) {
		m.cursor = 0
	}
	if len(m.releases) == 0 {
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

	// While the search input is focused, route most keys to it.
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
	case "esc", "b", "L":
		return m, func() tea.Msg { return BackToRunsMsg{} }
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
		rel := v[m.cursor]
		repo := m.params.Repo
		return m, func() tea.Msg { return OpenReleaseMsg{ReleaseID: rel.ID, Repo: repo, Release: rel} }
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

// fetchCmd returns a Cmd that performs a single ListReleases call and
// emits the result as a releasesLoadedMsg.
func (m *Model) fetchCmd() tea.Cmd {
	repo := m.params.Repo
	client := m.params.Client
	f := githubapi.ListReleasesFilter{IfNoneMatch: m.lastETag, PerPage: 50}
	return func() tea.Msg {
		r, err := client.ListReleases(context.Background(), repo, f)
		return releasesLoadedMsg{Result: r, Err: err}
	}
}

// scheduleTick returns a tickCmd if auto-refresh is on.
func (m *Model) scheduleTick() tea.Cmd {
	if !m.autoRefresh {
		return nil
	}
	return m.tickCmd()
}

// tickCmd schedules a single tickMsg after the model's configured interval.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// visible returns the rows after applying the search filter.
func (m *Model) visible() []domain.Release {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	out := make([]domain.Release, 0, len(m.releases))
	for _, r := range m.releases {
		if q != "" {
			hay := strings.ToLower(r.TagName + " " + r.Name + " " + r.Author.Login)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// latestID returns the ID of the first non-draft, non-prerelease entry,
// or 0 when none exist. Used to mark a row with the "latest" badge.
func (m *Model) latestID() int64 {
	for _, r := range m.releases {
		if !r.Draft && !r.Prerelease {
			return r.ID
		}
	}
	return 0
}
