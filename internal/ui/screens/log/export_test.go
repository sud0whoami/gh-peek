package log

// Test seams: expose internal types and accessors to in-package tests.

// TickMsg is the package-private auto-refresh tick, exposed for tests.
type TickMsg = tickMsg

// LogLoadedMsg is the package-private DownloadJobLog result message,
// exposed for tests.
type LogLoadedMsg = logLoadedMsg

// Top returns the top-of-viewport line index (0-based).
func (m *Model) Top() int { return m.top }

// IsLoading reports whether the screen is still in the loading state.
func (m *Model) IsLoading() bool { return m.state == stateLoading }

// Wrap reports whether line wrap is enabled.
func (m *Model) Wrap() bool { return m.wrap }

// IsSearching reports whether the search input has focus.
func (m *Model) IsSearching() bool { return m.input.Focused() }

// MatchCount returns the number of search matches.
func (m *Model) MatchCount() int { return len(m.matches) }

// MatchIndex returns the 0-based current match cursor; -1 when none.
func (m *Model) MatchIndex() int { return m.matchCursor }

// AutoRefreshOn reports whether auto-refresh is enabled.
func (m *Model) AutoRefreshOn() bool { return m.autoRefresh }

// Width returns the model's tracked terminal width.
func (m *Model) Width() int { return m.width }

// HasRefreshErr reports whether a refresh-error banner should be shown.
func (m *Model) HasRefreshErr() bool { return m.refreshErr != nil }

// TruncatedFromAPI reports whether the API returned ErrLogTooLarge on
// the last successful load.
func (m *Model) TruncatedFromAPI() bool { return m.truncatedFromAPI }

// CurrentViewMode returns the current ViewMode.
func (m *Model) CurrentViewMode() ViewMode { return m.viewMode }

// Cursor returns the current cursor index into visibleRows.
func (m *Model) Cursor() int { return m.cursor }

// VisibleRowCount returns the number of visible rows (outline/compact modes).
func (m *Model) VisibleRowCount() int { return len(m.visibleRows) }

// IsExpanded reports whether the node at the given expansion key is expanded.
func (m *Model) IsExpanded(key string) bool { return m.expanded[key] }

// ShowTimestamps reports whether timestamps are shown in outline/compact modes.
func (m *Model) ShowTimestamps() bool { return m.showTimestamps }
