package run

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// defaultTickInterval is the default auto-refresh polling cadence.
const defaultTickInterval = 7 * time.Second

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.getRunCmd(), m.listJobsCmd())
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case runLoadedMsg:
		return m.handleRunLoaded(msg), m.scheduleTick()

	case jobsLoadedMsg:
		return m.handleJobsLoaded(msg), m.scheduleTick()

	case tickMsg:
		return m.handleTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleRunLoaded(msg runLoadedMsg) *Model {
	if msg.Err != nil {
		// On initial load: surface as a hard error.
		// On refresh: keep prior data, set banner.
		if m.state == stateLoading {
			m.loadErr = msg.Err
			m.state = stateError
		} else {
			m.refreshErr = msg.Err
		}
		m.runLoaded = true
		return m
	}
	m.run = msg.Run
	m.runLoaded = true
	m.refreshErr = nil
	m.lastRefreshed = m.params.Now()
	m.maybeReady()
	return m
}

func (m *Model) handleJobsLoaded(msg jobsLoadedMsg) *Model {
	if msg.Err != nil {
		if m.state == stateLoading {
			m.loadErr = msg.Err
			m.state = stateError
		} else {
			m.refreshErr = msg.Err
		}
		m.jobsLoaded = true
		return m
	}
	m.jobs = msg.Jobs
	m.jobsLoaded = true
	m.refreshErr = nil
	m.lastRefreshed = m.params.Now()
	if m.jobCursor >= len(m.jobs) {
		m.jobCursor = 0
	}
	m.maybeReady()
	return m
}

func (m *Model) maybeReady() {
	if m.state == stateError {
		return
	}
	if m.runLoaded && m.jobsLoaded {
		m.state = stateReady
	}
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.autoRefresh || !m.runIsActive() {
		return m, m.tickCmd()
	}
	return m, tea.Batch(m.getRunCmd(), m.listJobsCmd())
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "b":
		return m, func() tea.Msg { return BackMsg{} }
	case "tab":
		if m.focus == focusJobs {
			m.focus = focusSteps
		} else {
			m.focus = focusJobs
		}
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(+1)
		return m, nil
	case "enter":
		if len(m.jobs) == 0 {
			return m, nil
		}
		job := m.jobs[m.jobCursor]
		repo := m.params.Repo
		runID := m.params.RunID
		jobID := job.ID
		jobName := job.Name
		active := m.runIsActive()
		steps := job.Steps
		return m, func() tea.Msg {
			return OpenJobLogMsg{Repo: repo, RunID: runID, JobID: jobID, JobName: jobName, RunActive: active, Steps: steps}
		}
	case "o":
		var url string
		if m.focus == focusJobs && len(m.jobs) > 0 {
			url = m.jobs[m.jobCursor].URL
		} else {
			url = m.run.URL
		}
		if url == "" {
			return m, nil
		}
		return m, func() tea.Msg { return OpenInBrowserMsg{URL: url} }
	case "r":
		return m, tea.Batch(m.getRunCmd(), m.listJobsCmd())
	case "R":
		m.autoRefresh = !m.autoRefresh
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	}
	return m, nil
}

// moveCursor moves the active pane's cursor by delta. When the job
// cursor changes, the step cursor resets to 0.
func (m *Model) moveCursor(delta int) {
	if m.focus == focusJobs {
		next := m.jobCursor + delta
		if next < 0 || next >= len(m.jobs) {
			return
		}
		if next != m.jobCursor {
			m.jobCursor = next
			m.stepCursor = 0
		}
		return
	}
	steps := m.currentSteps()
	next := m.stepCursor + delta
	if next < 0 || next >= len(steps) {
		return
	}
	m.stepCursor = next
}

func (m *Model) currentSteps() []domain.WorkflowStep {
	if m.jobCursor < 0 || m.jobCursor >= len(m.jobs) {
		return nil
	}
	return m.jobs[m.jobCursor].Steps
}

// getRunCmd returns a Cmd that fetches the run via GetRun.
func (m *Model) getRunCmd() tea.Cmd {
	repo := m.params.Repo
	runID := m.params.RunID
	client := m.params.Client
	return func() tea.Msg {
		r, err := client.GetRun(context.Background(), repo, runID)
		return runLoadedMsg{Run: r, Err: err}
	}
}

// listJobsCmd returns a Cmd that fetches the run's jobs.
func (m *Model) listJobsCmd() tea.Cmd {
	repo := m.params.Repo
	runID := m.params.RunID
	client := m.params.Client
	return func() tea.Msg {
		j, err := client.ListJobs(context.Background(), repo, runID)
		return jobsLoadedMsg{Jobs: j, Err: err}
	}
}

// scheduleTick returns a tickCmd if auto-refresh would fire, else nil.
func (m *Model) scheduleTick() tea.Cmd {
	if !m.autoRefresh {
		return nil
	}
	if !m.runIsActive() {
		return nil
	}
	return m.tickCmd()
}

// tickCmd schedules a single tickMsg after the configured interval.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// runIsActive reports whether the loaded run is in a Pending or
// Running semantic state.
func (m *Model) runIsActive() bool {
	if !m.runLoaded {
		return false
	}
	s := githubapi.MapAPIStatus(m.run.Status, m.run.Conclusion)
	return s == domain.StatusPending || s == domain.StatusRunning
}
