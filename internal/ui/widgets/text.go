package widgets

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

// TruncRune truncates s to n display columns, appending "…" if cut.
func TruncRune(s string, n int) string {
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

// PadRight pads s with spaces on the right to n display columns.
func PadRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// PadToVisible pads a possibly-styled string to n visible columns.
// Identical to PadRight but named to distinguish intent for pre-styled cells.
func PadToVisible(s string, n int) string {
	return PadRight(s, n)
}

// JoinCells joins cells with a single space separator.
func JoinCells(cells ...string) string {
	return strings.Join(cells, " ")
}

// Clamp clamps want into [lo, hi]. If hi <= 0 only lo is enforced.
func Clamp(want, lo, hi int) int {
	if want < lo {
		want = lo
	}
	if hi > 0 && want > hi {
		want = hi
	}
	return want
}

// HumanizeAgo renders a duration as "5s ago", "3m ago", "2h ago", "5d ago".
func HumanizeAgo(d time.Duration) string {
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
