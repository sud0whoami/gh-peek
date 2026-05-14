package layout

import (
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultMaxWidth is the content width cap used when GHPEEK_MAX_WIDTH is
	// not set. Content wider than this is not rendered; slack becomes
	// left/right padding so the frame is centered.
	DefaultMaxWidth = 140

	// MinWidth is the smallest terminal width gh-peek can render in.
	// Below this, the root model shows a "too narrow" hint instead of
	// delegating to child screens.
	MinWidth = 80
)

// maxWidth returns the effective max width, honoring the env override.
func maxWidth() int {
	if v := os.Getenv("GHPEEK_MAX_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= MinWidth {
			return n
		}
	}
	return DefaultMaxWidth
}

// Frame is the result of Compute: all the integers a screen or the root
// model needs to render content that is capped and centered.
type Frame struct {
	// Terminal is the raw terminal width as reported by WindowSizeMsg.
	Terminal int
	// Content is the width screens should render into (≤ Terminal, ≤ MaxWidth).
	Content int
	// LeftPad is the number of spaces to prepend to each line so the
	// rendered block appears centered in the terminal.
	LeftPad int
	// TooNarrow is true when Terminal < MinWidth.
	TooNarrow bool
}

// Compute returns a Frame for the given terminal width.
// Compute(0) is valid and returns {Content: maxWidth(), TooNarrow: false}
// (used in tests and default-init contexts).
func Compute(terminalWidth int) Frame {
	max := maxWidth()
	if terminalWidth <= 0 {
		return Frame{Terminal: terminalWidth, Content: max}
	}
	if terminalWidth < MinWidth {
		return Frame{Terminal: terminalWidth, Content: terminalWidth, TooNarrow: true}
	}
	if terminalWidth <= max {
		return Frame{Terminal: terminalWidth, Content: terminalWidth}
	}
	content := max
	leftPad := (terminalWidth - content) / 2
	return Frame{Terminal: terminalWidth, Content: content, LeftPad: leftPad}
}

// Center prepends LeftPad spaces to every non-empty line in s.
// If LeftPad is 0 the string is returned unchanged.
func (f Frame) Center(s string) string {
	if f.LeftPad <= 0 || s == "" {
		return s
	}
	pad := strings.Repeat(" ", f.LeftPad)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}
