package releases

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

type fakeClient struct {
	mu      sync.Mutex
	calls   int
	results []githubapi.ListReleasesResult
	errs    []error
}

func (f *fakeClient) push(r githubapi.ListReleasesResult, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	f.errs = append(f.errs, err)
}

func (f *fakeClient) ListReleases(_ context.Context, _ domain.RepoRef, _ githubapi.ListReleasesFilter) (githubapi.ListReleasesResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.results) == 0 {
		return githubapi.ListReleasesResult{}, nil
	}
	r := f.results[0]
	e := f.errs[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
		f.errs = f.errs[1:]
	}
	return r, e
}

func (f *fakeClient) GetRelease(context.Context, domain.RepoRef, int64) (domain.Release, error) {
	return domain.Release{}, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var fixedNow = time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

func repoRef() domain.RepoRef {
	return domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"}
}

func makeRelease(id int64, tag, name, author string, draft, pre bool, publishedAgo time.Duration) domain.Release {
	pub := fixedNow.Add(-publishedAgo)
	var p *time.Time
	if !draft {
		p = &pub
	}
	return domain.Release{
		ID:          id,
		TagName:     tag,
		Name:        name,
		Body:        "release body for " + tag,
		Draft:       draft,
		Prerelease:  pre,
		CreatedAt:   pub,
		PublishedAt: p,
		Author:      domain.ReleaseAuthor{Login: author},
		URL:         "https://example.com/releases/" + tag,
	}
}

func newModel(t *testing.T, fc *fakeClient) *Model {
	t.Helper()
	return New(Params{
		Repo:         repoRef(),
		Client:       fc,
		Now:          now,
		Width:        100,
		Height:       24,
		AutoRefresh:  true,
		TickInterval: time.Millisecond,
	})
}

func drainInit(t *testing.T, m *Model) *Model {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil")
	}
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(*Model)
		if _, ok := msg.(ReleasesLoadedMsg); ok {
			break
		}
	}
	return m
}

func TestInitialLoadReady(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1.0.0", "First", "octo", false, false, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "v1.0.0") {
		t.Errorf("view missing tag: %q", v)
	}
	if !strings.Contains(v, "latest") {
		t.Errorf("view missing latest badge: %q", v)
	}
	if !strings.Contains(v, "octo") {
		t.Errorf("view missing author: %q", v)
	}
}

func TestEmptyState(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: nil}, nil)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "No releases for this repository") {
		t.Errorf("view missing empty message: %q", v)
	}
}

func TestErrorState(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{}, errors.New("boom"))
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "boom") {
		t.Errorf("view missing error: %q", v)
	}
}

func TestBadgesPreDraft(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(3, "v2.0.0-rc.1", "RC", "octo", false, true, time.Hour),
		makeRelease(2, "v2.0.0", "Two", "octo", false, false, 2*time.Hour),
		makeRelease(4, "draft-x", "Draft", "octo", true, false, 0),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "pre") {
		t.Errorf("view missing pre badge: %q", v)
	}
	if !strings.Contains(v, "draft") {
		t.Errorf("view missing draft badge: %q", v)
	}
	if !strings.Contains(v, "latest") {
		t.Errorf("view missing latest badge: %q", v)
	}
}

func TestEnterEmitsOpenReleaseMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(11, "v1", "a", "octo", false, false, time.Hour),
		makeRelease(22, "v2", "b", "octo", false, false, 2*time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter returned nil cmd")
	}
	msg := cmd()
	op, ok := msg.(OpenReleaseMsg)
	if !ok {
		t.Fatalf("expected OpenReleaseMsg, got %T", msg)
	}
	if op.ReleaseID != 22 {
		t.Errorf("ReleaseID = %d, want 22", op.ReleaseID)
	}
	if op.Release.TagName != "v2" {
		t.Errorf("Release.TagName = %q, want v2", op.Release.TagName)
	}
}

func TestBackToRunsMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1", "", "octo", false, false, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	for _, k := range []string{"esc", "b", "L"} {
		_, cmd := m.Update(tea.KeyPressMsg{Text: k})
		if cmd == nil {
			t.Fatalf("%q returned nil cmd", k)
		}
		if _, ok := cmd().(BackToRunsMsg); !ok {
			t.Fatalf("%q did not emit BackToRunsMsg", k)
		}
	}
}

func TestOpenInBrowserMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1", "", "octo", false, false, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatal("o returned nil cmd")
	}
	msg, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if msg.URL != "https://example.com/releases/v1" {
		t.Errorf("URL = %q", msg.URL)
	}
}

func TestSearchFilter(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1.0.0", "alpha", "octo", false, false, time.Hour),
		makeRelease(2, "v2.0.0", "beta", "octo", false, false, 2*time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	if got := m.VisibleReleaseCount(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	// Enter search mode and type "alpha".
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	if !m.IsSearching() {
		t.Fatal("expected searching")
	}
	for _, r := range "alpha" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	if got := m.VisibleReleaseCount(); got != 1 {
		t.Errorf("filtered count = %d, want 1", got)
	}
}

func TestRefreshFetchesAgain(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1", "", "octo", false, false, time.Hour),
	}}, nil)
	fc.push(githubapi.ListReleasesResult{Releases: []domain.Release{
		makeRelease(1, "v1", "", "octo", false, false, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	before := fc.callCount()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned nil cmd")
	}
	_ = cmd()
	if got := fc.callCount(); got <= before {
		t.Errorf("expected another fetch; before=%d after=%d", before, got)
	}
}
