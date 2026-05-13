package release

import "time"

type ReleaseLoadedMsg = releaseLoadedMsg

func (m *Model) ReleaseID() int64    { return m.params.ReleaseID }
func (m *Model) Width() int          { return m.width }
func (m *Model) FocusIsNotes() bool  { return m.focus == focusNotes }
func (m *Model) FocusIsAssets() bool { return m.focus == focusAssets }
func (m *Model) AssetCursor() int    { return m.assetCursor }
func (m *Model) NotesScroll() int    { return m.notesScroll }
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}
