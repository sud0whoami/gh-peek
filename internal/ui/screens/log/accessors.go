package log

import "github.com/sud0whoami/gh-peek/internal/domain"

// Public accessors used by the root app router and other packages.
// Test-only accessors live in export_test.go.

// JobName returns the human-readable job name supplied by the parent.
// Empty when the parent did not provide one.
func (m *Model) JobName() string { return m.params.JobName }

// RunActive reports whether the parent told this screen the parent
// run is still in flight. The screen uses this to gate auto-refresh.
func (m *Model) RunActive() bool { return m.runActive }

// Steps returns the API-provided steps for this job, as supplied by the parent.
// Nil when the parent did not provide step data.
func (m *Model) Steps() []domain.WorkflowStep { return m.params.Steps }
