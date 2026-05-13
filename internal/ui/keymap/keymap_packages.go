package keymap

import "charm.land/bubbles/v2/key"

// Packages is the set of key bindings for the packages list screen.
type Packages struct {
	Up         key.Binding
	Down       key.Binding
	Open       key.Binding
	Search     key.Binding
	Refresh    key.Binding
	AutoToggle key.Binding
	OpenBrowse key.Binding
	Back       key.Binding
	Help       key.Binding
	Quit       key.Binding
	Cancel     key.Binding
}

// DefaultPackages returns the default Packages key bindings.
func DefaultPackages() Packages {
	return Packages{
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
			key.WithHelp("enter", "open package"),
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
		Back: key.NewBinding(
			key.WithKeys("esc", "b", "P"),
			key.WithHelp("esc/b/P", "back to runs"),
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
func (p Packages) ShortHelp() []key.Binding {
	return []key.Binding{p.Up, p.Down, p.Open, p.Search, p.Refresh, p.AutoToggle, p.OpenBrowse, p.Back, p.Help, p.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help overlay.
func (p Packages) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{p.Up, p.Down, p.Open, p.OpenBrowse},
		{p.Search, p.Cancel, p.Back},
		{p.Refresh, p.AutoToggle, p.Help, p.Quit},
	}
}

// PackageDetail is the set of key bindings for the package detail screen.
type PackageDetail struct {
	Up                key.Binding
	Down              key.Binding
	Refresh           key.Binding
	ToggleAutoRefresh key.Binding
	OpenBrowser       key.Binding
	OpenPackage       key.Binding
	Back              key.Binding
	Help              key.Binding
	Quit              key.Binding
}

// DefaultPackageDetail returns the default PackageDetail key bindings.
func DefaultPackageDetail() PackageDetail {
	return PackageDetail{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
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
			key.WithHelp("o", "open version"),
		),
		OpenPackage: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "open package"),
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
func (p PackageDetail) ShortHelp() []key.Binding {
	return []key.Binding{p.Up, p.Down, p.Refresh, p.ToggleAutoRefresh, p.OpenBrowser, p.OpenPackage, p.Back, p.Help, p.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help overlay.
func (p PackageDetail) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{p.Up, p.Down, p.OpenBrowser, p.OpenPackage},
		{p.Back},
		{p.Refresh, p.ToggleAutoRefresh, p.Help, p.Quit},
	}
}
