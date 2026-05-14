package packages

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
	title := "Packages · " + repo.Owner + "/" + repo.Name
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
		return m.theme.Muted("Loading packages…")
	case StateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	case StateEmpty:
		return m.theme.Muted("No packages published from this repository. Press 'r' to refresh.")
	case StateReady:
		v := m.visible()
		if len(v) == 0 {
			return m.theme.Muted("No packages match the current filter. Press 'r' to refresh.")
		}
		return m.renderTable(v)
	}
	return ""
}

// columnWidths computes per-column widths.
// Columns: TYPE | NAME | VISIBILITY | VERSIONS | UPDATED
func (m *Model) columnWidths() (typ, name, vis, versions, updated int) {
	const sepCount = 4
	avail := m.width - sepCount
	if avail < 30 {
		avail = 30
	}
	typ = widgets.Clamp(avail/8, 7, 11)
	vis = widgets.Clamp(avail/10, 7, 10)
	versions = widgets.Clamp(avail/12, 4, 8)
	updated = widgets.Clamp(avail/8, 8, 12)
	used := typ + vis + versions + updated
	name = avail - used
	if name < 8 {
		name = 8
	}
	return
}

func (m *Model) renderTable(rows []domain.Package) string {
	typ, name, vis, versions, updated := m.columnWidths()
	var b strings.Builder
	header := widgets.JoinCells(
		widgets.PadRight(widgets.TruncRune("TYPE", typ), typ),
		widgets.PadRight(widgets.TruncRune("NAME", name), name),
		widgets.PadRight(widgets.TruncRune("VISIBILITY", vis), vis),
		widgets.PadRight(widgets.TruncRune("VERS", versions), versions),
		widgets.PadRight(widgets.TruncRune("UPDATED", updated), updated),
	)
	b.WriteString(m.theme.SectionLabel(m.truncate(header)))
	b.WriteByte('\n')

	now := m.params.Now()
	for i, p := range rows {
		typeBadge := widgets.PadToVisible(m.renderTypeBadge(p), typ)
		upd := "—"
		if !p.UpdatedAt.IsZero() {
			upd = widgets.HumanizeAgo(now.Sub(p.UpdatedAt))
		}
		row := widgets.JoinCells(
			typeBadge,
			widgets.PadRight(widgets.TruncRune(p.Name, name), name),
			widgets.PadRight(widgets.TruncRune(p.Visibility, vis), vis),
			widgets.PadRight(widgets.TruncRune(fmt.Sprintf("%d", p.VersionCount), versions), versions),
			widgets.PadRight(widgets.TruncRune(upd, updated), updated),
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

// renderTypeBadge picks a colored badge for the package type.
func (m *Model) renderTypeBadge(p domain.Package) string {
	switch p.Type {
	case domain.PackageTypeContainer, domain.PackageTypeDocker:
		return styled(m.theme.Info, string(p.Type))
	case domain.PackageTypeNPM, domain.PackageTypeMaven, domain.PackageTypeRubyGems, domain.PackageTypeNuGet:
		return styled(m.theme.Success, string(p.Type))
	default:
		return styled(m.theme.MutedColor, string(p.Type))
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
	case errors.Is(err, githubapi.ErrMissingPackagesScope):
		return "Missing read:packages scope. Run `gh auth refresh -s read:packages` and retry."
	case errors.Is(err, githubapi.ErrUnauthorized):
		return "Unauthorized — Run `gh auth login` to refresh your token."
	case errors.Is(err, githubapi.ErrNotFound):
		return "Repository not found, or no packages published."
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
