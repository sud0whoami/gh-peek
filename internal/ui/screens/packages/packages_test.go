package packages

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
	results []githubapi.ListPackagesResult
	errs    []error
}

func (f *fakeClient) push(r githubapi.ListPackagesResult, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	f.errs = append(f.errs, err)
}

func (f *fakeClient) ListPackages(_ context.Context, _ domain.RepoRef, _ githubapi.ListPackagesFilter) (githubapi.ListPackagesResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.results) == 0 {
		return githubapi.ListPackagesResult{}, nil
	}
	r := f.results[0]
	e := f.errs[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
		f.errs = f.errs[1:]
	}
	return r, e
}

func (f *fakeClient) ListPackageVersions(context.Context, domain.RepoRef, domain.Package, githubapi.ListPackageVersionsFilter) (githubapi.ListPackageVersionsResult, error) {
	return githubapi.ListPackageVersionsResult{}, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var fixedNow = time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

func now() time.Time { return fixedNow }

func repoRef() domain.RepoRef {
	return domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"}
}

func makePackage(id int64, name string, pt domain.PackageType, vis string, vCount int, updatedAgo time.Duration) domain.Package {
	return domain.Package{
		ID:           id,
		Name:         name,
		Type:         pt,
		Visibility:   vis,
		Owner:        domain.PackageOwner{Login: "octo", Type: "Organization"},
		Repository:   &domain.PackageRepoRef{Name: "demo", FullName: "octo/demo"},
		URL:          "https://github.com/orgs/octo/packages/" + string(pt) + "/package/" + name,
		CreatedAt:    fixedNow.Add(-2 * updatedAgo),
		UpdatedAt:    fixedNow.Add(-updatedAgo),
		VersionCount: vCount,
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
		if _, ok := msg.(PackagesLoadedMsg); ok {
			break
		}
	}
	return m
}

func TestInitialLoadReady(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "demo-pkg", domain.PackageTypeContainer, "private", 3, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "demo-pkg") {
		t.Errorf("view missing name: %q", v)
	}
	if !strings.Contains(v, "container") {
		t.Errorf("view missing type: %q", v)
	}
	if !strings.Contains(v, "private") {
		t.Errorf("view missing visibility: %q", v)
	}
}

func TestEmptyState(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: nil}, nil)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "No packages published") {
		t.Errorf("view missing empty message: %q", v)
	}
}

func TestErrorState(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{}, errors.New("boom"))
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "boom") {
		t.Errorf("view missing error: %q", v)
	}
}

func TestMissingScopeRendersHint(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{}, githubapi.ErrMissingPackagesScope)
	m := drainInit(t, newModel(t, fc))
	v := m.View().Content
	if !strings.Contains(v, "read:packages") {
		t.Errorf("view should hint missing scope: %q", v)
	}
}

func TestEnterEmitsOpenPackageMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(11, "alpha", domain.PackageTypeContainer, "private", 1, time.Hour),
		makePackage(22, "beta", domain.PackageTypeNPM, "public", 2, 2*time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter returned nil cmd")
	}
	op, ok := cmd().(OpenPackageMsg)
	if !ok {
		t.Fatalf("expected OpenPackageMsg, got %T", cmd())
	}
	if op.PackageID != 22 || op.Package.Name != "beta" {
		t.Errorf("open msg = %+v", op)
	}
}

func TestBackToRunsMsg(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "x", domain.PackageTypeContainer, "private", 1, time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	for _, k := range []string{"esc", "b", "P"} {
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
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "x", domain.PackageTypeContainer, "private", 1, time.Hour),
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
	if !strings.Contains(msg.URL, "/x") {
		t.Errorf("URL = %q", msg.URL)
	}
}

func TestSearchFilter(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "alpha-img", domain.PackageTypeContainer, "private", 1, time.Hour),
		makePackage(2, "beta-pkg", domain.PackageTypeNPM, "public", 1, 2*time.Hour),
	}}, nil)
	m := drainInit(t, newModel(t, fc))
	if got := m.VisiblePackageCount(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(*Model)
	if !m.IsSearching() {
		t.Fatal("expected searching")
	}
	for _, r := range "alpha" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	if got := m.VisiblePackageCount(); got != 1 {
		t.Errorf("filtered count = %d, want 1", got)
	}
}

func TestRefreshFetchesAgain(t *testing.T) {
	fc := &fakeClient{}
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "x", domain.PackageTypeContainer, "private", 1, time.Hour),
	}}, nil)
	fc.push(githubapi.ListPackagesResult{Packages: []domain.Package{
		makePackage(1, "x", domain.PackageTypeContainer, "private", 1, time.Hour),
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
