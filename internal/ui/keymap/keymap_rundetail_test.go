package keymap_test

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
)

func TestDefaultRunDetail_BindingsAreComplete(t *testing.T) {
	r := keymap.DefaultRunDetail()
	bindings := map[string]key.Binding{
		"Up":                r.Up,
		"Down":              r.Down,
		"Tab":               r.Tab,
		"Enter":             r.Enter,
		"Refresh":           r.Refresh,
		"ToggleAutoRefresh": r.ToggleAutoRefresh,
		"OpenBrowser":       r.OpenBrowser,
		"Back":              r.Back,
		"Help":              r.Help,
		"Quit":              r.Quit,
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

func TestRunDetail_ShortAndFullHelp(t *testing.T) {
	r := keymap.DefaultRunDetail()
	if len(r.ShortHelp()) == 0 {
		t.Error("ShortHelp returned no bindings")
	}
	if len(r.FullHelp()) == 0 {
		t.Error("FullHelp returned no rows")
	}
}
