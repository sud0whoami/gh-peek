package pkgscreen

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

type fakeClient struct {
	mu       sync.Mutex
	calls    int
	versions []domain.PackageVersion
	err      error
}

func (f *fakeClient) ListPackages(context.Context, domain.RepoRef, githubapi.ListPackagesFilter) (githubapi.ListPackagesResult, error) {
	return githubapi.ListPackagesResult{}, nil
}

func (f *fakeClient) ListPackageVersions(context.Context, domain.RepoRef, domain.Package, githubapi.ListPackageVersionsFilter) (githubapi.ListPackageVersionsResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return githubapi.ListPackageVersionsResult{}, f.err
	}
	return githubapi.ListPackageVersionsResult{Versions: f.versions, ETag: `W/"v"`}, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func samplePackage() domain.Package {
	return domain.Package{
		ID:           1001,
		Name:         "demo-img",
		Type:         domain.PackageTypeContainer,
		Visibility:   "private",
		Owner:        domain.PackageOwner{Login: "octo", Type: "Organization"},
		Repository:   &domain.PackageRepoRef{Name: "demo", FullName: "octo/demo"},
		URL:          "https://github.com/orgs/octo/packages/container/package/demo-img",
		UpdatedAt:    fixedNow.Add(-time.Hour),
		VersionCount: 3,
	}
}

func sampleVersions() []domain.PackageVersion {
	return []domain.PackageVersion{
		{
			ID:             5001,
			Name:           "sha256:aaaa",
			PackageHTMLURL: "https://github.com/orgs/octo/packages/container/demo-img/5001",
			CreatedAt:      fixedNow.Add(-2 * time.Hour),
			Metadata:       domain.PackageVersionMetadata{PackageType: domain.PackageTypeContainer, ContainerTags: []string{"latest", "v1"}},
		},
		{
			ID:             5002,
			Name:           "sha256:bbbb",
			PackageHTMLURL: "https://github.com/orgs/octo/packages/container/demo-img/5002",
			CreatedAt:      fixedNow.Add(-48 * time.Hour),
			Metadata:       domain.PackageVersionMetadata{PackageType: domain.PackageTypeContainer, ContainerTags: []string{"v0.9"}},
		},
	}
}

func newModel(t *testing.T, fc *fakeClient) *Model {
	t.Helper()
	return New(Params{
		Repo:         domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		PackageID:    1001,
		Initial:      samplePackage(),
		Client:       fc,
		Now:          now,
		Width:        120,
		Height:       30,
		AutoRefresh:  false,
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
		if _, ok := msg.(VersionsLoadedMsg); ok {
			break
		}
	}
	return m
}

func TestRendersHeaderFromInitial(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "demo-img") {
		t.Errorf("missing name: %q", v)
	}
	if !strings.Contains(v, "container") {
		t.Errorf("missing type: %q", v)
	}
	if !strings.Contains(v, "private") {
		t.Errorf("missing visibility: %q", v)
	}
	if !strings.Contains(v, "octo/demo") {
		t.Errorf("missing repo: %q", v)
	}
	if !strings.Contains(v, "sha256:aaaa") {
		t.Errorf("missing version: %q", v)
	}
	if !strings.Contains(v, "latest") {
		t.Errorf("missing container tags: %q", v)
	}
}

func TestEmptyVersions(t *testing.T) {
	fc := &fakeClient{versions: nil}
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "no versions") {
		t.Errorf("missing empty state: %q", v)
	}
}

func TestErrorState(t *testing.T) {
	fc := &fakeClient{err: errors.New("kaboom")}
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "kaboom") {
		t.Errorf("missing error: %q", v)
	}
}

func TestMissingScopeRendersHint(t *testing.T) {
	fc := &fakeClient{err: githubapi.ErrMissingPackagesScope}
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "read:packages") {
		t.Errorf("missing scope hint: %q", v)
	}
}

func TestOpenInBrowserOnVersion(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatal("o nil cmd")
	}
	msg, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if !strings.HasSuffix(msg.URL, "/5001") {
		t.Errorf("URL = %q", msg.URL)
	}
}

func TestOpenPackageURLWithCapitalO(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'O', Text: "O"})
	if cmd == nil {
		t.Fatal("O nil cmd")
	}
	msg, ok := cmd().(OpenInBrowserMsg)
	if !ok {
		t.Fatalf("expected OpenInBrowserMsg, got %T", cmd())
	}
	if msg.URL != samplePackage().URL {
		t.Errorf("URL = %q want %q", msg.URL, samplePackage().URL)
	}
}

func TestBackMsg(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	for _, k := range []string{"esc", "b"} {
		_, cmd := m.Update(tea.KeyPressMsg{Text: k})
		if cmd == nil {
			t.Fatalf("%q nil cmd", k)
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Fatalf("%q wrong msg", k)
		}
	}
}

func TestRefresh(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	before := fc.callCount()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r nil cmd")
	}
	_ = cmd()
	if got := fc.callCount(); got <= before {
		t.Errorf("expected another fetch; before=%d after=%d", before, got)
	}
}

func TestCursorMovement(t *testing.T) {
	fc := &fakeClient{versions: sampleVersions()}
	m := drainInit(t, newModel(t, fc))
	if m.Cursor() != 0 {
		t.Fatalf("cursor=%d", m.Cursor())
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	if m.Cursor() != 1 {
		t.Errorf("cursor after j=%d, want 1", m.Cursor())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(*Model)
	if m.Cursor() != 0 {
		t.Errorf("cursor after k=%d, want 0", m.Cursor())
	}
}
