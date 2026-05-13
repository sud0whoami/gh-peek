// Package keymap defines shared key bindings for the gh-peek screens.
package keymap

import (
	"charm.land/bubbles/v2/key"
)

// Runs is the set of key bindings for the runs list screen.
type Runs struct {
	Up           key.Binding
	Down         key.Binding
	Open         key.Binding
	Search       key.Binding
	Refresh      key.Binding
	AutoToggle   key.Binding
	ActiveOnly   key.Binding
	BranchCycle  key.Binding
	OpenBrowse   key.Binding
	OpenReleases key.Binding
	Help         key.Binding
	Quit         key.Binding
	Cancel       key.Binding
}

// DefaultRuns returns the default Runs key bindings.
func DefaultRuns() Runs {
	return Runs{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open run"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		AutoToggle: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "toggle auto-refresh"),
		),
		ActiveOnly: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "active-only filter"),
		),
		BranchCycle: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "cycle branch/PR/all"),
		),
		OpenBrowse: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		OpenReleases: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "releases"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel/clear"),
		),
	}
}

// ShortHelp returns the bindings shown in the compact one-line help.
func (r Runs) ShortHelp() []key.Binding {
	return []key.Binding{r.Up, r.Down, r.Open, r.Search, r.Refresh, r.AutoToggle, r.OpenBrowse, r.OpenReleases, r.Help, r.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help
// overlay.
func (r Runs) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{r.Up, r.Down, r.Open, r.OpenBrowse},
		{r.Search, r.Cancel, r.ActiveOnly, r.BranchCycle},
		{r.OpenReleases, r.Refresh, r.AutoToggle, r.Help, r.Quit},
	}
}
