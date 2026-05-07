package log

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/logs"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

// View implements tea.Model.
func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteByte('\n')
	b.WriteString(m.renderSubheader())
	b.WriteByte('\n')
	if m.refreshErr != nil && m.state == stateReady {
		b.WriteString(m.theme.ErrorBanner(m.truncate(
			"! refresh failed: " + errorHint(m.refreshErr) + " (press r to retry)")))
		b.WriteByte('\n')
	}
	if m.input.Focused() {
		b.WriteString(m.truncate(m.input.View()))
		b.WriteByte('\n')
	}
	b.WriteString(m.renderBody())
	b.WriteByte('\n')
	b.WriteString(m.renderFooter())
	return tea.NewView(b.String())
}

func (m *Model) renderHeader() string {
	jobName := m.params.JobName
	if jobName == "" {
		jobName = "job " + strconv.FormatInt(m.params.JobID, 10)
	}
	left := m.theme.Header(fmt.Sprintf("Log · %s · job #%d", jobName, m.params.JobID))
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
	if m.state == stateError {
		return m.theme.Muted("")
	}
	lines := m.buf.Lines()
	parts := []string{
		fmt.Sprintf("%d lines", len(lines)),
		humanizeBytes(m.buf.Len()),
	}
	if m.buf.Truncated() {
		parts = append(parts, "⚠ truncated to 10 MB tail")
	}
	if m.truncatedFromAPI {
		parts = append(parts, "⚠ truncated by server (50 MB cap)")
	}
	// View mode label.
	switch m.viewMode {
	case ViewModeOutline:
		parts = append(parts, "mode:outline")
	case ViewModeCompact:
		parts = append(parts, "mode:compact")
	case ViewModeRaw:
		if m.wrap {
			parts = append(parts, "wrap:on")
		} else {
			parts = append(parts, "wrap:off")
		}
	}
	if q := strings.TrimSpace(m.input.Value()); q != "" {
		i := 0
		if m.matchCursor >= 0 {
			i = m.matchCursor + 1
		}
		parts = append(parts, fmt.Sprintf("search %q · %d/%d", q, i, len(m.matches)))
	}
	return m.theme.Muted(m.truncate(strings.Join(parts, " · ")))
}

func (m *Model) renderBody() string {
	switch m.state {
	case stateLoading:
		return m.theme.Muted("Loading log…")
	case stateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	}

	if m.viewMode != ViewModeRaw {
		return m.renderBodyOutline()
	}
	return m.renderBodyRaw()
}

// renderBodyRaw is the flat renderer used in ViewModeRaw.
func (m *Model) renderBodyRaw() string {
	lines := m.buf.Lines()
	if len(lines) == 0 {
		return m.theme.Muted("(empty log)")
	}
	height := m.bodyHeight()
	gutter := len(strconv.Itoa(len(lines)))
	// Width budget for log content after gutter and a single space.
	contentW := m.width - gutter - 1
	if contentW < 1 {
		contentW = 1
	}

	// Build rendered rows: walk logical lines starting at m.top,
	// optionally wrap each, until we fill `height` rows.
	var rows []string
	for i := m.top; i < len(lines) && len(rows) < height; i++ {
		raw := lines[i]
		var subs []string
		if m.wrap {
			subs = strings.Split(lipgloss.Wrap(raw, contentW, ""), "\n")
		} else {
			subs = []string{truncRune(raw, contentW)}
		}
		for j, sub := range subs {
			if len(rows) >= height {
				break
			}
			var num string
			if j == 0 {
				num = padLeft(strconv.Itoa(i+1), gutter)
			} else {
				num = strings.Repeat(" ", gutter)
			}
			row := m.theme.Muted(num) + " " + sub
			// Highlight current match line.
			if m.matchCursor >= 0 && i == m.matches[m.matchCursor] {
				row = m.theme.Selected(padRight(num+" "+sub, m.width))
			}
			rows = append(rows, row)
		}
	}
	return strings.Join(rows, "\n")
}

// renderBodyOutline renders outline or compact mode using m.visibleRows.
func (m *Model) renderBodyOutline() string {
	if len(m.visibleRows) == 0 {
		return m.theme.Muted("(empty log)")
	}
	bufLines := m.buf.Lines()
	height := m.bodyHeight()
	gutter := len(strconv.Itoa(len(bufLines)))

	// Determine the current match line index (for highlighting).
	matchLineIdx := -1
	if m.matchCursor >= 0 && m.matchCursor < len(m.matches) {
		matchLineIdx = m.matches[m.matchCursor]
	}

	var rendered []string
	linesUsed := 0 // visual lines consumed (accounts for wrapped rows)
	for i := m.top; i < len(m.visibleRows) && linesUsed < height; i++ {
		r := m.visibleRows[i]
		isCursor := i == m.cursor
		isMatchLine := !r.IsHeader && r.LineIdx == matchLineIdx

		var line string
		if r.IsHeader {
			line = m.renderOutlineHeader(r, isCursor)
		} else {
			line = m.renderOutlineLine(r, gutter, bufLines, isCursor || isMatchLine)
		}
		linesUsed += strings.Count(line, "\n") + 1
		rendered = append(rendered, line)
	}
	return strings.Join(rendered, "\n")
}

// renderOutlineHeader renders a step/group header row.
//
// Format: <indent><triangle> <sevBadge> <title><spaces><counts><dur>
func (m *Model) renderOutlineHeader(r row, cursor bool) string {
	indent := strings.Repeat("  ", r.Depth)
	triangle := "▸"
	if !r.Collapsed {
		triangle = "▾"
	}

	sevBadge := outlineSevBadge(r.Node.Sev, m.theme)
	title := r.Node.Title

	// For top-level steps with no log-derived badge, fall back to the API step
	// conclusion badge. This shows ✓/⊘ for steps the log severity doesn't cover.
	if sevBadge == "" && r.Depth == 0 && r.Node.Kind == logs.NodeStep {
		if step, ok := m.apiStepByTitle(r.Node.Title); ok {
			sevBadge = apiConclusionBadge(step.Conclusion, m.theme)
		}
	}

	// Right-side: counts and duration.
	counts := outlineCounts(r.Node)
	dur := outlineDuration(r.Node)
	// For top-level steps, fall back to the API step's duration when the log
	// lacks timestamps.
	if dur == "" && r.Depth == 0 && r.Node.Kind == logs.NodeStep {
		if step, ok := m.apiStepByTitle(r.Node.Title); ok {
			dur = apiStepDuration(step)
		}
	}
	right := ""
	if counts != "" && dur != "" {
		right = counts + "  " + dur
	} else if counts != "" {
		right = counts
	} else if dur != "" {
		right = dur
	}

	// Assemble left part.
	var left string
	if sevBadge != "" {
		left = indent + triangle + " " + sevBadge + " " + title
	} else {
		left = indent + triangle + " " + title
	}

	// Total width budget.
	w := m.width
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	var line string
	if right == "" || lw+rw+2 > w {
		// No room for right side; just truncate left.
		line = truncRune(left, w)
	} else {
		gap := w - lw - rw
		if gap < 1 {
			gap = 1
		}
		line = left + strings.Repeat(" ", gap) + right
	}

	if cursor {
		return m.theme.Selected(padRight(line, w))
	}
	return line
}

// renderOutlineLine renders a log-content line row.
//
// Format: <indent><gutter line#> <sevBadge> <content>
// When m.wrap is true, long content is wrapped; continuation lines use a
// blank gutter so the line number only appears on the first sub-line.
func (m *Model) renderOutlineLine(r row, gutter int, bufLines []string, highlight bool) string {
	indent := strings.Repeat("  ", r.Depth)
	num := m.theme.Muted(padLeft(strconv.Itoa(r.LineIdx+1), gutter))

	var rawLine string
	if r.LineIdx >= 0 && r.LineIdx < len(bufLines) {
		rawLine = bufLines[r.LineIdx]
	}
	content := rawLine
	if !m.showTimestamps {
		content = stripLogTimestamp(rawLine)
	}

	sevBadge := outlineSevBadge(r.Node.Sev, m.theme)

	// Width budget for content after the fixed prefix (indent + gutter + space).
	prefixW := lipgloss.Width(indent) + gutter + 1 // indent + num + space
	if sevBadge != "" {
		prefixW += lipgloss.Width(sevBadge) + 1
	}
	contentW := m.width - prefixW
	if contentW < 1 {
		contentW = 1
	}

	buildFirstLine := func(c string) string {
		if sevBadge != "" {
			return indent + num + " " + sevBadge + " " + c
		}
		return indent + num + " " + c
	}

	if !m.wrap {
		assembled := buildFirstLine(truncRune(content, contentW))
		if highlight {
			return m.theme.Selected(padRight(assembled, m.width))
		}
		return assembled
	}

	// Wrap mode: split content into sub-lines at contentW.
	wrapped := lipgloss.Wrap(content, contentW, "")
	subLines := strings.Split(wrapped, "\n")
	contIndent := strings.Repeat(" ", lipgloss.Width(indent)+gutter+1)
	if sevBadge != "" {
		contIndent += strings.Repeat(" ", lipgloss.Width(sevBadge)+1)
	}
	var parts []string
	for j, sl := range subLines {
		if j == 0 {
			parts = append(parts, buildFirstLine(sl))
		} else {
			parts = append(parts, contIndent+sl)
		}
	}
	assembled := strings.Join(parts, "\n")
	if highlight {
		// Only highlight the first sub-line to avoid over-brightening.
		lines := strings.SplitN(assembled, "\n", 2)
		lines[0] = m.theme.Selected(padRight(lines[0], m.width))
		return strings.Join(lines, "\n")
	}
	return assembled
}

func (m *Model) renderFooter() string {
	if m.showHelp {
		rows := m.keys.FullHelp()
		lines := make([]string, 0, len(rows))
		for _, r := range rows {
			parts := make([]string, 0, len(r))
			for _, b := range r {
				h := b.Help()
				parts = append(parts, h.Key+" "+h.Desc)
			}
			lines = append(lines, m.theme.Help(m.truncate(strings.Join(parts, "  ·  "))))
		}
		return strings.Join(lines, "\n")
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

// ---------------------------------------------------------------------------
// Outline rendering helpers
// ---------------------------------------------------------------------------

// tsStripRe matches the leading ISO-8601 timestamp that GitHub Actions prepends
// to every log line. Used by renderOutlineLine to optionally strip it.
var tsStripRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\s+`)

// stripLogTimestamp removes a leading ISO-8601 timestamp from a log line.
func stripLogTimestamp(line string) string {
	return tsStripRe.ReplaceAllString(line, "")
}

// outlineSevBadge returns a colored single-char badge for the severity level,
// or "" for SevPlain/SevDebug/SevCommand.
func outlineSevBadge(sev logs.Severity, _ interface{ Muted(string) string }) string {
	switch sev {
	case logs.SevError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true).Render("✗")
	case logs.SevWarning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922")).Bold(true).Render("⚠")
	case logs.SevNotice:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff")).Render("ℹ")
	}
	return ""
}

// outlineCounts returns a human-readable count summary such as
// "⚠ 2 errors · 1 warning". Zero counts are omitted. Empty string when all zero.
func outlineCounts(n *logs.Node) string {
	var parts []string
	if n.ErrorCount > 0 {
		s := "error"
		if n.ErrorCount != 1 {
			s = "errors"
		}
		parts = append(parts, fmt.Sprintf("✗ %d %s", n.ErrorCount, s))
	}
	if n.WarningCount > 0 {
		s := "warning"
		if n.WarningCount != 1 {
			s = "warnings"
		}
		parts = append(parts, fmt.Sprintf("⚠ %d %s", n.WarningCount, s))
	}
	if n.NoticeCount > 0 {
		s := "notice"
		if n.NoticeCount != 1 {
			s = "notices"
		}
		parts = append(parts, fmt.Sprintf("ℹ %d %s", n.NoticeCount, s))
	}
	return strings.Join(parts, " · ")
}

// outlineDuration formats a node's execution time as "Xm Ys" or "Xs".
// Returns "" when timestamps are not available.
func outlineDuration(n *logs.Node) string {
	if n.StartTime.IsZero() || n.EndTime.IsZero() {
		return ""
	}
	d := n.EndTime.Sub(n.StartTime)
	return formatDuration(d)
}

// formatDuration renders a duration as "Xm Ys" or "Xs". Returns "" for
// negative or zero durations.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// apiStepByTitle returns the WorkflowStep whose Name matches title.
func (m *Model) apiStepByTitle(title string) (domain.WorkflowStep, bool) {
	for _, s := range m.params.Steps {
		if s.Name == title {
			return s, true
		}
	}
	return domain.WorkflowStep{}, false
}

// apiConclusionBadge returns a colored single-char badge for an API step
// conclusion. Returns "" when no badge is appropriate.
func apiConclusionBadge(conclusion string, theme styles.Theme) string {
	switch conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Render("✓")
	case "skipped", "cancelled":
		return theme.Muted("⊘")
	case "failure", "timed_out":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true).Render("✗")
	}
	return ""
}

// apiStepDuration computes a formatted duration from a WorkflowStep's API
// timestamps. Returns "" when timestamps are absent.
func apiStepDuration(s domain.WorkflowStep) string {
	if s.StartedAt == nil || s.CompletedAt == nil {
		return ""
	}
	return formatDuration(s.CompletedAt.Sub(*s.StartedAt))
}

// ---------------------------------------------------------------------------
// Error hint and humanize helpers
// ---------------------------------------------------------------------------

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
		return "Job log not found (it may have been deleted)."
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

// humanizeBytes renders n as "X B", "X KiB", "X MiB", etc.
func humanizeBytes(n int) string {
	const (
		kib = 1024
		mib = 1024 * 1024
	)
	switch {
	case n < kib:
		return fmt.Sprintf("%d B", n)
	case n < mib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
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

// padLeft pads s with spaces on the left to n display columns.
func padLeft(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return strings.Repeat(" ", n-w) + s
}
