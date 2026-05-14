package releases

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
	"github.com/sud0whoami/gh-peek/internal/ui/widgets"
	"github.com/sud0whoami/gh-peek/internal/ui/widgets/table"
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
		return "✓ " + widgets.HumanizeAgo(d)
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

// releasesTable defines the column layout for the releases list.
var releasesTable = table.Table{
	Cols: []table.Col{
		{Title: "", Min: 10, Max: 10, Ideal: 10}, // badge (fixed)
		{Title: "TAG", Min: 8, Max: 16, Ideal: 14},
		{Title: "TITLE", Min: 8, Max: 80, Ideal: 44, Elastic: true},
		{Title: "AUTHOR", Min: 6, Max: 14, Ideal: 10},
		{Title: "ASSETS", Min: 6, Max: 8, Ideal: 7},
		{Title: "PUBLISHED", Min: 8, Max: 12, Ideal: 10},
	},
}

func (m *Model) renderTable(rows []domain.Release) string {
	widths := releasesTable.Layout(m.width)
	badge, tag, title, author, assets, published := widths[0], widths[1], widths[2], widths[3], widths[4], widths[5]
	var b strings.Builder
	header := releasesTable.Header(widths, func(s string) string { return s })
	b.WriteString(m.theme.SectionLabel(m.truncate(header)))
	b.WriteByte('\n')

	latestID := m.latestID()
	now := m.params.Now()
	for i, r := range rows {
		badgeCell := widgets.PadToVisible(m.renderBadge(r, latestID), badge)
		pub := "—"
		if r.PublishedAt != nil {
			pub = widgets.HumanizeAgo(now.Sub(*r.PublishedAt))
		} else if r.Draft {
			pub = "draft"
		}
		titleText := r.Name
		if titleText == "" {
			titleText = r.TagName
		}
		row := widgets.JoinCells(
			badgeCell,
			widgets.PadRight(widgets.TruncRune(r.TagName, tag), tag),
			widgets.PadRight(widgets.TruncRune(titleText, title), title),
			widgets.PadRight(widgets.TruncRune(r.Author.Login, author), author),
			widgets.PadRight(widgets.TruncRune(fmt.Sprintf("%d", len(r.Assets)), assets), assets),
			widgets.PadRight(widgets.TruncRune(pub, published), published),
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
	return widgets.TruncRune(s, m.width)
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

// truncRune truncates s to n display columns, appending "…" if cut.
