package release

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sud0whoami/gh-peek/internal/domain"
	"github.com/sud0whoami/gh-peek/internal/githubapi"
)

var fixedNow = time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

func sampleRelease() domain.Release {
	pub := fixedNow.Add(-2 * time.Hour)
	return domain.Release{
		ID:          101,
		TagName:     "v1.2.0",
		Name:        "v1.2.0 — Bug fixes",
		Body:        "## What's changed\n\n- fix flaky test\n- improve docs\n",
		PublishedAt: &pub,
		Author:      domain.ReleaseAuthor{Login: "octo"},
		URL:         "https://github.com/octo/demo/releases/tag/v1.2.0",
		Assets: []domain.ReleaseAsset{
			{ID: 1, Name: "demo_linux_amd64.tar.gz", Size: 12345, DownloadCount: 7, BrowserURL: "https://example.com/dl/linux"},
			{ID: 2, Name: "demo_darwin_arm64.tar.gz", Size: 67890, DownloadCount: 3, BrowserURL: "https://example.com/dl/darwin"},
		},
	}
}

func newModelWithInitial(t *testing.T, rel domain.Release) *Model {
	t.Helper()
	return New(Params{
		Repo:         domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
		ReleaseID:    rel.ID,
		Initial:      rel,
		Client:       &realFake{},
		Now:          now,
		Width:        120,
		Height:       30,
		AutoRefresh:  false,
		TickInterval: time.Millisecond,
	})
}

// realFake implements githubapi.ReleasesClient with optional GetRelease error.
type realFake struct {
	errOnGet error
	rel      domain.Release
	mu       sync.Mutex
	calls    int
}

func (r *realFake) ListReleases(context.Context, domain.RepoRef, githubapi.ListReleasesFilter) (githubapi.ListReleasesResult, error) {
	return githubapi.ListReleasesResult{}, nil
}
func (r *realFake) GetRelease(context.Context, domain.RepoRef, int64) (domain.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.errOnGet != nil {
		return domain.Release{}, r.errOnGet
	}
	return r.rel, nil
}

func TestInitialRenderFromInitial(t *testing.T) {
	rel := sampleRelease()
	m := newModelWithInitial(t, rel)
	v := m.View().Content
	if !strings.Contains(v, "v1.2.0") {
		t.Errorf("view missing tag: %q", v)
	}
	if !strings.Contains(v, "fix flaky test") {
		t.Errorf("view missing body content: %q", v)
	}
	if !strings.Contains(v, "demo_linux_amd64.tar.gz") {
		t.Errorf("view missing asset name: %q", v)
	}
	if !strings.Contains(v, "octo") {
		t.Errorf("view missing author: %q", v)
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := newModelWithInitial(t, sampleRelease())
	if !m.FocusIsNotes() {
		t.Fatal("expected initial focus on notes")
	}
	next, _ := m.Update(tea.KeyPressMsg{Text: "tab"})
	m = next.(*Model)
	if !m.FocusIsAssets() {
		t.Fatal("expected focus on assets after tab")
	}
}

func TestEnterOnAssetEmitsOpenInBrowser(t *testing.T) {
	m := newModelWithInitial(t, sampleRelease())
	next, _ := m.Update(tea.KeyPressMsg{Text: "tab"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Text: "enter"})
	if cmd == nil {
		t.Fatal("enter returned nil cmd")
	}
	msg, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if msg.URL != "https://example.com/dl/darwin" {
		t.Errorf("URL = %q", msg.URL)
	}
}

func TestOpenInBrowserFromHeader(t *testing.T) {
	m := newModelWithInitial(t, sampleRelease())
	_, cmd := m.Update(tea.KeyPressMsg{Text: "o"})
	if cmd == nil {
		t.Fatal("o returned nil cmd")
	}
	msg, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if msg.URL != "https://github.com/octo/demo/releases/tag/v1.2.0" {
		t.Errorf("URL = %q", msg.URL)
	}
}

func TestBackMsg(t *testing.T) {
	m := newModelWithInitial(t, sampleRelease())
	for _, k := range []string{"esc", "b"} {
		_, cmd := m.Update(tea.KeyPressMsg{Text: k})
		if cmd == nil {
			t.Fatalf("%q returned nil cmd", k)
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Fatalf("%q did not emit BackMsg", k)
		}
	}
}

func TestNotesScrolling(t *testing.T) {
	m := newModelWithInitial(t, sampleRelease())
	if m.NotesScroll() != 0 {
		t.Fatalf("initial scroll = %d", m.NotesScroll())
	}
	next, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next.(*Model)
	if m.NotesScroll() != 1 {
		t.Errorf("scroll after j = %d, want 1", m.NotesScroll())
	}
	next, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	m = next.(*Model)
	if m.NotesScroll() != 0 {
		t.Errorf("scroll after k = %d, want 0", m.NotesScroll())
	}
}

func TestLoadErrorFromEmptyInitial(t *testing.T) {
	fake := &realFake{errOnGet: errors.New("kaboom")}
	m := New(Params{
		Repo:         domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
		ReleaseID:    99,
		Client:       fake,
		Now:          now,
		Width:        100,
		Height:       30,
		TickInterval: time.Millisecond,
	})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)
	v := m.View().Content
	if !strings.Contains(v, "kaboom") && !strings.Contains(v, "failed") {
		t.Errorf("expected error in view, got: %q", v)
	}
}
