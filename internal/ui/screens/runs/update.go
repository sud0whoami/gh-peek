package runs

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// defaultTickInterval is the default auto-refresh polling cadence.
const defaultTickInterval = 7 * time.Second

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

	case runsLoadedMsg:
		return m.handleLoaded(msg), m.scheduleTick()

	case tickMsg:
		return m.handleTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleLoaded(msg runsLoadedMsg) *Model {
	m.loading = false
	if msg.Err != nil {
		// Refresh failure with prior data → keep state, set banner.
		if len(m.runs) > 0 {
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
	m.runs = msg.Result.Runs
	m.lastETag = msg.Result.ETag
	m.lastRefreshed = m.params.Now()
	if m.cursor >= len(m.visible()) {
		m.cursor = 0
	}
	if len(m.runs) == 0 {
		m.state = StateEmpty
	} else {
		m.state = StateReady
	}
	return m
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh || m.input.Focused() || !m.hasActiveRuns() {
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
		// Keep cursor in range as the visible set shrinks/grows.
		if v := m.visible(); m.cursor >= len(v) {
			m.cursor = 0
		}
		return m, cmd
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
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
		run := v[m.cursor]
		repo := m.params.Startup.Repo.Repo
		return m, func() tea.Msg { return OpenRunMsg{RunID: run.ID, Repo: repo} }
	case "o":
		v := m.visible()
		if len(v) == 0 {
			return m, nil
		}
		url := v[m.cursor].URL
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
	case "a":
		m.activeOnly = !m.activeOnly
		m.cursor = 0
		return m, nil
	case "b":
		if !m.cycleViewMode() {
			return m, nil
		}
		m.lastETag = ""
		m.cursor = 0
		m.loading = true
		return m, m.fetchCmd()
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	}
	return m, nil
}

// fetchCmd returns a Cmd that performs a single ListRuns call and
// emits the result as a runsLoadedMsg. The current ETag is sent as
// If-None-Match.
func (m *Model) fetchCmd() tea.Cmd {
	f := m.currentFilter()
	f.IfNoneMatch = m.lastETag
	repo := m.params.Startup.Repo.Repo
	client := m.params.Client
	return func() tea.Msg {
		r, err := client.ListRuns(context.Background(), repo, f)
		return runsLoadedMsg{Result: r, Err: err}
	}
}

// currentFilter builds a ListRunsFilter from the current view mode.
func (m *Model) currentFilter() githubapi.ListRunsFilter {
	f := githubapi.ListRunsFilter{}
	switch m.viewMode {
	case viewModeBranch:
		f.Branch = m.params.Startup.Repo.CurrentBranch
	case viewModePR:
		if m.params.Startup.PR != nil {
			f.HeadSHA = m.params.Startup.PR.HeadRefOID
		}
	}
	return f
}

// cycleViewMode advances m.viewMode to the next applicable mode given
// the StartupContext. Modes that are not applicable (no PR, no branch)
// are skipped. Returns true if the mode actually changed.
func (m *Model) cycleViewMode() bool {
	sc := m.params.Startup
	hasPR := sc.PR != nil && sc.PR.HeadRefOID != ""
	hasBranch := sc.Repo.CurrentBranch != ""
	order := []viewMode{viewModeBranch, viewModePR, viewModeAll}
	applicable := order[:0]
	for _, v := range order {
		switch v {
		case viewModeBranch:
			if hasBranch {
				applicable = append(applicable, v)
			}
		case viewModePR:
			if hasPR {
				applicable = append(applicable, v)
			}
		case viewModeAll:
			applicable = append(applicable, v)
		}
	}
	if len(applicable) < 2 {
		return false
	}
	cur := 0
	for i, v := range applicable {
		if v == m.viewMode {
			cur = i
			break
		}
	}
	next := applicable[(cur+1)%len(applicable)]
	if next == m.viewMode {
		return false
	}
	m.viewMode = next
	return true
}

// scheduleTick returns a tickCmd if auto-refresh would fire, else nil.
// Called at the end of every fetch result; the goal is to keep exactly
// one tick in flight.
func (m *Model) scheduleTick() tea.Cmd {
	if !m.autoRefresh {
		return nil
	}
	if !m.hasActiveRuns() {
		return nil
	}
	return m.tickCmd()
}

// tickCmd schedules a single tickMsg after the model's configured interval.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// hasActiveRuns reports whether any loaded run is in a Pending or
// Running semantic state.
func (m *Model) hasActiveRuns() bool {
	for _, r := range m.runs {
		s := githubapi.MapAPIStatus(r.Status, r.Conclusion)
		if s == domain.StatusPending || s == domain.StatusRunning {
			return true
		}
	}
	return false
}

// visible returns the rows after applying the active-only and search
// filters.
func (m *Model) visible() []domain.WorkflowRun {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	out := make([]domain.WorkflowRun, 0, len(m.runs))
	for _, r := range m.runs {
		if m.activeOnly {
			s := githubapi.MapAPIStatus(r.Status, r.Conclusion)
			if s != domain.StatusPending && s != domain.StatusRunning {
				continue
			}
		}
		if q != "" {
			hay := strings.ToLower(r.WorkflowName + " " + r.DisplayTitle + " " + r.HeadBranch)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
