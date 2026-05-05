package run

import "time"

// Test seams: expose internal types and accessors to in-package tests
// (and any future external_test files) without leaking from the public
// API.

// TickMsg is the package-private auto-refresh tick, exposed for tests.
type TickMsg = tickMsg

// RunLoadedMsg is the package-private GetRun result message.
type RunLoadedMsg = runLoadedMsg

// JobsLoadedMsg is the package-private ListJobs result message.
type JobsLoadedMsg = jobsLoadedMsg

// JobIndex returns the jobs-pane cursor position.
func (m *Model) JobIndex() int { return m.jobCursor }

// StepIndex returns the steps-pane cursor position.
func (m *Model) StepIndex() int { return m.stepCursor }

// FocusJobs reports whether the jobs pane currently has focus.
func (m *Model) FocusJobs() bool { return m.focus == focusJobs }

// IsLoading reports whether the screen is still in the loading state.
func (m *Model) IsLoading() bool { return m.state == stateLoading }

// HasRefreshErr reports whether a refresh-error banner should be shown.
func (m *Model) HasRefreshErr() bool { return m.refreshErr != nil }

// LastRefreshed returns the last successful refresh time and whether
// any refresh has completed yet.
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}

// AutoRefreshOn reports whether auto-refresh is enabled.
func (m *Model) AutoRefreshOn() bool { return m.autoRefresh }
