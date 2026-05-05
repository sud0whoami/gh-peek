// Package styles defines semantic theme tokens and helpers for the
// gh-peek UI layer.
//
// Tokens are intentionally semantic (Success, Failure, Pending, ...)
// rather than brand colors so that the TUI feels native in any
// terminal theme.
//
// Note: lipgloss v2 does not expose AdaptiveColor; instead we use
// truecolor hex values that read well on both light and dark
// backgrounds. The lipgloss profile downgrades to ANSI/256 colors as
// appropriate at render time.
package styles

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// Theme is a bundle of semantic colors and pre-built styles used
// across the UI.
type Theme struct {
	Success    color.Color
	Failure    color.Color
	Pending    color.Color
	Running    color.Color
	Cancelled  color.Color
	Skipped    color.Color
	MutedColor color.Color
	Accent     color.Color
	Selection  color.Color
	Border     color.Color
}

// DefaultTheme returns the built-in theme.
func DefaultTheme() Theme {
	return Theme{
		Success:    lipgloss.Color("#3fb950"),
		Failure:    lipgloss.Color("#f85149"),
		Pending:    lipgloss.Color("#d29922"),
		Running:    lipgloss.Color("#58a6ff"),
		Cancelled:  lipgloss.Color("#8b949e"),
		Skipped:    lipgloss.Color("#6e7681"),
		MutedColor: lipgloss.Color("#8b949e"),
		Accent:     lipgloss.Color("#79c0ff"),
		Selection:  lipgloss.Color("#1f6feb"),
		Border:     lipgloss.Color("#30363d"),
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

// Selected renders a row using the selection color.
func (t Theme) Selected(text string) string {
	return lipgloss.NewStyle().Foreground(t.Selection).Bold(true).Render(text)
}
