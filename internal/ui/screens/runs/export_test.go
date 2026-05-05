package runs

import "time"

// Test seam: re-export internal types and helpers so external_test
// (and the runs_test.go file in this package) can drive the model
// directly.

// TickMsg is the package-private auto-refresh tick, exposed for tests.
type TickMsg = tickMsg

// RunsLoadedMsg is the package-private fetch-result message, exposed
// for tests.
type RunsLoadedMsg = runsLoadedMsg

// SelectedIndex returns the table cursor position. Test-only.
func (m *Model) SelectedIndex() int { return m.cursor }

// IsSearching reports whether the search input has focus. Test-only.
func (m *Model) IsSearching() bool { return m.input.Focused() }

// LastETag returns the ETag stored from the most recent successful
// fetch. Test-only.
func (m *Model) LastETag() string { return m.lastETag }

// LastRefreshed returns the last-successful-fetch timestamp.
// Test-only.
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}

// VisibleRunCount returns the number of runs the table currently
// shows after applying client-side filters. Test-only.
func (m *Model) VisibleRunCount() int { return len(m.visible()) }

// ViewMode returns the current view mode as one of "branch", "pr", or
// "all". Test-only.
func (m *Model) ViewMode() string { return m.viewMode.String() }
