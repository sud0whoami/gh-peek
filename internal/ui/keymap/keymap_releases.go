package keymap

import "charm.land/bubbles/v2/key"

// Releases is the set of key bindings for the releases list screen.
type Releases struct {
	Up           key.Binding
	Down         key.Binding
	Open         key.Binding
	Search       key.Binding
	Refresh      key.Binding
	AutoToggle   key.Binding
	OpenBrowse   key.Binding
	OpenPackages key.Binding
	Back         key.Binding
	Help         key.Binding
	Quit         key.Binding
	Cancel       key.Binding
}

// DefaultReleases returns the default Releases key bindings.
func DefaultReleases() Releases {
	return Releases{
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
			key.WithHelp("enter", "open release"),
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
		OpenBrowse: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		OpenPackages: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "packages"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "b", "L", "W"),
			key.WithHelp("esc/b/L/W", "back to runs"),
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
func (r Releases) ShortHelp() []key.Binding {
	return []key.Binding{r.Up, r.Down, r.Open, r.Search, r.Refresh, r.AutoToggle, r.OpenBrowse, r.OpenPackages, r.Back, r.Help, r.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help
// overlay.
func (r Releases) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{r.Up, r.Down, r.Open, r.OpenBrowse},
		{r.Search, r.Cancel, r.Back},
		{r.OpenPackages, r.Refresh, r.AutoToggle, r.Help, r.Quit},
	}
}

// ReleaseDetail is the set of key bindings for the release detail screen.
type ReleaseDetail struct {
	Up                key.Binding
	Down              key.Binding
	Tab               key.Binding
	Enter             key.Binding
	Refresh           key.Binding
	ToggleAutoRefresh key.Binding
	OpenBrowser       key.Binding
	Back              key.Binding
	Help              key.Binding
	Quit              key.Binding
}

// DefaultReleaseDetail returns the default ReleaseDetail key bindings.
func DefaultReleaseDetail() ReleaseDetail {
	return ReleaseDetail{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "focus notes/assets"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open asset"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		ToggleAutoRefresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "toggle auto-refresh"),
		),
		OpenBrowser: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "b"),
			key.WithHelp("esc/b", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns the bindings shown in the compact one-line help.
func (r ReleaseDetail) ShortHelp() []key.Binding {
	return []key.Binding{r.Up, r.Down, r.Tab, r.Enter, r.Refresh, r.ToggleAutoRefresh, r.OpenBrowser, r.Back, r.Help, r.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help overlay.
func (r ReleaseDetail) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{r.Up, r.Down, r.Tab, r.Enter},
		{r.OpenBrowser, r.Back},
		{r.Refresh, r.ToggleAutoRefresh, r.Help, r.Quit},
	}
}
