package release

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/widgets"
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
	b.WriteString(m.renderBody())
	b.WriteByte('\n')
	b.WriteString(m.renderFooter())
	return tea.NewView(b.String())
}

func (m *Model) renderHeader() string {
	title := m.rel.Name
	if title == "" {
		title = m.rel.TagName
	}
	if title == "" {
		title = fmt.Sprintf("Release #%d", m.params.ReleaseID)
	}
	headerTitle := "Release · " + title
	return m.theme.HeaderBar(headerTitle, m.statusIndicatorText(), m.width)
}

func (m *Model) statusIndicatorText() string {
	switch {
	case m.state == stateLoading:
		return "↻"
	case !m.autoRefresh:
		return "⏼ off"
	case !m.lastRefreshed.IsZero():
		d := m.params.Now().Sub(m.lastRefreshed)
		return "✓ " + widgets.HumanizeAgo(d)
	default:
		return "✓"
	}
}

func (m *Model) renderSubheader() string {
	if m.state != stateReady {
		return m.theme.Muted("")
	}
	parts := []string{m.rel.TagName}
	if m.rel.Draft {
		parts = append(parts, m.theme.Muted("draft"))
	} else if m.rel.Prerelease {
		parts = append(parts, lipgloss.NewStyle().Foreground(m.theme.Pending).Bold(true).Render("prerelease"))
	}
	if m.rel.Author.Login != "" {
		parts = append(parts, "by "+m.rel.Author.Login)
	}
	if m.rel.PublishedAt != nil {
		parts = append(parts, "published "+widgets.HumanizeAgo(m.params.Now().Sub(*m.rel.PublishedAt)))
	}
	parts = append(parts, fmt.Sprintf("%d assets", len(m.rel.Assets)))
	return m.truncate(strings.Join(parts, " · "))
}

func (m *Model) renderBody() string {
	switch m.state {
	case stateLoading:
		return m.theme.Muted("Loading release…")
	case stateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	}
	if m.width >= 100 {
		return m.renderTwoPane()
	}
	return m.renderStacked()
}

func (m *Model) renderTwoPane() string {
	notesOuter := m.width * 6 / 10
	if notesOuter < 40 {
		notesOuter = 40
	}
	if notesOuter > m.width-30 {
		notesOuter = m.width - 30
	}
	assetsOuter := m.width - notesOuter
	notesInner := notesOuter - 2
	assetsInner := assetsOuter - 2

	notesContent := m.buildNotesContent(notesInner)
	assetsContent := m.buildAssetsContent(assetsInner)
	notesPane := m.theme.PaneBox(notesContent, notesInner)
	assetsPane := m.theme.PaneBox(assetsContent, assetsInner)
	return lipgloss.JoinHorizontal(lipgloss.Top, notesPane, assetsPane)
}

func (m *Model) renderStacked() string {
	innerWidth := m.width - 2
	notesContent := m.buildNotesContent(innerWidth)
	assetsContent := m.buildAssetsContent(innerWidth)
	notesPane := m.theme.PaneBox(notesContent, innerWidth)
	assetsPane := m.theme.PaneBox(assetsContent, innerWidth)
	return lipgloss.JoinVertical(lipgloss.Left, notesPane, assetsPane)
}

// notesViewportHeight returns the number of lines reserved for the notes pane.
func (m *Model) notesViewportHeight() int {
	// Heuristic: leave room for header (3) + subheader + footer (~3) + asset pane when stacked.
	rows := m.height - 10
	if m.width >= 100 {
		rows = m.height - 8
	}
	if rows < 6 {
		rows = 6
	}
	return rows
}

func (m *Model) buildNotesContent(width int) string {
	var b strings.Builder
	header := "NOTES"
	if m.focus == focusNotes {
		header = "▶ NOTES"
	}
	b.WriteString(widgets.PadRight(m.theme.SectionLabel(widgets.TruncRune(header, width)), width))
	b.WriteByte('\n')

	body := strings.TrimSpace(m.rel.Body)
	if body == "" {
		b.WriteString(widgets.PadRight(m.theme.Muted(widgets.TruncRune("(no release notes)", width)), width))
		return b.String()
	}
	lines := wrapLines(body, width)
	vh := m.notesViewportHeight()
	start := m.notesScroll
	if start > len(lines)-1 {
		start = len(lines) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + vh
	if end > len(lines) {
		end = len(lines)
	}
	for i := start; i < end; i++ {
		b.WriteString(widgets.PadRight(widgets.TruncRune(lines[i], width), width))
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *Model) buildAssetsContent(width int) string {
	var b strings.Builder
	header := "ASSETS"
	if m.focus == focusAssets {
		header = "▶ ASSETS"
	}
	b.WriteString(widgets.PadRight(m.theme.SectionLabel(widgets.TruncRune(header, width)), width))
	b.WriteByte('\n')

	if len(m.rel.Assets) == 0 {
		b.WriteString(widgets.PadRight(m.theme.Muted(widgets.TruncRune("(no assets)", width)), width))
		return b.String()
	}
	for i, a := range m.rel.Assets {
		size := humanizeBytes(a.Size)
		prefix := "  "
		if i == m.assetCursor && m.focus == focusAssets {
			prefix = "▶ "
		}
		row := fmt.Sprintf("%s%s  %s  ↓%d", prefix, a.Name, size, a.DownloadCount)
		if i == m.assetCursor {
			row = m.theme.SelectedRow(row, width)
		} else {
			row = widgets.PadRight(widgets.TruncRune(row, width), width)
		}
		b.WriteString(row)
		if i < len(m.rel.Assets)-1 {
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

func (m *Model) truncate(s string) string {
	if m.width <= 0 {
		return s
	}
	if lipgloss.Width(s) <= m.width {
		return s
	}
	return widgets.TruncRune(s, m.width)
}

// wrapLines wraps the input string into lines that fit width display
// columns. Existing newlines are preserved. Markdown is rendered as
// plain text — heading prefixes (`#`, `##`) and list markers (`-`, `*`)
// are kept verbatim. Trailing/leading blank lines are preserved as-is.
func wrapLines(s string, width int) []string {
	if width <= 0 {
		width = 80
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			out = append(out, "")
			continue
		}
		if lipgloss.Width(line) <= width {
			out = append(out, line)
			continue
		}
		out = append(out, breakLine(line, width)...)
	}
	return out
}

// breakLine splits a single line at word boundaries to fit width.
func breakLine(line string, width int) []string {
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{line}
	}
	// Preserve any leading indent / list marker by capturing leading whitespace.
	leading := ""
	for _, r := range line {
		if r == ' ' || r == '\t' {
			leading += string(r)
		} else {
			break
		}
	}
	var out []string
	var b strings.Builder
	b.WriteString(leading)
	curW := lipgloss.Width(leading)
	for _, w := range words {
		ww := lipgloss.Width(w)
		if curW == lipgloss.Width(leading) {
			// Start of a new wrapped line.
			b.WriteString(w)
			curW += ww
			continue
		}
		if curW+1+ww > width {
			out = append(out, b.String())
			b.Reset()
			b.WriteString(leading)
			b.WriteString(w)
			curW = lipgloss.Width(leading) + ww
			continue
		}
		b.WriteByte(' ')
		b.WriteString(w)
		curW += 1 + ww
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

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
		return "Release not found or no longer accessible."
	case errors.Is(err, githubapi.ErrForbidden):
		return "Forbidden — your token may lack the required scopes."
	default:
		return err.Error()
	}
}

func humanizeBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
