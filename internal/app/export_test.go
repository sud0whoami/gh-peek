package app

import (
	logscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/log"
	runscreen "github.com/sud0whoami/gh-peek/internal/ui/screens/run"
	"github.com/sud0whoami/gh-peek/internal/ui/screens/runs"
)

// ActiveScreenName returns the name of the currently active screen
// ("runs", "run-detail", "log-viewer", or "none"). Test-only.
func (m *Model) ActiveScreenName() string {
	switch m.active {
	case activeRuns:
		return "runs"
	case activeRunDetail:
		return "run-detail"
	case activeLogViewer:
		return "log-viewer"
	default:
		return "none"
	}
}

// RunsScreen exposes the embedded runs-list screen for tests.
func (m *Model) RunsScreen() *runs.Model { return m.runsScreen }

// DetailScreen exposes the embedded run-detail screen for tests.
func (m *Model) DetailScreen() *runscreen.Model { return m.detailScreen }

// LogScreen exposes the embedded log-viewer screen for tests.
func (m *Model) LogScreen() *logscreen.Model { return m.logScreen }

// Width returns the router's tracked terminal width.
func (m *Model) Width() int { return m.width }
