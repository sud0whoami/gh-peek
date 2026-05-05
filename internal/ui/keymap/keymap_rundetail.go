package keymap

import "charm.land/bubbles/v2/key"

// RunDetail is the set of key bindings for the run-detail screen.
type RunDetail struct {
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

// DefaultRunDetail returns the default RunDetail key bindings.
func DefaultRunDetail() RunDetail {
	return RunDetail{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "focus jobs/steps"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open log"),
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
func (r RunDetail) ShortHelp() []key.Binding {
	return []key.Binding{r.Up, r.Down, r.Tab, r.Enter, r.Refresh, r.ToggleAutoRefresh, r.OpenBrowser, r.Back, r.Help, r.Quit}
}

// FullHelp returns bindings grouped into rows for the expanded help overlay.
func (r RunDetail) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{r.Up, r.Down, r.Tab, r.Enter},
		{r.OpenBrowser, r.Back},
		{r.Refresh, r.ToggleAutoRefresh, r.Help, r.Quit},
	}
}
