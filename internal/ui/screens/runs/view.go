package runs

import (
	"errors"
	"fmt"
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
	b.WriteString(m.renderFilterLine())
	b.WriteByte('\n')
	if m.input.Focused() {
		b.WriteString(m.truncate(m.input.View()))
		b.WriteByte('\n')
	}
	if m.refreshErr != nil {
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
	sc := m.params.Startup
	repo := sc.Repo.Repo
	var title string
	switch m.viewMode {
	case viewModePR:
		if sc.PR != nil {
			title = fmt.Sprintf("PR #%d · %s", sc.PR.Number, sc.PR.Title)
		} else {
			title = "PR runs"
		}
	case viewModeBranch:
		title = "Branch runs · " + sc.Repo.CurrentBranch
	default:
		title = "All runs · " + repo.Owner + "/" + repo.Name
	}
	return m.theme.HeaderBar(title, m.statusIndicatorText(), m.width)
}

func (m *Model) statusIndicatorText() string {
	switch {
	case m.loading:
		return "↻"
	case m.input.Focused():
		return "⏸"
	case !m.autoRefresh:
		return "⏼ off"
	case !m.lastRefreshed.IsZero():
		d := m.params.Now().Sub(m.lastRefreshed)
		return "✓ " + humanizeAgo(d)
	default:
		return "✓"
	}
}

func (m *Model) renderFilterLine() string {
	parts := []string{}
	sc := m.params.Startup
	switch m.viewMode {
	case viewModeBranch:
		parts = append(parts, "branch:"+sc.Repo.CurrentBranch)
	case viewModePR:
		if sc.PR != nil {
			parts = append(parts, fmt.Sprintf("pr:%d", sc.PR.Number))
		}
	}
	if m.activeOnly {
		parts = append(parts, "active-only")
	}
	if v := strings.TrimSpace(m.input.Value()); v != "" {
		parts = append(parts, "search:"+v)
	}
	if len(parts) == 0 {
		return m.theme.Muted(m.truncate("(no filter)"))
	}
	return m.theme.Muted(m.truncate(strings.Join(parts, "  ")))
}

func (m *Model) renderBody() string {
	switch m.state {
	case StateLoading:
		return m.theme.Muted("Loading runs…")
	case StateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	case StateEmpty:
		return m.theme.Muted("No runs match the current filter. Press 'r' to refresh.")
	case StateReady:
		v := m.visible()
		if len(v) == 0 {
			return m.theme.Muted("No runs match the current filter. Press 'r' to refresh.")
		}
		return m.renderTable(v)
	}
	return ""
}

// columnWidths computes the per-column widths so the row fits in the
// available width. Status is fixed at the badge width to avoid having
// to truncate ANSI-styled content.
func (m *Model) columnWidths() (wf, title, branch, event, status, updated int) {
	const sepCount = 5
	const statusCol = 14 // "[ ✓ success ]" + 1 trailing pad
	avail := m.width - sepCount
	if avail < 30 {
		avail = 30
	}
	status = statusCol
	rest := avail - status
	event = clampInt(rest/8, 5, 8)
	updated = clampInt(rest/8, 6, 10)
	branch = clampInt(rest/4, 6, 18)
	used := event + updated + branch
	rem := rest - used
	if rem < 8 {
		rem = 8
	}
	wf = rem / 3
	if wf < 4 {
		wf = 4
	}
	title = rem - wf
	if title < 4 {
		title = 4
	}
	return
}

func (m *Model) renderTable(rows []domain.WorkflowRun) string {
	wf, title, branch, event, status, updated := m.columnWidths()
	var b strings.Builder
	header := joinRow(
		padRight(truncRune("WORKFLOW", wf), wf),
		padRight(truncRune("TITLE", title), title),
		padRight(truncRune("BRANCH", branch), branch),
		padRight(truncRune("EVENT", event), event),
		padRight(truncRune("STATUS", status), status),
		padRight(truncRune("UPDATED", updated), updated),
	)
	b.WriteString(m.theme.SectionLabel(m.truncate(header)))
	b.WriteByte('\n')

	for i, r := range rows {
		s := githubapi.MapAPIStatus(r.Status, r.Conclusion)
		statusCell := padToVisible(m.theme.Badge(s), status)
		updatedCell := humanizeAgo(m.params.Now().Sub(r.UpdatedAt))
		row := joinRow(
			padRight(truncRune(r.WorkflowName, wf), wf),
			padRight(truncRune(r.DisplayTitle, title), title),
			padRight(truncRune(r.HeadBranch, branch), branch),
			padRight(truncRune(r.Event, event), event),
			statusCell,
			padRight(truncRune(updatedCell, updated), updated),
		)
		if i == m.cursor {
			row = m.theme.SelectedRow(row, m.width)
		}
		b.WriteString(row)
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *Model) renderFooter() string {
	div := m.theme.Divider(m.width)
	if m.showHelp {
		return div + "\n" + m.theme.Help(m.truncate(fullHelpText(m)))
	}
	return div + "\n" + m.theme.Help(m.truncate(shortHelpText(m)))
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

// truncate clips a string to the model's current width using cell
// widths so ANSI sequences are not counted.
func (m *Model) truncate(s string) string {
	if m.width <= 0 {
		return s
	}
	if lipgloss.Width(s) <= m.width {
		return s
	}
	return truncRune(s, m.width)
}

// errorHint maps known sentinels to user-facing hints.
func errorHint(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, githubapi.ErrUnauthorized):
		return "Unauthorized — Run `gh auth login` to refresh your token."
	case errors.Is(err, githubapi.ErrNotFound):
		return "Repository not found, or no Actions configured for it."
	case errors.Is(err, githubapi.ErrRateLimited):
		var ae *githubapi.APIError
		if errors.As(err, &ae) && ae.RetryAfter > 0 {
			return "Rate limited by GitHub. Retry after " + ae.RetryAfter.String() + "."
		}
		return "Rate limited by GitHub. Try again shortly."
	case errors.Is(err, githubapi.ErrForbidden):
		return "Forbidden — Your token may lack the required scopes."
	default:
		return err.Error()
	}
}

// humanizeAgo renders a duration as e.g. "5s ago" / "3m ago" / "2h ago".
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
	// Walk the runes accumulating width; cut at n-1 then add ellipsis.
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

// padToVisible pads a possibly-styled string to n visible columns.
func padToVisible(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// joinRow joins cells with a single space.
func joinRow(cells ...string) string {
	return strings.Join(cells, " ")
}

func clampInt(want, lo, hi int) int {
	if want < lo {
		want = lo
	}
	if hi > 0 && want > hi {
		want = hi
	}
	return want
}
