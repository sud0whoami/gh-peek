package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// ErrMissingPackagesScope is returned when a Packages API request fails
// because the authenticated token lacks the read:packages scope.
// errors.Is(err, ErrForbidden) is also true for these errors.
var ErrMissingPackagesScope = errors.New("github api: missing read:packages scope")

// PackagesClient is the interface for fetching GitHub Packages.
type PackagesClient interface {
	ListPackages(ctx context.Context, repo domain.RepoRef, filter ListPackagesFilter) (ListPackagesResult, error)
	ListPackageVersions(ctx context.Context, repo domain.RepoRef, pkg domain.Package, filter ListPackageVersionsFilter) (ListPackageVersionsResult, error)
}

// ListPackagesFilter parameterises a packages query.
//
// PackageTypes: empty means all six supported types (fanned out in parallel).
// Visibility: optional GitHub visibility filter ("public"|"private"|"internal").
// IfNoneMatch: per-type ETag map for conditional requests.
type ListPackagesFilter struct {
	PackageTypes []domain.PackageType
	Visibility   string
	Page         int
	PerPage      int
	IfNoneMatch  map[domain.PackageType]string
}

// ListPackagesResult aggregates per-type fan-out responses.
//
// Packages: union of all per-type responses, filtered to repo.Name.
// ETags: per-type ETag returned (or echoed when 304).
// NotModified: true only when every fanned-out request returned 304.
// NextPages: per-type next-page numbers parsed from Link headers.
type ListPackagesResult struct {
	Packages    []domain.Package
	ETags       map[domain.PackageType]string
	NotModified bool
	NextPages   map[domain.PackageType]int
}

// ListPackageVersionsFilter parameterises a package versions query.
type ListPackageVersionsFilter struct {
	Page        int
	PerPage     int
	IfNoneMatch string
}

// ListPackageVersionsResult is the parsed response of ListPackageVersions.
type ListPackageVersionsResult struct {
	Versions    []domain.PackageVersion
	ETag        string
	NotModified bool
	NextPage    int
}

// ListPackages fetches packages attached to repo across the requested
// package types in parallel, then filters the union by repo.Name.
//
// Owner-type routing: when repo.OwnerType is "User" the /users/{owner}
// endpoint is used; otherwise (org or unknown) /orgs/{owner} is tried
// first and a 404 falls back to /users/{owner}.
func (c *Client) ListPackages(ctx context.Context, repo domain.RepoRef, f ListPackagesFilter) (ListPackagesResult, error) {
	types := f.PackageTypes
	if len(types) == 0 {
		types = domain.AllPackageTypes()
	}

	type perType struct {
		pkgs        []domain.Package
		etag        string
		notModified bool
		next        int
		err         error
	}
	results := make([]perType, len(types))

	var wg sync.WaitGroup
	for i, pt := range types {
		i, pt := i, pt
		wg.Add(1)
		go func() {
			defer wg.Done()
			pkgs, etag, nm, next, err := c.listPackagesForType(ctx, repo, pt, f)
			results[i] = perType{pkgs: pkgs, etag: etag, notModified: nm, next: next, err: err}
		}()
	}
	wg.Wait()

	out := ListPackagesResult{
		ETags:       make(map[domain.PackageType]string, len(types)),
		NextPages:   make(map[domain.PackageType]int, len(types)),
		NotModified: true,
	}
	for i, pt := range types {
		r := results[i]
		if r.err != nil {
			// Tolerate per-type 404s when the owner has no packages of a
			// given type. Surface anything else as a hard error.
			if errors.Is(r.err, ErrNotFound) {
				continue
			}
			return ListPackagesResult{}, r.err
		}
		if r.etag != "" {
			out.ETags[pt] = r.etag
		}
		if r.next > 0 {
			out.NextPages[pt] = r.next
		}
		if !r.notModified {
			out.NotModified = false
		}
		for _, p := range r.pkgs {
			// Drop packages without a repository link, and any whose
			// repository name does not match. Org-level packages with no
			// repo binding (e.g. some container packages) would otherwise
			// leak into every repo in the org.
			if p.Repository == nil || p.Repository.Name != repo.Name {
				continue
			}
			out.Packages = append(out.Packages, p)
		}
	}
	// If every type errored or returned no data and none reported fresh
	// data, NotModified stays true only if at least one 304 was seen.
	if len(out.ETags) == 0 && len(out.Packages) == 0 {
		out.NotModified = false
	}
	return out, nil
}

// listPackagesForType issues a single typed listing, with org→user
// fallback when OwnerType is unknown.
func (c *Client) listPackagesForType(ctx context.Context, repo domain.RepoRef, pt domain.PackageType, f ListPackagesFilter) ([]domain.Package, string, bool, int, error) {
	pkgs, etag, nm, next, err := c.fetchPackagesPage(ctx, repo, ownerSegment(repo, false), pt, f)
	if err == nil {
		return pkgs, etag, nm, next, nil
	}
	// Unknown owner type: try /users on 404 from /orgs.
	if repo.OwnerType == "" && errors.Is(err, ErrNotFound) {
		return c.fetchPackagesPage(ctx, repo, "users", pt, f)
	}
	return nil, "", false, 0, err
}

// ownerSegment returns "orgs" or "users" for the listing path.
//
// Default is "orgs"; explicit "User" yields "users". A forceUsers flag
// (used by retry path) overrides.
func ownerSegment(repo domain.RepoRef, forceUsers bool) string {
	if forceUsers || strings.EqualFold(repo.OwnerType, "User") {
		return "users"
	}
	return "orgs"
}

func (c *Client) fetchPackagesPage(ctx context.Context, repo domain.RepoRef, segment string, pt domain.PackageType, f ListPackagesFilter) ([]domain.Package, string, bool, int, error) {
	q := url.Values{}
	q.Set("package_type", string(pt))
	if f.Visibility != "" {
		q.Set("visibility", f.Visibility)
	}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(f.PerPage))
	}
	endpoint := fmt.Sprintf("%s/%s/%s/packages?%s", c.baseFor(repo), segment, repo.Owner, q.Encode())

	headers := http.Header{}
	if inm := f.IfNoneMatch[pt]; inm != "" {
		headers.Set("If-None-Match", inm)
	}

	key := "GET " + endpoint + " inm=" + headers.Get("If-None-Match")
	v, err, _ := c.sf.Do(key, func() (any, error) {
		resp, err := c.do(ctx, repo, http.MethodGet, endpoint, headers)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable

		if resp.StatusCode == http.StatusNotModified {
			etag := resp.Header.Get("ETag")
			if etag == "" {
				etag = headers.Get("If-None-Match")
			}
			return packagesPage{etag: etag, notModified: true}, nil
		}
		if err := c.checkPackagesStatus(resp, endpoint); err != nil {
			return nil, err
		}
		var raw []packagePayload
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, fmt.Errorf("github api: decode packages: %w", err)
		}
		out := packagesPage{
			etag: resp.Header.Get("ETag"),
			next: parseNextPage(resp.Header.Get("Link")),
			pkgs: make([]domain.Package, 0, len(raw)),
		}
		for _, p := range raw {
			out.pkgs = append(out.pkgs, p.toDomain())
		}
		return out, nil
	})
	if err != nil {
		return nil, "", false, 0, err
	}
	page := v.(packagesPage)
	return page.pkgs, page.etag, page.notModified, page.next, nil
}

type packagesPage struct {
	pkgs        []domain.Package
	etag        string
	notModified bool
	next        int
}

// ListPackageVersions fetches versions for a single package.
func (c *Client) ListPackageVersions(ctx context.Context, repo domain.RepoRef, pkg domain.Package, f ListPackageVersionsFilter) (ListPackageVersionsResult, error) {
	q := url.Values{}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(f.PerPage))
	}
	segment := ownerSegment(repo, false)
	if pkg.Owner.Type != "" {
		// Prefer the owner type carried on the package itself.
		if strings.EqualFold(pkg.Owner.Type, "User") {
			segment = "users"
		} else {
			segment = "orgs"
		}
	}
	endpoint := fmt.Sprintf("%s/%s/%s/packages/%s/%s/versions",
		c.baseFor(repo), segment, repo.Owner, pkg.Type, url.PathEscape(pkg.Name))
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	headers := http.Header{}
	if f.IfNoneMatch != "" {
		headers.Set("If-None-Match", f.IfNoneMatch)
	}

	key := "GET " + endpoint + " inm=" + f.IfNoneMatch
	v, err, _ := c.sf.Do(key, func() (any, error) {
		resp, err := c.do(ctx, repo, http.MethodGet, endpoint, headers)
		if err != nil {
			return ListPackageVersionsResult{}, err
		}
		defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable

		if resp.StatusCode == http.StatusNotModified {
			etag := resp.Header.Get("ETag")
			if etag == "" {
				etag = headers.Get("If-None-Match")
			}
			return ListPackageVersionsResult{NotModified: true, ETag: etag}, nil
		}
		if err := c.checkPackagesStatus(resp, endpoint); err != nil {
			return ListPackageVersionsResult{}, err
		}
		var raw []packageVersionPayload
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return ListPackageVersionsResult{}, fmt.Errorf("github api: decode package versions: %w", err)
		}
		out := ListPackageVersionsResult{
			ETag:     resp.Header.Get("ETag"),
			NextPage: parseNextPage(resp.Header.Get("Link")),
			Versions: make([]domain.PackageVersion, 0, len(raw)),
		}
		for _, p := range raw {
			out.Versions = append(out.Versions, p.toDomain(pkg))
		}
		return out, nil
	})
	if err != nil {
		return ListPackageVersionsResult{}, err
	}
	return v.(ListPackageVersionsResult), nil
}

// checkPackagesStatus wraps Client.checkStatus and promotes 403 responses
// that look like missing-scope failures to ErrMissingPackagesScope.
func (c *Client) checkPackagesStatus(resp *http.Response, endpoint string) error {
	err := c.checkStatus(resp, endpoint)
	if err == nil {
		return nil
	}
	if resp.StatusCode == http.StatusForbidden {
		// Heuristic: the response body or X-Accepted-OAuth-Scopes header
		// references read:packages.
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			haystack := strings.ToLower(apiErr.Message + " " + resp.Header.Get("X-Accepted-OAuth-Scopes"))
			if strings.Contains(haystack, "read:packages") {
				apiErr.wrapped = wrappedMissingScope{}
				return apiErr
			}
		}
	}
	return err
}

// wrappedMissingScope makes errors.Is(err, ErrMissingPackagesScope) AND
// errors.Is(err, ErrForbidden) both true.
type wrappedMissingScope struct{}

func (wrappedMissingScope) Error() string { return ErrMissingPackagesScope.Error() }
func (wrappedMissingScope) Is(target error) bool {
	return target == ErrMissingPackagesScope || target == ErrForbidden
}

// ----- payload types -----

type packagePayload struct {
	ID           int64                  `json:"id"`
	Name         string                 `json:"name"`
	PackageType  domain.PackageType     `json:"package_type"`
	Visibility   string                 `json:"visibility"`
	HTMLURL      string                 `json:"html_url"`
	URL          string                 `json:"url"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	VersionCount int                    `json:"version_count"`
	Owner        packageOwnerPayload    `json:"owner"`
	Repository   *packageRepoRefPayload `json:"repository"`
}

func (p packagePayload) toDomain() domain.Package {
	htmlURL := p.HTMLURL
	if htmlURL == "" {
		htmlURL = p.URL
	}
	return domain.Package{
		ID:           p.ID,
		Name:         p.Name,
		Type:         p.PackageType,
		Visibility:   p.Visibility,
		Owner:        p.Owner.toDomain(),
		Repository:   p.Repository.toDomain(),
		URL:          htmlURL,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
		VersionCount: p.VersionCount,
	}
}

type packageOwnerPayload struct {
	Login   string `json:"login"`
	Type    string `json:"type"`
	HTMLURL string `json:"html_url"`
}

func (p packageOwnerPayload) toDomain() domain.PackageOwner {
	return domain.PackageOwner{Login: p.Login, Type: p.Type, HTMLURL: p.HTMLURL}
}

type packageRepoRefPayload struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

func (p *packageRepoRefPayload) toDomain() *domain.PackageRepoRef {
	if p == nil {
		return nil
	}
	return &domain.PackageRepoRef{Name: p.Name, FullName: p.FullName}
}

type packageVersionPayload struct {
	ID        int64                         `json:"id"`
	Name      string                        `json:"name"`
	URL       string                        `json:"url"`
	HTMLURL   string                        `json:"html_url"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Metadata  *packageVersionMetadataLoader `json:"metadata"`
}

func (p packageVersionPayload) toDomain(pkg domain.Package) domain.PackageVersion {
	htmlURL := p.HTMLURL
	if htmlURL == "" {
		htmlURL = p.URL
	}
	meta := domain.PackageVersionMetadata{PackageType: pkg.Type}
	if p.Metadata != nil {
		meta.PackageType = pkg.Type
		if p.Metadata.PackageType != "" {
			meta.PackageType = p.Metadata.PackageType
		}
		if p.Metadata.Container != nil {
			meta.ContainerTags = append([]string{}, p.Metadata.Container.Tags...)
		}
	}
	return domain.PackageVersion{
		ID:             p.ID,
		Name:           p.Name,
		URL:            p.URL,
		PackageHTMLURL: htmlURL,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		Metadata:       meta,
	}
}

type packageVersionMetadataLoader struct {
	PackageType domain.PackageType           `json:"package_type"`
	Container   *packageVersionContainerMeta `json:"container"`
}

type packageVersionContainerMeta struct {
	Tags []string `json:"tags"`
}

// Compile-time interface check.
var _ PackagesClient = (*Client)(nil)
