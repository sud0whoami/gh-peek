package githubapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

const sampleReleasesJSON = `[
  {
    "id": 101,
    "tag_name": "v1.2.0",
    "name": "v1.2.0 — Bug fixes",
    "body": "## What's changed\n- fix flaky test",
    "draft": false,
    "prerelease": false,
    "created_at": "2025-01-01T10:00:00Z",
    "published_at": "2025-01-01T11:00:00Z",
    "author": {"login": "octo", "avatar_url": "https://example.com/a.png", "html_url": "https://github.com/octo"},
    "html_url": "https://github.com/octo/demo/releases/tag/v1.2.0",
    "tarball_url": "https://api.github.com/repos/octo/demo/tarball/v1.2.0",
    "zipball_url": "https://api.github.com/repos/octo/demo/zipball/v1.2.0",
    "assets": [
      {
        "id": 9001,
        "name": "demo_linux_amd64.tar.gz",
        "label": "Linux amd64",
        "content_type": "application/gzip",
        "size": 12345,
        "download_count": 7,
        "state": "uploaded",
        "created_at": "2025-01-01T11:00:00Z",
        "updated_at": "2025-01-01T11:00:00Z",
        "browser_download_url": "https://example.com/dl/demo_linux_amd64.tar.gz"
      }
    ]
  },
  {
    "id": 102,
    "tag_name": "v1.3.0-rc.1",
    "name": "",
    "body": "",
    "draft": false,
    "prerelease": true,
    "created_at": "2025-01-02T10:00:00Z",
    "published_at": "2025-01-02T11:00:00Z",
    "author": {"login": "octo"},
    "html_url": "https://github.com/octo/demo/releases/tag/v1.3.0-rc.1",
    "assets": []
  },
  {
    "id": 103,
    "tag_name": "v1.4.0",
    "name": "Draft for next minor",
    "body": "",
    "draft": true,
    "prerelease": false,
    "created_at": "2025-01-03T10:00:00Z",
    "published_at": null,
    "author": {"login": "octo"},
    "html_url": "https://github.com/octo/demo/releases/tag/untagged-xyz",
    "assets": []
  }
]`

func TestListReleases_QueryParams(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	_, err := c.ListReleases(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListReleasesFilter{Page: 2, PerPage: 50})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("got %d requests, want 1", len(got))
	}
	if got[0].Path != "/repos/o/r/releases" {
		t.Fatalf("path = %q", got[0].Path)
	}
	req, _ := http.NewRequest("GET", "?"+got[0].Query, nil)
	v := req.URL.Query()
	if v.Get("page") != "2" || v.Get("per_page") != "50" {
		t.Fatalf("query = %v", v)
	}
}

func TestListReleases_DecodesPayload(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"etag-1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, sampleReleasesJSON)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListReleases(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListReleasesFilter{})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if res.ETag != `W/"etag-1"` {
		t.Fatalf("ETag = %q", res.ETag)
	}
	if len(res.Releases) != 3 {
		t.Fatalf("releases count = %d, want 3", len(res.Releases))
	}
	r0 := res.Releases[0]
	if r0.TagName != "v1.2.0" || r0.Name != "v1.2.0 — Bug fixes" {
		t.Errorf("release[0] tag/name = %q/%q", r0.TagName, r0.Name)
	}
	if r0.Draft || r0.Prerelease {
		t.Errorf("release[0] draft/prerelease = %v/%v", r0.Draft, r0.Prerelease)
	}
	if r0.PublishedAt == nil {
		t.Error("release[0] PublishedAt should not be nil")
	}
	if r0.Author.Login != "octo" {
		t.Errorf("release[0] author.login = %q", r0.Author.Login)
	}
	if len(r0.Assets) != 1 {
		t.Fatalf("release[0] assets = %d, want 1", len(r0.Assets))
	}
	a := r0.Assets[0]
	if a.Name != "demo_linux_amd64.tar.gz" || a.Size != 12345 || a.DownloadCount != 7 {
		t.Errorf("asset[0] = %+v", a)
	}
	if a.BrowserURL == "" {
		t.Error("asset[0] BrowserURL empty")
	}
	if !res.Releases[1].Prerelease {
		t.Error("release[1] should be prerelease")
	}
	r2 := res.Releases[2]
	if !r2.Draft {
		t.Error("release[2] should be draft")
	}
	if r2.PublishedAt != nil {
		t.Error("release[2] PublishedAt should be nil for draft")
	}
}

func TestListReleases_NotModified(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `W/"prev"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListReleases(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListReleasesFilter{IfNoneMatch: `W/"prev"`})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if !res.NotModified {
		t.Fatal("expected NotModified")
	}
	if res.ETag != `W/"prev"` {
		t.Errorf("ETag echo = %q", res.ETag)
	}
}

func TestListReleases_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	_, err := c.ListReleases(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListReleasesFilter{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetRelease_DecodesPayload(t *testing.T) {
	t.Parallel()
	const single = `{
      "id": 101,
      "tag_name": "v1.2.0",
      "name": "v1.2.0",
      "body": "notes",
      "draft": false,
      "prerelease": false,
      "created_at": "2025-01-01T10:00:00Z",
      "published_at": "2025-01-01T11:00:00Z",
      "author": {"login": "octo"},
      "html_url": "https://example.com/r",
      "assets": []
    }`
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, single)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	rel, err := c.GetRelease(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 101)
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if rel.ID != 101 || rel.TagName != "v1.2.0" {
		t.Errorf("rel = %+v", rel)
	}
	got := rec.snapshot()
	if !strings.HasSuffix(got[0].Path, "/releases/101") {
		t.Errorf("path = %q", got[0].Path)
	}
}
