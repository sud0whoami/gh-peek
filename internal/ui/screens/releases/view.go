package releases

import (
	"errors"
	"fmt"
	"image/color"
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
	repo := m.params.Repo
	title := "Releases · " + repo.Owner + "/" + repo.Name
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
		return m.theme.Muted("Loading releases…")
	case StateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	case StateEmpty:
		return m.theme.Muted("No releases for this repository. Press 'r' to refresh.")
	case StateReady:
		v := m.visible()
		if len(v) == 0 {
			return m.theme.Muted("No releases match the current filter. Press 'r' to refresh.")
		}
		return m.renderTable(v)
	}
	return ""
}

// columnWidths computes the per-column widths.
// Columns: BADGE | TAG | TITLE | AUTHOR | ASSETS | PUBLISHED
func (m *Model) columnWidths() (badge, tag, title, author, assets, published int) {
	const sepCount = 5
	const badgeCol = 10 // "[ latest ]" is 10 visible cells (currently widest badge)
	avail := m.width - sepCount
	if avail < 30 {
		avail = 30
	}
	badge = badgeCol
	rest := avail - badge
	tag = clampInt(rest/6, 8, 16)
	author = clampInt(rest/8, 6, 14)
	assets = clampInt(rest/12, 6, 8)
	published = clampInt(rest/8, 8, 12)
	used := tag + author + assets + published
	title = rest - used
	if title < 8 {
		title = 8
	}
	return
}

func (m *Model) renderTable(rows []domain.Release) string {
	badge, tag, title, author, assets, published := m.columnWidths()
	var b strings.Builder
	header := joinRow(
		padRight(truncRune("", badge), badge),
		padRight(truncRune("TAG", tag), tag),
		padRight(truncRune("TITLE", title), title),
		padRight(truncRune("AUTHOR", author), author),
		padRight(truncRune("ASSETS", assets), assets),
		padRight(truncRune("PUBLISHED", published), published),
	)
	b.WriteString(m.theme.SectionLabel(m.truncate(header)))
	b.WriteByte('\n')

	latestID := m.latestID()
	now := m.params.Now()
	for i, r := range rows {
		badgeCell := padToVisible(m.renderBadge(r, latestID), badge)
		pub := "—"
		if r.PublishedAt != nil {
			pub = humanizeAgo(now.Sub(*r.PublishedAt))
		} else if r.Draft {
			pub = "draft"
		}
		titleText := r.Name
		if titleText == "" {
			titleText = r.TagName
		}
		row := joinRow(
			badgeCell,
			padRight(truncRune(r.TagName, tag), tag),
			padRight(truncRune(titleText, title), title),
			padRight(truncRune(r.Author.Login, author), author),
			padRight(truncRune(fmt.Sprintf("%d", len(r.Assets)), assets), assets),
			padRight(truncRune(pub, published), published),
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

// renderBadge picks a colored badge for the release.
//   - "draft"  (muted)  for draft releases
//   - "pre"    (yellow) for prereleases
//   - "latest" (green)  for the newest published non-draft non-prerelease
//   - "stable" (cyan)   for any other published non-draft non-prerelease
func (m *Model) renderBadge(r domain.Release, latestID int64) string {
	switch {
	case r.Draft:
		return styled(m.theme.MutedColor, "[ draft ]")
	case r.Prerelease:
		return styled(m.theme.Pending, "[ pre ]")
	case r.ID == latestID && latestID != 0:
		return styled(m.theme.Success, "[ latest ]")
	default:
		return styled(m.theme.Info, "[ stable ]")
	}
}

func styled(c color.Color, text string) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(text)
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

// errorHint maps known sentinels to user-facing hints.
func errorHint(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, githubapi.ErrUnauthorized):
		return "Unauthorized — Run `gh auth login` to refresh your token."
	case errors.Is(err, githubapi.ErrNotFound):
		return "Repository not found, or no releases configured."
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

// humanizeAgo renders a duration as e.g. "5s ago" / "3m ago" / "2h ago" / "5d ago".
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

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func padToVisible(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

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
