package styles_test

import (
	"strings"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/testutil"
	"github.com/sud0whoami/gh-peek/internal/ui/styles"
)

func TestDefaultTheme_BadgeForEachStatus(t *testing.T) {
	testutil.LockColorProfile(t)
	th := styles.DefaultTheme()

	cases := []struct {
		status domain.SemanticStatus
		marker string
	}{
		{domain.StatusSuccess, "✓"},
		{domain.StatusFailure, "✗"},
		{domain.StatusRunning, "●"},
		{domain.StatusPending, "○"},
		{domain.StatusCancelled, "⊘"},
		{domain.StatusSkipped, "−"},
		{domain.StatusUnknown, "?"},
	}
	for _, c := range cases {
		got := th.Badge(c.status)
		if got == "" {
			t.Errorf("Badge(%q) returned empty string", c.status)
			continue
		}
		if !strings.Contains(got, c.marker) {
			t.Errorf("Badge(%q) = %q; want it to contain %q", c.status, got, c.marker)
		}
		if !strings.Contains(got, string(c.status)) {
			t.Errorf("Badge(%q) = %q; want it to contain status text %q", c.status, got, c.status)
		}
	}
}

func TestTheme_HelperHelpers(t *testing.T) {
	testutil.LockColorProfile(t)
	th := styles.DefaultTheme()

	for name, fn := range map[string]func(string) string{
		"Muted":       th.Muted,
		"Help":        th.Help,
		"ErrorBanner": th.ErrorBanner,
	} {
		out := fn("hello")
		if !strings.Contains(out, "hello") {
			t.Errorf("%s: expected output to contain payload, got %q", name, out)
		}
	}
}
