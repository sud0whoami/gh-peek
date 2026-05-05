package keymap_test

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
)

func TestDefaultLogViewer_BindingsAreComplete(t *testing.T) {
	l := keymap.DefaultLogViewer()
	bindings := map[string]key.Binding{
		"Up":                l.Up,
		"Down":              l.Down,
		"PageUp":            l.PageUp,
		"PageDown":          l.PageDown,
		"Top":               l.Top,
		"Bottom":            l.Bottom,
		"Search":            l.Search,
		"NextMatch":         l.NextMatch,
		"PrevMatch":         l.PrevMatch,
		"ToggleWrap":        l.ToggleWrap,
		"JumpFailure":       l.JumpFailure,
		"Refresh":           l.Refresh,
		"ToggleAutoRefresh": l.ToggleAutoRefresh,
		"OpenBrowser":       l.OpenBrowser,
		"Back":              l.Back,
		"Help":              l.Help,
		"Quit":              l.Quit,
	}
	for name, b := range bindings {
		if len(b.Keys()) == 0 {
			t.Errorf("%s: expected at least one key", name)
		}
		if b.Help().Desc == "" {
			t.Errorf("%s: expected non-empty Help.Desc", name)
		}
	}
}

func TestLogViewer_ShortAndFullHelp(t *testing.T) {
	l := keymap.DefaultLogViewer()
	if len(l.ShortHelp()) == 0 {
		t.Error("ShortHelp returned no bindings")
	}
	if len(l.FullHelp()) == 0 {
		t.Error("FullHelp returned no rows")
	}
}
