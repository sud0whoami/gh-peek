package keymap_test

import (
	"testing"

	"github.com/sud0whoami/gh-peek/internal/ui/keymap"
)

func TestDefaultReleases_BindingsComplete(t *testing.T) {
	r := keymap.DefaultReleases()
	if len(r.ShortHelp()) == 0 {
		t.Error("ShortHelp empty")
	}
	if len(r.FullHelp()) == 0 {
		t.Error("FullHelp empty")
	}
	for _, b := range r.ShortHelp() {
		if len(b.Keys()) == 0 {
			t.Errorf("binding missing keys: %+v", b.Help())
		}
	}
}

func TestRuns_HasOpenReleasesBinding(t *testing.T) {
	r := keymap.DefaultRuns()
	keys := r.OpenReleases.Keys()
	if len(keys) == 0 || keys[0] != "L" {
		t.Errorf("OpenReleases keys = %v, want [L]", keys)
	}
}

func TestDefaultReleaseDetail_BindingsComplete(t *testing.T) {
	r := keymap.DefaultReleaseDetail()
	if len(r.ShortHelp()) == 0 || len(r.FullHelp()) == 0 {
		t.Error("help empty")
	}
}
