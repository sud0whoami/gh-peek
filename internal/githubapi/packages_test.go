package githubapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

func samplePackagesJSONForType(pt domain.PackageType, repoName string) string {
	return fmt.Sprintf(`[
      {
        "id": 1001,
        "name": "demo-pkg-%s",
        "package_type": "%s",
        "visibility": "private",
        "html_url": "https://github.com/orgs/octo/packages/%s/package/demo-pkg-%s",
        "url": "https://api.github.com/orgs/octo/packages/%s/demo-pkg-%s",
        "created_at": "2025-01-01T10:00:00Z",
        "updated_at": "2025-02-01T10:00:00Z",
        "version_count": 3,
        "owner": {"login": "octo", "type": "Organization", "html_url": "https://github.com/octo"},
        "repository": {"name": %q, "full_name": "octo/%s"}
      },
      {
        "id": 1002,
        "name": "other-repo-pkg-%s",
        "package_type": "%s",
        "visibility": "public",
        "html_url": "https://github.com/orgs/octo/packages/%s/package/other-repo-pkg-%s",
        "created_at": "2025-01-01T10:00:00Z",
        "updated_at": "2025-02-01T10:00:00Z",
        "version_count": 1,
        "owner": {"login": "octo", "type": "Organization"},
        "repository": {"name": "other-repo", "full_name": "octo/other-repo"}
      }
    ]`, pt, pt, pt, pt, pt, pt, repoName, repoName, pt, pt, pt, pt)
}

// packagesMux returns a handler that serves a per-type listing for the
// org segment and 404 for everything else. Requests for unknown types
// receive an empty JSON array.
func packagesMux(t *testing.T, repoName string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/orgs/octo/packages") {
			http.NotFound(w, r)
			return
		}
		pt := domain.PackageType(r.URL.Query().Get("package_type"))
		if pt == "" {
			http.Error(w, `{"message":"package_type required"}`, http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `W/"etag-`+string(pt)+`"`)
		// Only container + npm in this fixture; other types return [].
		switch pt {
		case domain.PackageTypeContainer, domain.PackageTypeNPM:
			_, _ = io.WriteString(w, samplePackagesJSONForType(pt, repoName))
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	})
}

func TestListPackages_FanOutAndRepoFilter(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, packagesMux(t, "demo"))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		ListPackagesFilter{})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	// Two types had results, but only the repo "demo" rows survive
	// the client-side repo filter (one per type).
	if len(res.Packages) != 2 {
		t.Fatalf("packages = %d, want 2 (filtered to repo)", len(res.Packages))
	}
	names := map[string]bool{}
	for _, p := range res.Packages {
		names[p.Name] = true
		if p.Repository == nil || p.Repository.Name != "demo" {
			t.Errorf("repo filter leaked: %+v", p)
		}
	}
	if !names["demo-pkg-container"] || !names["demo-pkg-npm"] {
		t.Errorf("expected container+npm packages, got %v", names)
	}
	// One request per type — six total fan-out calls.
	got := rec.snapshot()
	seenTypes := map[string]int{}
	for _, r := range got {
		v, _ := url.ParseQuery(r.Query)
		seenTypes[v.Get("package_type")]++
	}
	if len(seenTypes) != 6 {
		t.Errorf("expected 6 distinct package_type calls, got %d (%v)", len(seenTypes), seenTypes)
	}
	// ETags returned for the two non-empty types.
	if res.ETags[domain.PackageTypeContainer] == "" || res.ETags[domain.PackageTypeNPM] == "" {
		t.Errorf("missing ETag, got %v", res.ETags)
	}
	if res.NotModified {
		t.Error("NotModified should be false when fresh data was returned")
	}
}

func TestListPackages_AllTypesNotModified(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	inm := map[domain.PackageType]string{}
	for _, pt := range domain.AllPackageTypes() {
		inm[pt] = `W/"prev-` + string(pt) + `"`
	}
	res, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		ListPackagesFilter{IfNoneMatch: inm})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if !res.NotModified {
		t.Error("expected NotModified=true when all 6 types returned 304")
	}
	if len(res.Packages) != 0 {
		t.Errorf("packages should be empty, got %d", len(res.Packages))
	}
	if len(res.ETags) != len(domain.AllPackageTypes()) {
		t.Errorf("expected echoed ETags for all types, got %d", len(res.ETags))
	}
}

func TestListPackages_OrgToUserFallback(t *testing.T) {
	t.Parallel()
	var orgHits, userHits int32
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/orgs/"):
			atomic.AddInt32(&orgHits, 1)
			http.NotFound(w, r)
		case strings.HasPrefix(r.URL.Path, "/users/"):
			atomic.AddInt32(&userHits, 1)
			pt := domain.PackageType(r.URL.Query().Get("package_type"))
			w.Header().Set("Content-Type", "application/json")
			if pt == domain.PackageTypeContainer {
				_, _ = io.WriteString(w, samplePackagesJSONForType(pt, "demo"))
			} else {
				_, _ = io.WriteString(w, `[]`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	// OwnerType empty → org first, fall back to users on 404.
	res, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo"},
		ListPackagesFilter{})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(res.Packages) == 0 {
		t.Error("expected fallback to surface packages from /users")
	}
	if atomic.LoadInt32(&orgHits) == 0 || atomic.LoadInt32(&userHits) == 0 {
		t.Errorf("expected both org and user hits, got org=%d user=%d", orgHits, userHits)
	}
}

func TestListPackages_DropsPackagesWithoutRepository(t *testing.T) {
	t.Parallel()
	// One container package binds to "demo"; a second has repository:null
	// (e.g. an org-level container with no repo link). The null one must
	// not leak into any specific repo's view.
	body := `[
      {
        "id": 2001,
        "name": "demo-pkg-container",
        "package_type": "container",
        "visibility": "private",
        "html_url": "https://github.com/orgs/octo/packages/container/package/demo-pkg-container",
        "created_at": "2025-01-01T10:00:00Z",
        "updated_at": "2025-02-01T10:00:00Z",
        "version_count": 1,
        "owner": {"login": "octo", "type": "Organization"},
        "repository": {"name": "demo", "full_name": "octo/demo"}
      },
      {
        "id": 2002,
        "name": "orphan-container",
        "package_type": "container",
        "visibility": "private",
        "html_url": "https://github.com/orgs/octo/packages/container/package/orphan-container",
        "created_at": "2025-01-01T10:00:00Z",
        "updated_at": "2025-02-01T10:00:00Z",
        "version_count": 1,
        "owner": {"login": "octo", "type": "Organization"},
        "repository": null
      }
    ]`
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pt := domain.PackageType(r.URL.Query().Get("package_type"))
		w.Header().Set("Content-Type", "application/json")
		if pt == domain.PackageTypeContainer {
			_, _ = io.WriteString(w, body)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		ListPackagesFilter{PackageTypes: []domain.PackageType{domain.PackageTypeContainer}})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(res.Packages) != 1 {
		t.Fatalf("packages = %d, want 1 (orphan must be dropped); got: %+v", len(res.Packages), res.Packages)
	}
	if res.Packages[0].Name != "demo-pkg-container" {
		t.Errorf("kept the wrong package: %+v", res.Packages[0])
	}
}

// TestListPackages_FollowsPagination guards against a regression where
// the listing only fetched page 1 and silently dropped repo-matching
// packages that happened to live on later pages.
func TestListPackages_FollowsPagination(t *testing.T) {
	t.Parallel()
	// Page 1: two packages, one belongs to "demo".
	// Page 2: two packages, one belongs to "demo" — this is the one
	//         that used to disappear.
	page1 := `[
      {"id":1,"name":"p1","package_type":"container","visibility":"private","html_url":"","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","version_count":1,"owner":{"login":"octo","type":"Organization"},"repository":{"name":"demo","full_name":"octo/demo"}},
      {"id":2,"name":"p2","package_type":"container","visibility":"private","html_url":"","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","version_count":1,"owner":{"login":"octo","type":"Organization"},"repository":{"name":"other","full_name":"octo/other"}}
    ]`
	page2 := `[
      {"id":3,"name":"p3","package_type":"container","visibility":"private","html_url":"","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","version_count":1,"owner":{"login":"octo","type":"Organization"},"repository":{"name":"demo","full_name":"octo/demo"}},
      {"id":4,"name":"p4","package_type":"container","visibility":"private","html_url":"","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","version_count":1,"owner":{"login":"octo","type":"Organization"},"repository":{"name":"other","full_name":"octo/other"}}
    ]`
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if domain.PackageType(r.URL.Query().Get("package_type")) != domain.PackageTypeContainer {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			// Advertise a next page via Link header.
			w.Header().Set("Link", `<`+r.URL.Path+`?package_type=container&page=2>; rel="next"`)
			_, _ = io.WriteString(w, page1)
		case "2":
			_, _ = io.WriteString(w, page2)
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		ListPackagesFilter{PackageTypes: []domain.PackageType{domain.PackageTypeContainer}})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(res.Packages) != 2 {
		t.Fatalf("packages = %d, want 2 (p1 + p3 across two pages); got: %+v", len(res.Packages), res.Packages)
	}
	got := map[string]bool{}
	for _, p := range res.Packages {
		got[p.Name] = true
	}
	if !got["p1"] || !got["p3"] {
		t.Errorf("missing expected packages; got %v", got)
	}
}

func TestListPackages_MissingScope(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accepted-OAuth-Scopes", "repo, read:packages")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration. Token is missing the read:packages scope."}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	_, err := c.ListPackages(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		ListPackagesFilter{})
	if !errors.Is(err, ErrMissingPackagesScope) {
		t.Fatalf("err = %v, want ErrMissingPackagesScope", err)
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("err should also satisfy ErrForbidden, got %v", err)
	}
}

func TestListPackages_SingleflightDedupe(t *testing.T) {
	t.Parallel()
	var hits int32
	gate := make(chan struct{})
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-gate
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	repo := domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.ListPackages(context.Background(), repo,
				ListPackagesFilter{PackageTypes: []domain.PackageType{domain.PackageTypeContainer}})
		}()
	}
	// Give goroutines time to enter singleflight before releasing.
	for i := 0; i < 50 && atomic.LoadInt32(&hits) < 1; i++ {
	}
	close(gate)
	wg.Wait()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 server hit (deduped), got %d", got)
	}
}

const sampleVersionsJSON = `[
  {
    "id": 5001,
    "name": "sha256:abcd",
    "url": "https://api.github.com/orgs/octo/packages/container/demo-pkg-container/versions/5001",
    "html_url": "https://github.com/orgs/octo/packages/container/demo-pkg-container/5001",
    "created_at": "2025-03-01T10:00:00Z",
    "updated_at": "2025-03-01T10:00:00Z",
    "metadata": {"package_type": "container", "container": {"tags": ["latest", "v1.2.0"]}}
  },
  {
    "id": 5002,
    "name": "sha256:efgh",
    "url": "https://api.github.com/orgs/octo/packages/container/demo-pkg-container/versions/5002",
    "html_url": "https://github.com/orgs/octo/packages/container/demo-pkg-container/5002",
    "created_at": "2025-02-01T10:00:00Z",
    "updated_at": "2025-02-01T10:00:00Z",
    "metadata": {"package_type": "container", "container": {"tags": ["v1.1.0"]}}
  }
]`

func TestListPackageVersions_DecodesContainerTags(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `W/"v-etag-1"`)
		_, _ = io.WriteString(w, sampleVersionsJSON)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	pkg := domain.Package{
		Name:  "demo-pkg-container",
		Type:  domain.PackageTypeContainer,
		Owner: domain.PackageOwner{Login: "octo", Type: "Organization"},
	}
	res, err := c.ListPackageVersions(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "octo", Name: "demo", OwnerType: "Organization"},
		pkg, ListPackageVersionsFilter{})
	if err != nil {
		t.Fatalf("ListPackageVersions: %v", err)
	}
	if len(res.Versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(res.Versions))
	}
	if res.ETag != `W/"v-etag-1"` {
		t.Errorf("ETag = %q", res.ETag)
	}
	v0 := res.Versions[0]
	if v0.ID != 5001 || v0.Name != "sha256:abcd" {
		t.Errorf("v0 = %+v", v0)
	}
	if v0.Metadata.PackageType != domain.PackageTypeContainer {
		t.Errorf("v0 metadata.PackageType = %q", v0.Metadata.PackageType)
	}
	if got := v0.Metadata.ContainerTags; len(got) != 2 || got[0] != "latest" || got[1] != "v1.2.0" {
		t.Errorf("v0 container tags = %v", got)
	}
	if v0.PackageHTMLURL == "" {
		t.Error("v0 PackageHTMLURL empty")
	}
	got := rec.snapshot()
	if !strings.Contains(got[0].Path, "/orgs/octo/packages/container/demo-pkg-container/versions") {
		t.Errorf("path = %q", got[0].Path)
	}
}
