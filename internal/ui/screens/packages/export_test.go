package packages

import "time"

// Test seam: re-export internal types and helpers.

type TickMsg = tickMsg
type PackagesLoadedMsg = packagesLoadedMsg

func (m *Model) SelectedIndex() int       { return m.cursor }
func (m *Model) IsSearching() bool        { return m.input.Focused() }
func (m *Model) VisiblePackageCount() int { return len(m.visible()) }
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}
