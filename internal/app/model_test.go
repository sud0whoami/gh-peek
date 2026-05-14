package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/ui/layout"
)

func TestNewModel_Init(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	// Calling Init must not panic.
	_ = m.Init()
}

func TestModel_QuitOnQ(t *testing.T) {
	m := New()
	key := tea.KeyPressMsg{Code: 'q', Text: "q"}
	_, cmd := m.Update(key)
	if cmd == nil {
		t.Fatal("Update on 'q' returned nil cmd; expected a quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestModel_WindowSize_CapsContentWidth(t *testing.T) {
	m := New()
	_, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if m.frame.Content > layout.DefaultMaxWidth {
		t.Fatalf("expected content width ≤ %d, got %d", layout.DefaultMaxWidth, m.frame.Content)
	}
	if m.frame.Content != layout.DefaultMaxWidth {
		t.Fatalf("expected content width = %d for wide terminal, got %d", layout.DefaultMaxWidth, m.frame.Content)
	}
}

func TestModel_WindowSize_TooNarrow(t *testing.T) {
	m := New()
	_, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	if !m.frame.TooNarrow {
		t.Fatal("expected TooNarrow=true for 60-col terminal")
	}
	view := m.View()
	if !strings.Contains(view.Content, "too narrow") {
		t.Fatalf("expected 'too narrow' in view, got: %q", view.Content)
	}
}
