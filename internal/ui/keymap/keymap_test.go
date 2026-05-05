package keymap_test

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
)

func TestDefaultRuns_BindingsAreComplete(t *testing.T) {
	r := keymap.DefaultRuns()
	bindings := map[string]key.Binding{
		"Up":         r.Up,
		"Down":       r.Down,
		"Open":       r.Open,
		"Search":     r.Search,
		"Refresh":    r.Refresh,
		"AutoToggle": r.AutoToggle,
		"ActiveOnly": r.ActiveOnly,
		"OpenBrowse": r.OpenBrowse,
		"Help":       r.Help,
		"Quit":       r.Quit,
		"Cancel":     r.Cancel,
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

func TestRuns_ShortAndFullHelp(t *testing.T) {
	r := keymap.DefaultRuns()
	if len(r.ShortHelp()) == 0 {
		t.Error("ShortHelp returned no bindings")
	}
	if len(r.FullHelp()) == 0 {
		t.Error("FullHelp returned no rows")
	}
}
