package release

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// defaultTickInterval is the default refresh cadence for the detail screen.
const defaultTickInterval = 60 * time.Second

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	// Always fetch once so we get fresh assets/body even if Initial was provided.
	return m.getReleaseCmd()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case releaseLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()

	case tickMsg:
		return m.handleTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg releaseLoadedMsg) *Model {
	if msg.Err != nil {
		if m.state == stateLoading {
			m.loadErr = msg.Err
			m.state = stateError
		} else {
			m.refreshErr = msg.Err
		}
		return m
	}
	m.rel = msg.Release
	m.refreshErr = nil
	m.lastRefreshed = m.params.Now()
	m.state = stateReady
	if m.assetCursor >= len(m.rel.Assets) {
		m.assetCursor = 0
	}
	return m
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh {
		return m, m.tickCmd()
	}
	return m, m.getReleaseCmd()
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "b":
		return m, func() tea.Msg { return BackMsg{} }
	case "tab":
		if m.focus == focusNotes {
			m.focus = focusAssets
		} else {
			m.focus = focusNotes
		}
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(+1)
		return m, nil
	case "pgup":
		m.moveCursor(-10)
		return m, nil
	case "pgdown":
		m.moveCursor(+10)
		return m, nil
	case "enter":
		if m.focus == focusAssets && len(m.rel.Assets) > 0 {
			a := m.rel.Assets[m.assetCursor]
			if a.BrowserURL == "" {
				return m, nil
			}
			url := a.BrowserURL
			return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
		}
		return m, nil
	case "o":
		if m.rel.URL == "" {
			return m, nil
		}
		url := m.rel.URL
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "r":
		return m, m.getReleaseCmd()
	case "R":
		m.autoRefresh = !m.autoRefresh
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if m.focus == focusAssets {
		next := m.assetCursor + delta
		if next < 0 {
			next = 0
		}
		if next > len(m.rel.Assets)-1 {
			next = len(m.rel.Assets) - 1
		}
		if next < 0 {
			next = 0
		}
		m.assetCursor = next
		return
	}
	// Notes scroll
	next := m.notesScroll + delta
	if next < 0 {
		next = 0
	}
	m.notesScroll = next
}

// getReleaseCmd fetches the release via GetRelease.
func (m *Model) getReleaseCmd() tea.Cmd {
	repo := m.params.Repo
	id := m.params.ReleaseID
	client := m.params.Client
	return func() tea.Msg {
		r, err := client.GetRelease(context.Background(), repo, id)
		return releaseLoadedMsg{Release: r, Err: err}
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
