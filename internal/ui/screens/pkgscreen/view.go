package pkgscreen

import (
	"errors"
	"fmt"
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
	title := m.pkg.Name
	if title == "" {
		title = fmt.Sprintf("Package #%d", m.params.PackageID)
	}
	return m.theme.HeaderBar("Package · "+title, m.statusIndicatorText(), m.width)
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
	parts := []string{}
	if m.pkg.Type != "" {
		parts = append(parts, string(m.pkg.Type))
	}
	if m.pkg.Visibility != "" {
		parts = append(parts, m.pkg.Visibility)
	}
	if m.pkg.Owner.Login != "" {
		parts = append(parts, "owner: "+m.pkg.Owner.Login)
	}
	if m.pkg.Repository != nil && m.pkg.Repository.FullName != "" {
		parts = append(parts, "repo: "+m.pkg.Repository.FullName)
	}
	if m.pkg.VersionCount > 0 {
		parts = append(parts, fmt.Sprintf("%d versions", m.pkg.VersionCount))
	}
	if !m.pkg.UpdatedAt.IsZero() {
		parts = append(parts, "updated "+widgets.HumanizeAgo(m.params.Now().Sub(m.pkg.UpdatedAt)))
	}
	return m.theme.Muted(m.truncate(strings.Join(parts, " · ")))
}

func (m *Model) renderBody() string {
	switch m.state {
	case stateLoading:
		return m.theme.Muted("Loading versions…")
	case stateError:
		return m.theme.ErrorBanner(m.truncate(errorHint(m.loadErr)))
	}
	if len(m.versions) == 0 {
		return m.theme.Muted("(no versions)")
	}
	return m.renderVersions()
}

// containerTable defines columns for container/docker versions: NAME (fixed sha256) | TAGS (elastic, capped) | CREATED
var containerTable = table.Table{
	Cols: []table.Col{
		{Title: "NAME", Min: 19, Max: 20, Ideal: 20},
		{Title: "TAGS", Min: 12, Max: 32, Ideal: 32, Elastic: true},
		{Title: "CREATED", Min: 8, Max: 12, Ideal: 12},
	},
}

// versionTable defines columns for non-container versions: NAME (elastic) | CREATED
var versionTable = table.Table{
	Cols: []table.Col{
		{Title: "NAME", Min: 12, Max: 120, Ideal: 60, Elastic: true},
		{Title: "CREATED", Min: 8, Max: 12, Ideal: 12},
	},
}

// renderVersions lists the package versions in a single pane.
// Columns: NAME | TAGS (containers only) | CREATED
func (m *Model) renderVersions() string {
	isContainer := m.pkg.Type == domain.PackageTypeContainer || m.pkg.Type == domain.PackageTypeDocker

	var b strings.Builder
	if isContainer {
		widths := containerTable.Layout(m.width)
		nameW, tagsW, createdW := widths[0], widths[1], widths[2]
		header := containerTable.Header(widths, func(s string) string { return s })
		b.WriteString(m.theme.SectionLabel(m.truncate(header)))
		b.WriteByte('\n')
		now := m.params.Now()
		for i, v := range m.versions {
			created := "—"
			if !v.CreatedAt.IsZero() {
				created = widgets.HumanizeAgo(now.Sub(v.CreatedAt))
			}
			name := shortenDigest(v.Name)
			tags := strings.Join(v.Metadata.ContainerTags, ", ")
			row := widgets.JoinCells(
				widgets.PadRight(widgets.TruncRune(name, nameW), nameW),
				widgets.PadRight(widgets.TruncRune(tags, tagsW), tagsW),
				widgets.PadRight(widgets.TruncRune(created, createdW), createdW),
			)
			if i == m.cursor {
				row = m.theme.SelectedRow(row, m.width)
			}
			b.WriteString(row)
			if i < len(m.versions)-1 {
				b.WriteByte('\n')
			}
		}
	} else {
		widths := versionTable.Layout(m.width)
		nameW, createdW := widths[0], widths[1]
		header := versionTable.Header(widths, func(s string) string { return s })
		b.WriteString(m.theme.SectionLabel(m.truncate(header)))
		b.WriteByte('\n')
		now := m.params.Now()
		for i, v := range m.versions {
			created := "—"
			if !v.CreatedAt.IsZero() {
				created = widgets.HumanizeAgo(now.Sub(v.CreatedAt))
			}
			row := widgets.JoinCells(
				widgets.PadRight(widgets.TruncRune(v.Name, nameW), nameW),
				widgets.PadRight(widgets.TruncRune(created, createdW), createdW),
			)
			if i == m.cursor {
				row = m.theme.SelectedRow(row, m.width)
			}
			b.WriteString(row)
			if i < len(m.versions)-1 {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// shortenDigest collapses a "sha256:<64-hex>" digest to a short
// "sha256:<first-12>" form for display. Other strings are returned
// unchanged.
func shortenDigest(s string) string {
	const prefix = "sha256:"
	if strings.HasPrefix(s, prefix) && len(s) >= len(prefix)+12 {
		return prefix + s[len(prefix):len(prefix)+12]
	}
	return s
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
		return "Package not found or no longer accessible."
	case errors.Is(err, githubapi.ErrRateLimited):
		var ae *githubapi.APIError
		if errors.As(err, &ae) && ae.RetryAfter > 0 {
			return "Rate limited; retry after " + ae.RetryAfter.String() + "."
		}
		return "Rate limited; retry shortly."
	case errors.Is(err, githubapi.ErrForbidden):
		return "Forbidden — your token may lack the required scopes."
	default:
		return err.Error()
	}
}
