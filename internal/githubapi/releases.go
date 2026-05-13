package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// ReleasesClient is the interface for fetching GitHub Releases.
type ReleasesClient interface {
	ListReleases(ctx context.Context, repo domain.RepoRef, filter ListReleasesFilter) (ListReleasesResult, error)
	GetRelease(ctx context.Context, repo domain.RepoRef, releaseID int64) (domain.Release, error)
}

// ListReleasesFilter parameterises a releases query.
type ListReleasesFilter struct {
	Page        int
	PerPage     int
	IfNoneMatch string
}

// ListReleasesResult is the parsed response of ListReleases.
type ListReleasesResult struct {
	Releases    []domain.Release
	ETag        string
	NotModified bool
	NextPage    int
}

// ListReleases fetches a page of releases for the given repository.
// Releases are returned in published-date desc order by the GitHub API.
// Drafts are included only when the authenticated token has push access.
func (c *Client) ListReleases(ctx context.Context, repo domain.RepoRef, f ListReleasesFilter) (ListReleasesResult, error) {
	q := url.Values{}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(f.PerPage))
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases", c.baseFor(repo), repo.Owner, repo.Name)
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	headers := http.Header{}
	if f.IfNoneMatch != "" {
		headers.Set("If-None-Match", f.IfNoneMatch)
	}

	key := "GET " + endpoint + " inm=" + f.IfNoneMatch
	v, err, _ := c.sf.Do(key, func() (any, error) {
		return c.doListReleases(ctx, repo, endpoint, headers)
	})
	if err != nil {
		return ListReleasesResult{}, err
	}
	return v.(ListReleasesResult), nil
}

func (c *Client) doListReleases(ctx context.Context, repo domain.RepoRef, endpoint string, hdr http.Header) (ListReleasesResult, error) {
	resp, err := c.do(ctx, repo, http.MethodGet, endpoint, hdr)
	if err != nil {
		return ListReleasesResult{}, err
	}
	defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable

	if resp.StatusCode == http.StatusNotModified {
		etag := resp.Header.Get("ETag")
		if etag == "" {
			etag = hdr.Get("If-None-Match")
		}
		return ListReleasesResult{NotModified: true, ETag: etag}, nil
	}

	if err := c.checkStatus(resp, endpoint); err != nil {
		return ListReleasesResult{}, err
	}

	var raw []releasePayload
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ListReleasesResult{}, fmt.Errorf("github api: decode releases: %w", err)
	}
	out := ListReleasesResult{
		ETag:     resp.Header.Get("ETag"),
		NextPage: parseNextPage(resp.Header.Get("Link")),
		Releases: make([]domain.Release, 0, len(raw)),
	}
	for _, p := range raw {
		out.Releases = append(out.Releases, p.toDomain())
	}
	return out, nil
}

// GetRelease fetches a single release by ID.
func (c *Client) GetRelease(ctx context.Context, repo domain.RepoRef, releaseID int64) (domain.Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/%d", c.baseFor(repo), repo.Owner, repo.Name, releaseID)
	key := "GET " + endpoint
	v, err, _ := c.sf.Do(key, func() (any, error) {
		resp, err := c.do(ctx, repo, http.MethodGet, endpoint, nil)
		if err != nil {
			return domain.Release{}, err
		}
		defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable
		if err := c.checkStatus(resp, endpoint); err != nil {
			return domain.Release{}, err
		}
		var p releasePayload
		if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
			return domain.Release{}, fmt.Errorf("github api: decode release: %w", err)
		}
		return p.toDomain(), nil
	})
	if err != nil {
		return domain.Release{}, err
	}
	return v.(domain.Release), nil
}

// ----- payload types -----

type releasePayload struct {
	ID          int64                 `json:"id"`
	TagName     string                `json:"tag_name"`
	Name        string                `json:"name"`
	Body        string                `json:"body"`
	Draft       bool                  `json:"draft"`
	Prerelease  bool                  `json:"prerelease"`
	CreatedAt   time.Time             `json:"created_at"`
	PublishedAt *time.Time            `json:"published_at"`
	Author      releaseAuthorPayload  `json:"author"`
	HTMLURL     string                `json:"html_url"`
	TarballURL  string                `json:"tarball_url"`
	ZipballURL  string                `json:"zipball_url"`
	Assets      []releaseAssetPayload `json:"assets"`
}

func (p releasePayload) toDomain() domain.Release {
	assets := make([]domain.ReleaseAsset, 0, len(p.Assets))
	for _, a := range p.Assets {
		assets = append(assets, a.toDomain())
	}
	return domain.Release{
		ID:          p.ID,
		TagName:     p.TagName,
		Name:        p.Name,
		Body:        p.Body,
		Draft:       p.Draft,
		Prerelease:  p.Prerelease,
		CreatedAt:   p.CreatedAt,
		PublishedAt: nilIfZero(p.PublishedAt),
		Author:      p.Author.toDomain(),
		URL:         p.HTMLURL,
		TarballURL:  p.TarballURL,
		ZipballURL:  p.ZipballURL,
		Assets:      assets,
	}
}

type releaseAuthorPayload struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

func (p releaseAuthorPayload) toDomain() domain.ReleaseAuthor {
	return domain.ReleaseAuthor{
		Login:     p.Login,
		AvatarURL: p.AvatarURL,
		HTMLURL:   p.HTMLURL,
	}
}

type releaseAssetPayload struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Label         string    `json:"label"`
	ContentType   string    `json:"content_type"`
	Size          int64     `json:"size"`
	DownloadCount int       `json:"download_count"`
	State         string    `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	BrowserURL    string    `json:"browser_download_url"`
}

func (p releaseAssetPayload) toDomain() domain.ReleaseAsset {
	return domain.ReleaseAsset{
		ID:            p.ID,
		Name:          p.Name,
		Label:         p.Label,
		ContentType:   p.ContentType,
		Size:          p.Size,
		DownloadCount: p.DownloadCount,
		State:         p.State,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
		BrowserURL:    p.BrowserURL,
	}
}

// Compile-time interface check.
var _ ReleasesClient = (*Client)(nil)
