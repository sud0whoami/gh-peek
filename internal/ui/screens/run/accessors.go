package run

// Public accessors used by the root app router and other packages.
// Test-only accessors live in export_test.go.

// RunID returns the run ID this detail screen is showing.
func (m *Model) RunID() int64 { return m.params.RunID }

// Repo returns the repository this detail screen is showing.
func (m *Model) Repo() (host, owner, name string) {
	return m.params.Repo.Host, m.params.Repo.Owner, m.params.Repo.Name
}

// ViewportWidth returns the model's tracked terminal width.
func (m *Model) Width() int { return m.width }

// ViewportHeight returns the model's tracked terminal height.
func (m *Model) Height() int { return m.height }
