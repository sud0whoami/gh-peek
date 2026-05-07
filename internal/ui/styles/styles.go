// Package styles defines semantic theme tokens and helpers for the
// gh-peek UI layer.
package styles

import (
	"image/color"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// Theme is a bundle of semantic colors and pre-built styles used across the UI.
// Colors follow the Dracula palette.
type Theme struct {
	// Status colors.
	Success   color.Color
	Failure   color.Color
	Pending   color.Color
	Running   color.Color
	Cancelled color.Color
	Skipped   color.Color

	// UI chrome colors.
	MutedColor  color.Color // subtle text, timestamps, pane section labels
	Accent      color.Color // titles, highlights, section labels
	Selection   color.Color // kept for single-color selected text (raw log match)
	SelectionBg color.Color // background fill for the cursor row
	SelectionFg color.Color // foreground text on the cursor row
	Border      color.Color // pane borders, divider lines
	HeaderBg    color.Color // header bar background
}

// DefaultTheme returns the built-in Dracula-palette theme.
func DefaultTheme() Theme {
	return Theme{
		Success:   lipgloss.Color("#50fa7b"),
		Failure:   lipgloss.Color("#ff5555"),
		Pending:   lipgloss.Color("#f1fa8c"),
		Running:   lipgloss.Color("#8be9fd"),
		Cancelled: lipgloss.Color("#6272a4"),
		Skipped:   lipgloss.Color("#6272a4"),

		MutedColor:  lipgloss.Color("#6272a4"),
		Accent:      lipgloss.Color("#bd93f9"),
		Selection:   lipgloss.Color("#8be9fd"),
		SelectionBg: lipgloss.Color("#44475a"),
		SelectionFg: lipgloss.Color("#f8f8f2"),
		Border:      lipgloss.Color("#6272a4"),
		HeaderBg:    lipgloss.Color("#44475a"),
	}
}

// statusGlyph returns the single short character for the given status.
func statusGlyph(s domain.SemanticStatus) string {
	switch s {
	case domain.StatusSuccess:
		return "✓"
	case domain.StatusFailure:
		return "✗"
	case domain.StatusRunning:
		return "●"
	case domain.StatusPending:
		return "○"
	case domain.StatusCancelled:
		return "⊘"
	case domain.StatusSkipped:
		return "−"
	default:
		return "?"
	}
}

func (t Theme) colorFor(s domain.SemanticStatus) color.Color {
	switch s {
	case domain.StatusSuccess:
		return t.Success
	case domain.StatusFailure:
		return t.Failure
	case domain.StatusRunning:
		return t.Running
	case domain.StatusPending:
		return t.Pending
	case domain.StatusCancelled:
		return t.Cancelled
	case domain.StatusSkipped:
		return t.Skipped
	default:
		return t.MutedColor
	}
}

// Badge renders a compact, colored label for the given status, e.g.
// "[ ✓ success ]".
func (t Theme) Badge(s domain.SemanticStatus) string {
	text := "[ " + statusGlyph(s) + " " + string(s) + " ]"
	return lipgloss.NewStyle().Foreground(t.colorFor(s)).Bold(true).Render(text)
}

// Header renders a top-of-screen header line.
func (t Theme) Header(text string) string {
	return lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(text)
}

// Muted renders text in the muted color.
func (t Theme) Muted(text string) string {
	return lipgloss.NewStyle().Foreground(t.MutedColor).Render(text)
}

// Help renders the bottom help line.
func (t Theme) Help(text string) string {
	return lipgloss.NewStyle().Foreground(t.MutedColor).Render(text)
}

// ErrorBanner renders an inline error / warning banner.
func (t Theme) ErrorBanner(text string) string {
	return lipgloss.NewStyle().Foreground(t.Failure).Bold(true).Render(text)
}

// Selected renders a row using the selection foreground color (used in raw log match).
func (t Theme) Selected(text string) string {
	return lipgloss.NewStyle().Foreground(t.Selection).Bold(true).Render(text)
}

// SelectedRow fills a full-width row with the selection background.
func (t Theme) SelectedRow(text string, width int) string {
	return lipgloss.NewStyle().
		Background(t.SelectionBg).
		Foreground(t.SelectionFg).
		Bold(true).
		Width(width).
		Render(text)
}

// HeaderBar renders a full-width background bar with a bold accent title on
// the left and a muted indicator string on the right.
func (t Theme) HeaderBar(title, right string, width int) string {
	titleStyled := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(title)
	rightStyled := lipgloss.NewStyle().Foreground(t.MutedColor).Render(right)
	lw := lipgloss.Width(titleStyled)
	rw := lipgloss.Width(rightStyled)
	gap := width - lw - rw
	if gap < 1 {
		gap = 1
	}
	content := titleStyled + strings.Repeat(" ", gap) + rightStyled
	return lipgloss.NewStyle().Background(t.HeaderBg).Render(content)
}

// Divider renders a horizontal rule in the border color.
func (t Theme) Divider(width int) string {
	return lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", width))
}

// SectionLabel renders a column/section header label (e.g., WORKFLOW, JOBS).
func (t Theme) SectionLabel(text string) string {
	return lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(text)
}

// PaneBox wraps content in a rounded border. innerWidth is the inner content width;
// the rendered box will be innerWidth+2 characters wide.
func (t Theme) PaneBox(content string, innerWidth int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Width(innerWidth).
		Render(content)
}
