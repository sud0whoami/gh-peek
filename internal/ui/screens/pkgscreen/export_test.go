package pkgscreen

import "time"

type VersionsLoadedMsg = versionsLoadedMsg

func (m *Model) PackageID() int64 { return m.params.PackageID }
func (m *Model) Cursor() int      { return m.cursor }
func (m *Model) VersionCount() int {
	return len(m.versions)
}
func (m *Model) LastRefreshed() (time.Time, bool) {
	return m.lastRefreshed, !m.lastRefreshed.IsZero()
}
