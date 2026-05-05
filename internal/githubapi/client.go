// Package githubapi provides a GitHub Actions REST API client for
// runs, jobs, and logs.
//
// The Client is safe for concurrent use. Identical concurrent GET
// requests are deduplicated via singleflight. Callers handle
// caching/ETag policy by passing IfNoneMatch on subsequent calls.
package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/auth"
	"golang.org/x/sync/singleflight"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// Version is the User-Agent suffix used by default.
const Version = "0.0.0-dev"

// maxLogBytes caps how many bytes DownloadJobLog will read into memory.
const maxLogBytes int64 = 50 * 1024 * 1024

// jobsPageCap bounds how many pages of jobs ListJobs will fetch.
const jobsPageCap = 5

// Sentinel errors. Wrap with %w so errors.Is works.
var (
	ErrUnauthorized   = errors.New("github api: unauthorized")
	ErrForbidden      = errors.New("github api: forbidden")
	ErrNotFound       = errors.New("github api: not found")
	ErrUnprocessable  = errors.New("github api: unprocessable entity")
	ErrRateLimited    = errors.New("github api: rate limited")
	ErrLogTooLarge    = errors.New("github api: log exceeds size cap")
	ErrUnexpectedBody = errors.New("github api: unexpected response body")
)

// APIError carries HTTP-level detail surfaced from the GitHub API.
type APIError struct {
	Status     int
	Message    string
	URL        string
	RetryAfter time.Duration
	wrapped    error
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("github api: %s (status %d)", http.StatusText(e.Status), e.Status)
	}
	return fmt.Sprintf("github api: %s (status %d)", e.Message, e.Status)
}

func (e *APIError) Unwrap() error { return e.wrapped }

// ActionsClient is the public interface implemented by Client. Other
// packages depend on this interface, not on Client directly.
type ActionsClient interface {
	ListRuns(ctx context.Context, repo domain.RepoRef, filter ListRunsFilter) (ListRunsResult, error)
	GetRun(ctx context.Context, repo domain.RepoRef, runID int64) (domain.WorkflowRun, error)
	ListJobs(ctx context.Context, repo domain.RepoRef, runID int64) ([]domain.WorkflowJob, error)
	DownloadJobLog(ctx context.Context, repo domain.RepoRef, jobID int64) ([]byte, error)
}

// ListRunsFilter parameterises a runs query.
type ListRunsFilter struct {
	Branch      string
	Event       string
	Status      string
	HeadSHA     string
	Page        int
	PerPage     int
	IfNoneMatch string
}

// ListRunsResult is the parsed response of ListRuns.
type ListRunsResult struct {
	Runs        []domain.WorkflowRun
	TotalCount  int
	ETag        string
	NotModified bool
	NextPage    int
}

// Option configures a Client.
type Option func(*Client)

// Client implements ActionsClient.
type Client struct {
	httpClient *http.Client
	transport  http.RoundTripper
	baseURL    string // when non-empty, overrides host-derived base
	tokenFunc  func(host string) (string, error)
	userAgent  string
	now        func() time.Time
	// sf deduplicates identical concurrent GETs. Note: singleflight.Group.Do
	// runs the closure from the first caller and shares the result with all
	// waiters. If the first caller's context is cancelled, waiters whose
	// contexts are still valid are also cancelled. All current call sites pass
	// context.Background() so this cannot trigger in practice; if a
	// program-level shutdown context is ever plumbed in, revisit and switch to
	// DoChan / DoAndForget to isolate per-caller cancellation.
	sf singleflight.Group
}

// New constructs a Client.
func New(opts ...Option) *Client {
	c := &Client{
		transport: http.DefaultTransport,
		tokenFunc: func(host string) (string, error) {
			tok, _ := auth.TokenForHost(host)
			return tok, nil
		},
		userAgent: "gh-peek/" + Version,
		now:       time.Now,
	}
	for _, o := range opts {
		o(c)
	}
	c.httpClient = &http.Client{
		Transport: c.transport,
		Timeout:   30 * time.Second,
		// Strip Authorization on cross-host redirects (signed log URLs).
		CheckRedirect: stripAuthOnRedirect,
	}
	return c
}

// WithTransport overrides the underlying http.RoundTripper. Useful for
// httptest.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) {
		if rt != nil {
			c.transport = rt
		}
	}
}

// WithBaseURL overrides the host-derived base URL for all requests.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithTokenFunc overrides the default auth.TokenForHost resolver.
func WithTokenFunc(f func(host string) (string, error)) Option {
	return func(c *Client) {
		if f != nil {
			c.tokenFunc = f
		}
	}
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithClock overrides the clock used to compute Retry-After from
// X-RateLimit-Reset.
func WithClock(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// stripAuthOnRedirect removes the Authorization header when the
// redirect target is on a different host than the original request.
// This is required for log downloads, which 302 to a signed S3 URL
// that rejects GitHub bearer tokens.
func stripAuthOnRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
	}
	return nil
}

// baseFor returns the API base URL for the given repo, honoring
// WithBaseURL overrides.
func (c *Client) baseFor(repo domain.RepoRef) string {
	if c.baseURL != "" {
		return c.baseURL
	}
	if repo.Host == "" || repo.Host == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + repo.Host + "/api/v3"
}

// ----- Public API methods -----

// ListRuns fetches a page of workflow runs.
func (c *Client) ListRuns(ctx context.Context, repo domain.RepoRef, f ListRunsFilter) (ListRunsResult, error) {
	q := url.Values{}
	if f.Branch != "" {
		q.Set("branch", f.Branch)
	}
	if f.Event != "" {
		q.Set("event", f.Event)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.HeadSHA != "" {
		q.Set("head_sha", f.HeadSHA)
	}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(f.PerPage))
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs", c.baseFor(repo), repo.Owner, repo.Name)
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	headers := http.Header{}
	if f.IfNoneMatch != "" {
		headers.Set("If-None-Match", f.IfNoneMatch)
	}

	key := "GET " + endpoint + " inm=" + f.IfNoneMatch
	v, err, _ := c.sf.Do(key, func() (any, error) {
		return c.doListRuns(ctx, repo, endpoint, headers)
	})
	if err != nil {
		return ListRunsResult{}, err
	}
	return v.(ListRunsResult), nil
}

func (c *Client) doListRuns(ctx context.Context, repo domain.RepoRef, endpoint string, hdr http.Header) (ListRunsResult, error) {
	resp, err := c.do(ctx, repo, http.MethodGet, endpoint, hdr)
	if err != nil {
		return ListRunsResult{}, err
	}
	defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable

	if resp.StatusCode == http.StatusNotModified {
		etag := resp.Header.Get("ETag")
		if etag == "" {
			etag = hdr.Get("If-None-Match")
		}
		return ListRunsResult{NotModified: true, ETag: etag}, nil
	}

	if err := c.checkStatus(resp, endpoint); err != nil {
		return ListRunsResult{}, err
	}

	var raw struct {
		TotalCount   int          `json:"total_count"`
		WorkflowRuns []runPayload `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ListRunsResult{}, fmt.Errorf("github api: decode runs: %w", err)
	}
	out := ListRunsResult{
		TotalCount: raw.TotalCount,
		ETag:       resp.Header.Get("ETag"),
		NextPage:   parseNextPage(resp.Header.Get("Link")),
		Runs:       make([]domain.WorkflowRun, 0, len(raw.WorkflowRuns)),
	}
	for _, p := range raw.WorkflowRuns {
		out.Runs = append(out.Runs, p.toDomain())
	}
	return out, nil
}

// GetRun fetches a single workflow run by ID.
func (c *Client) GetRun(ctx context.Context, repo domain.RepoRef, runID int64) (domain.WorkflowRun, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d", c.baseFor(repo), repo.Owner, repo.Name, runID)
	key := "GET " + endpoint
	v, err, _ := c.sf.Do(key, func() (any, error) {
		resp, err := c.do(ctx, repo, http.MethodGet, endpoint, nil)
		if err != nil {
			return domain.WorkflowRun{}, err
		}
		defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable
		if err := c.checkStatus(resp, endpoint); err != nil {
			return domain.WorkflowRun{}, err
		}
		var p runPayload
		if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
			return domain.WorkflowRun{}, fmt.Errorf("github api: decode run: %w", err)
		}
		return p.toDomain(), nil
	})
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	return v.(domain.WorkflowRun), nil
}

// ListJobs returns the jobs for a run, paginating up to jobsPageCap pages.
func (c *Client) ListJobs(ctx context.Context, repo domain.RepoRef, runID int64) ([]domain.WorkflowJob, error) {
	base := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/jobs", c.baseFor(repo), repo.Owner, repo.Name, runID)
	all := []domain.WorkflowJob{}
	page := 1
	for fetched := 0; fetched < jobsPageCap; fetched++ {
		q := url.Values{}
		q.Set("per_page", "100")
		if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}
		endpoint := base + "?" + q.Encode()
		jobs, next, err := c.fetchJobsPage(ctx, repo, endpoint)
		if err != nil {
			return nil, err
		}
		all = append(all, jobs...)
		if next == 0 {
			break
		}
		page = next
	}
	return all, nil
}

func (c *Client) fetchJobsPage(ctx context.Context, repo domain.RepoRef, endpoint string) ([]domain.WorkflowJob, int, error) {
	resp, err := c.do(ctx, repo, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable
	if err := c.checkStatus(resp, endpoint); err != nil {
		return nil, 0, err
	}
	var raw struct {
		TotalCount int          `json:"total_count"`
		Jobs       []jobPayload `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, 0, fmt.Errorf("github api: decode jobs: %w", err)
	}
	out := make([]domain.WorkflowJob, 0, len(raw.Jobs))
	for _, p := range raw.Jobs {
		out = append(out, p.toDomain())
	}
	return out, parseNextPage(resp.Header.Get("Link")), nil
}

// DownloadJobLog fetches the raw log bytes for a single job. The
// endpoint redirects to a signed URL; Authorization is stripped on the
// cross-host redirect. The body is capped at maxLogBytes; on overflow,
// returns the partial bytes wrapped in ErrLogTooLarge.
func (c *Client) DownloadJobLog(ctx context.Context, repo domain.RepoRef, jobID int64) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/jobs/%d/logs", c.baseFor(repo), repo.Owner, repo.Name, jobID)
	key := "GET " + endpoint
	v, err, _ := c.sf.Do(key, func() (any, error) {
		resp, err := c.do(ctx, repo, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close() //nolint:errcheck // body close on read path; error is unactionable
		if err := c.checkStatus(resp, endpoint); err != nil {
			return nil, err
		}
		// Cap body size at maxLogBytes + 1 so we can detect overflow.
		lim := io.LimitReader(resp.Body, maxLogBytes+1)
		body, readErr := io.ReadAll(lim)
		if readErr != nil {
			return nil, fmt.Errorf("github api: read log: %w", readErr)
		}
		if int64(len(body)) > maxLogBytes {
			return body[:maxLogBytes], fmt.Errorf("%w: job %d", ErrLogTooLarge, jobID)
		}
		return body, nil
	})
	// v can be a non-nil partial body even when err is non-nil (overflow case).
	var body []byte
	if v != nil {
		body, _ = v.([]byte)
	}
	return body, err
}

// ----- internals -----

func (c *Client) do(ctx context.Context, repo domain.RepoRef, method, urlStr string, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("github api: build request: %w", err)
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.userAgent)

	if c.tokenFunc != nil {
		tok, terr := c.tokenFunc(repo.Host)
		if terr != nil {
			return nil, fmt.Errorf("github api: resolve token: %w", terr)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Do not include the URL with token-ish content; do not include token.
		return nil, fmt.Errorf("github api: %s %s: %w", method, redactURL(urlStr), err)
	}
	return resp, nil
}

// checkStatus inspects the response and returns a typed error for
// non-2xx codes. The body is consumed to extract the GitHub message.
// On success (2xx), it returns nil and leaves resp.Body untouched.
func (c *Client) checkStatus(resp *http.Response, endpoint string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	msg := extractMessage(body)
	apiErr := &APIError{
		Status:  resp.StatusCode,
		Message: msg,
		URL:     redactURL(endpoint),
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		apiErr.wrapped = ErrUnauthorized
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			apiErr.wrapped = wrappedRateLimited{}
			apiErr.RetryAfter = c.parseRetryAfter(resp.Header)
		} else {
			apiErr.wrapped = ErrForbidden
		}
	case http.StatusNotFound:
		apiErr.wrapped = ErrNotFound
	case http.StatusUnprocessableEntity:
		apiErr.wrapped = ErrUnprocessable
	case http.StatusTooManyRequests:
		apiErr.wrapped = ErrRateLimited
		apiErr.RetryAfter = c.parseRetryAfter(resp.Header)
	}
	return apiErr
}

// wrappedRateLimited makes errors.Is(err, ErrRateLimited) AND
// errors.Is(err, ErrForbidden) both true for a 403+rate-limited error.
type wrappedRateLimited struct{}

func (wrappedRateLimited) Error() string { return ErrRateLimited.Error() }
func (wrappedRateLimited) Is(target error) bool {
	return target == ErrRateLimited || target == ErrForbidden
}

func (c *Client) parseRetryAfter(h http.Header) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			d := t.Sub(c.now())
			if d > 0 {
				return d
			}
		}
	}
	if v := strings.TrimSpace(h.Get("X-RateLimit-Reset")); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			d := time.Unix(ts, 0).Sub(c.now())
			if d > 0 {
				return d
			}
		}
	}
	return 0
}

// extractMessage tries to pull a GitHub error message from a JSON body
// like {"message":"..."}. Falls back to a trimmed body string.
func extractMessage(body []byte) string {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return ""
	}
	var p struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Message != "" {
		return p.Message
	}
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}

// redactURL strips userinfo from URLs (defense in depth; GitHub URLs
// don't carry it, but log download redirects are signed and may be
// noisy in errors).
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	// Drop query (signed URLs include keys); keep path.
	u.RawQuery = ""
	return u.String()
}

// linkNextRE matches the "next" rel of a Link header and captures the URL.
var linkNextRE = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// parseNextPage extracts the page query param from the rel="next" Link
// header value. Returns 0 when absent or malformed.
func parseNextPage(linkHeader string) int {
	if linkHeader == "" {
		return 0
	}
	m := linkNextRE.FindStringSubmatch(linkHeader)
	if len(m) != 2 {
		return 0
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return 0
	}
	pStr := u.Query().Get("page")
	if pStr == "" {
		return 0
	}
	p, err := strconv.Atoi(pStr)
	if err != nil {
		return 0
	}
	return p
}

// ----- payload types -----

type runPayload struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	DisplayTitle string     `json:"display_title"`
	Event        string     `json:"event"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	RunAttempt   int        `json:"run_attempt"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
	HTMLURL      string     `json:"html_url"`
}

func (p runPayload) toDomain() domain.WorkflowRun {
	return domain.WorkflowRun{
		ID:           p.ID,
		Name:         p.Name,
		WorkflowName: p.Name,
		DisplayTitle: p.DisplayTitle,
		Event:        p.Event,
		HeadBranch:   p.HeadBranch,
		HeadSHA:      p.HeadSHA,
		Status:       p.Status,
		Conclusion:   p.Conclusion,
		Attempt:      p.RunAttempt,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
		StartedAt:    nilIfZero(p.RunStartedAt),
		URL:          p.HTMLURL,
	}
}

type jobPayload struct {
	ID              int64         `json:"id"`
	RunID           int64         `json:"run_id"`
	Name            string        `json:"name"`
	Status          string        `json:"status"`
	Conclusion      string        `json:"conclusion"`
	StartedAt       *time.Time    `json:"started_at"`
	CompletedAt     *time.Time    `json:"completed_at"`
	HTMLURL         string        `json:"html_url"`
	WorkflowName    string        `json:"workflow_name"`
	HeadBranch      string        `json:"head_branch"`
	RunnerName      string        `json:"runner_name"`
	RunnerGroupName string        `json:"runner_group_name"`
	Labels          []string      `json:"labels"`
	Steps           []stepPayload `json:"steps"`
}

func (p jobPayload) toDomain() domain.WorkflowJob {
	steps := make([]domain.WorkflowStep, 0, len(p.Steps))
	for _, s := range p.Steps {
		steps = append(steps, s.toDomain())
	}
	return domain.WorkflowJob{
		ID:           p.ID,
		RunID:        p.RunID,
		Name:         p.Name,
		Status:       p.Status,
		Conclusion:   p.Conclusion,
		StartedAt:    nilIfZero(p.StartedAt),
		CompletedAt:  nilIfZero(p.CompletedAt),
		WorkflowName: p.WorkflowName,
		HeadBranch:   p.HeadBranch,
		RunnerName:   p.RunnerName,
		RunnerGroup:  p.RunnerGroupName,
		Labels:       append([]string(nil), p.Labels...),
		Steps:        steps,
		URL:          p.HTMLURL,
	}
}

type stepPayload struct {
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

func (p stepPayload) toDomain() domain.WorkflowStep {
	return domain.WorkflowStep{
		Number:      p.Number,
		Name:        p.Name,
		Status:      p.Status,
		Conclusion:  p.Conclusion,
		StartedAt:   nilIfZero(p.StartedAt),
		CompletedAt: nilIfZero(p.CompletedAt),
	}
}

func nilIfZero(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

// Compile-time interface check.
var _ ActionsClient = (*Client)(nil)
