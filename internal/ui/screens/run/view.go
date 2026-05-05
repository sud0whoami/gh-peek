package run

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

// View implements tea.Model.
func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteByte('\n')
	b.WriteString(m.renderSubheader())
	b.WriteByte('\n')
	if m.refreshErr != nil && m.runLoaded && m.jobsLoaded {
		b.WriteString(m.theme.ErrorBanner(m.truncate(
			"! refresh failed: " + errorHint(m.refreshErr) + " (press r to retry)")))
		b.WriteByte('\n')
	}
	b.WriteString(m.renderBody())
	b.WriteByte('\n')
	b.WriteString(m.renderFooter())
	return tea.NewView(b.String())
}

func (m *Model) renderHeader() string {
	title := m.run.DisplayTitle
	if title == "" {
		title = m.run.Name
	}
	left := m.theme.Header(fmt.Sprintf("Run #%d · %s", m.params.RunID, title))
	return m.layoutLR(left, m.renderStatusIndicator())
}

func (m *Model) renderStatusIndicator() string {
	switch {
	case m.state == stateLoading:
		return m.theme.Muted("↻")
	case !m.autoRefresh:
		return m.theme.Muted("⏼ off")
	case !m.lastRefreshed.IsZero():
		d := m.params.Now().Sub(m.lastRefreshed)
		return m.theme.Muted("✓ " + humanizeAgo(d))
	default:
		return m.theme.Muted("✓")
	}
}

func (m *Model) renderSubheader() string {
	if !m.runLoaded || m.state == stateError {
		return m.theme.Muted(m.truncate(""))
	}
	parts := []string{
		m.run.WorkflowName,
		m.run.HeadBranch,
		m.run.Event,
		m.theme.Badge(githubapi.MapAPIStatus(m.run.Status, m.run.Conclusion)),
	}
	if m.run.StartedAt != nil {
		parts = append(parts, "started "+humanizeAgo(m.params.Now().Sub(*m.run.StartedAt)))
	}
	parts = append(parts, "duration "+humanizeDuration(m.runDuration()))
	line := strings.Join(parts, " · ")
	if s := githubapi.MapAPIStatus(m.run.Status, m.run.Conclusion); s == domain.StatusPending || s == domain.StatusRunning {
		active := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("● ACTIVE")
		line += " " + active
	}
	return m.truncate(line)
}

func (m *Model) runDuration() time.Duration {
	if m.run.StartedAt == nil {
		return 0
	}
	end := m.params.Now()
	if !m.run.UpdatedAt.IsZero() && (m.run.Status == "completed") {
		end = m.run.UpdatedAt
	}
	return end.Sub(*m.run.StartedAt)
}

func (m *Model) renderBody() string {
	switch m.state {
	case stateLoading:
		return m.theme.Muted("Loading run…")
	case stateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	}
	// ready
	if m.width >= 100 {
		return m.renderTwoPane()
	}
	return m.renderStacked()
}

func (m *Model) renderTwoPane() string {
	jobsW := m.width * 4 / 10
	if jobsW < 30 {
		jobsW = 30
	}
	if jobsW > m.width-20 {
		jobsW = m.width - 20
	}
	stepsW := m.width - jobsW - 1
	if stepsW < 10 {
		stepsW = 10
	}
	jobsPane := m.renderJobsPane(jobsW, len(m.jobs))
	stepsPane := m.renderStepsPane(stepsW)
	return lipgloss.JoinHorizontal(lipgloss.Top, jobsPane, " ", stepsPane)
}

func (m *Model) renderStacked() string {
	rows := len(m.jobs)
	if rows > 8 {
		rows = 8
	}
	jobs := m.renderJobsPane(m.width, rows)
	divider := m.theme.Muted(strings.Repeat("─", m.width))
	steps := m.renderStepsPane(m.width)
	return lipgloss.JoinVertical(lipgloss.Left, jobs, divider, steps)
}

func (m *Model) renderJobsPane(width, max int) string {
	if max <= 0 || len(m.jobs) == 0 {
		return padRight(m.theme.Muted("No jobs."), width)
	}
	if max > len(m.jobs) {
		max = len(m.jobs)
	}
	var b strings.Builder
	b.WriteString(padRight(m.theme.Muted(truncRune("JOBS", width)), width))
	b.WriteByte('\n')
	for i := 0; i < max; i++ {
		j := m.jobs[i]
		s := githubapi.MapAPIStatus(j.Status, j.Conclusion)
		dur := humanizeDuration(jobDuration(j, m.params.Now()))
		prefix := "  "
		if i == m.jobCursor && m.focus == focusJobs {
			prefix = "▶ "
		}
		row := prefix + m.theme.Badge(s) + " " + j.Name + "  " + dur
		row = padRight(truncRune(row, width), width)
		if i == m.jobCursor {
			if m.focus == focusJobs {
				row = m.theme.Selected(row)
			} else {
				row = m.theme.Muted(row)
			}
		}
		b.WriteString(row)
		if i < max-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *Model) renderStepsPane(width int) string {
	steps := m.currentSteps()
	var b strings.Builder
	header := "STEPS"
	if m.jobCursor >= 0 && m.jobCursor < len(m.jobs) {
		header = "STEPS · " + m.jobs[m.jobCursor].Name
	}
	b.WriteString(padRight(m.theme.Muted(truncRune(header, width)), width))
	b.WriteByte('\n')
	if len(steps) == 0 {
		b.WriteString(padRight(m.theme.Muted(truncRune("(no steps)", width)), width))
		return b.String()
	}
	for i, s := range steps {
		sem := githubapi.MapAPIStatus(s.Status, s.Conclusion)
		dur := humanizeDuration(stepDuration(s, m.params.Now()))
		prefix := "  "
		if i == m.stepCursor && m.focus == focusSteps {
			prefix = "▶ "
		}
		row := prefix + strconv.Itoa(s.Number) + ". " + m.theme.Badge(sem) + " " + s.Name + "  " + dur
		row = padRight(truncRune(row, width), width)
		if i == m.stepCursor {
			if m.focus == focusSteps {
				row = m.theme.Selected(row)
			} else {
				row = m.theme.Muted(row)
			}
		}
		b.WriteString(row)
		if i < len(steps)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *Model) renderFooter() string {
	if m.showHelp {
		return m.theme.Help(m.truncate(fullHelpText(m)))
	}
	return m.theme.Help(m.truncate(shortHelpText(m)))
}

func shortHelpText(m *Model) string {
	bindings := m.keys.ShortHelp()
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, "  ·  ")
}

func fullHelpText(m *Model) string {
	rows := m.keys.FullHelp()
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		parts := make([]string, 0, len(r))
		for _, b := range r {
			h := b.Help()
			parts = append(parts, h.Key+" "+h.Desc)
		}
		lines = append(lines, strings.Join(parts, "  ·  "))
	}
	return strings.Join(lines, " | ")
}

// truncate clips a string to the model's width using cell widths.
func (m *Model) truncate(s string) string {
	if m.width <= 0 {
		return s
	}
	if lipgloss.Width(s) <= m.width {
		return s
	}
	return truncRune(s, m.width)
}

// layoutLR places left and right on the same line padded to width.
func (m *Model) layoutLR(left, right string) string {
	w := m.width
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	if lw+rw+1 > w {
		if lw > w {
			return truncRune(left, w)
		}
		return left
	}
	gap := w - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// errorHint maps known sentinels to user-facing hints.
func errorHint(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, githubapi.ErrUnauthorized):
		return "Not authorized. Run `gh auth login`."
	case errors.Is(err, githubapi.ErrRateLimited):
		var ae *githubapi.APIError
		if errors.As(err, &ae) && ae.RetryAfter > 0 {
			return "Rate limited; retry after " + ae.RetryAfter.String() + "."
		}
		return "Rate limited; retry shortly."
	case errors.Is(err, githubapi.ErrNotFound):
		return "Run not found or no longer accessible."
	case errors.Is(err, githubapi.ErrForbidden):
		return "Forbidden — your token may lack the required scopes."
	default:
		return err.Error()
	}
}

// humanizeAgo renders a duration as e.g. "5s ago" / "3m ago".
func humanizeAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// humanizeDuration renders an elapsed duration as "Xm Ys" / "Ys".
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	mm := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh %dm", h, mm)
}

func jobDuration(j domain.WorkflowJob, now time.Time) time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	end := now
	if j.CompletedAt != nil {
		end = *j.CompletedAt
	}
	return end.Sub(*j.StartedAt)
}

func stepDuration(s domain.WorkflowStep, now time.Time) time.Duration {
	if s.StartedAt == nil {
		return 0
	}
	end := now
	if s.CompletedAt != nil {
		end = *s.CompletedAt
	}
	return end.Sub(*s.StartedAt)
}

// truncRune truncates s to n display columns, appending "…" if cut.
func truncRune(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > n-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteRune('…')
	return b.String()
}

// padRight pads s with spaces on the right to n display columns.
func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
