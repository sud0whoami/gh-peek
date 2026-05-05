package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
