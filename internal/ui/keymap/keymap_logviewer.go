package keymap

import "charm.land/bubbles/v2/key"

// LogViewer is the set of key bindings for the job log viewer screen.
type LogViewer struct {
	Up                key.Binding
	Down              key.Binding
	PageUp            key.Binding
	PageDown          key.Binding
	Top               key.Binding
	Bottom            key.Binding
	Search            key.Binding
	NextMatch         key.Binding
	PrevMatch         key.Binding
	ToggleWrap        key.Binding
	JumpFailure       key.Binding
	Refresh           key.Binding
	ToggleAutoRefresh key.Binding
	OpenBrowser       key.Binding
	Back              key.Binding
	Help              key.Binding
	Quit              key.Binding

	// Outline-mode navigation bindings (outline/compact only).
	Toggle      key.Binding // enter/space — toggle focused node
	Expand      key.Binding // right/l — expand focused node
	Collapse    key.Binding // left/h — collapse or jump to parent
	ExpandAll   key.Binding // E — expand all nodes
	CollapseAll key.Binding // O — collapse all nodes
	Mode        key.Binding // v — cycle outline → compact → raw
	Timestamps  key.Binding // t — toggle timestamp display
}

// DefaultLogViewer returns the default LogViewer key bindings.
func DefaultLogViewer() LogViewer {
	return LogViewer{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("PgUp", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("PgDn", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g/home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G/end", "bottom"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		ToggleWrap: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "toggle wrap"),
		),
		JumpFailure: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "first failure"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reload"),
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
		Toggle: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("enter/space", "toggle node"),
		),
		Expand: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "collapse all"),
		),
		Mode: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "cycle mode"),
		),
		Timestamps: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "toggle timestamps"),
		),
	}
}

// ShortHelp returns the bindings shown in the compact one-line help.
func (l LogViewer) ShortHelp() []key.Binding {
	return []key.Binding{
		l.Up, l.Down, l.PageDown, l.Top, l.Bottom,
		l.Search, l.NextMatch, l.ToggleWrap, l.JumpFailure,
		l.Toggle, l.Mode,
		l.Refresh, l.ToggleAutoRefresh, l.OpenBrowser, l.Back, l.Help, l.Quit,
	}
}

// FullHelp returns bindings grouped into rows for the expanded help overlay.
func (l LogViewer) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{l.Up, l.Down, l.PageUp, l.PageDown, l.Top, l.Bottom},
		{l.Toggle, l.Expand, l.Collapse, l.ExpandAll, l.CollapseAll},
		{l.Search, l.NextMatch, l.PrevMatch, l.ToggleWrap, l.JumpFailure},
		{l.Mode, l.Timestamps, l.Refresh, l.ToggleAutoRefresh, l.OpenBrowser},
		{l.Back, l.Help, l.Quit},
	}
}
