package releases

import "time"

// Test seam: re-export internal types and helpers.

type TickMsg = tickMsg
type ReleasesLoadedMsg = releasesLoadedMsg

func (m *Model) SelectedIndex() int       { return m.cursor }
func (m *Model) IsSearching() bool        { return m.input.Focused() }
func (m *Model) LastETag() string         { return m.lastETag }
func (m *Model) VisibleReleaseCount() int { return len(m.visible()) }
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}
