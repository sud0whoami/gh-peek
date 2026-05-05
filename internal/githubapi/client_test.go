package githubapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sud0whoami/gh-peek/internal/domain"
)

// ----- Base URL composition -----

func TestBaseFor_GitHubDotCom(t *testing.T) {
	t.Parallel()
	c := New(WithTokenFunc(emptyTokenFunc))
	got := c.baseFor(domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"})
	want := "https://api.github.com"
	if got != want {
		t.Fatalf("baseFor github.com = %q, want %q", got, want)
	}
}

func TestBaseFor_GHES(t *testing.T) {
	t.Parallel()
	c := New(WithTokenFunc(emptyTokenFunc))
	got := c.baseFor(domain.RepoRef{Host: "ghe.example.com", Owner: "o", Name: "r"})
	want := "https://ghe.example.com/api/v3"
	if got != want {
		t.Fatalf("baseFor GHES = %q, want %q", got, want)
	}
}

func TestBaseFor_OverrideViaWithBaseURL(t *testing.T) {
	t.Parallel()
	c := New(WithTokenFunc(emptyTokenFunc), WithBaseURL("http://127.0.0.1:9999"))
	got := c.baseFor(domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"})
	if got != "http://127.0.0.1:9999" {
		t.Fatalf("baseFor with override = %q", got)
	}
}

// ----- ListRuns query params -----

func TestListRuns_QueryParams(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	_, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{
			Branch:  "feat/x",
			Event:   "push",
			Status:  "in_progress",
			HeadSHA: "abc123",
			Page:    2,
			PerPage: 50,
		})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("got %d requests, want 1", len(got))
	}
	if got[0].Path != "/repos/o/r/actions/runs" {
		t.Fatalf("path = %q", got[0].Path)
	}
	q := got[0].Header.Get("Accept")
	if q != "application/vnd.github+json" {
		t.Fatalf("Accept = %q", q)
	}
	if got[0].Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Fatalf("missing X-GitHub-Api-Version")
	}
	if got[0].Header.Get("User-Agent") == "" {
		t.Fatalf("missing User-Agent")
	}
	// Parse query values.
	req, _ := http.NewRequest("GET", "?"+got[0].Query, nil)
	v := req.URL.Query()
	wantPairs := map[string]string{
		"branch":   "feat/x",
		"event":    "push",
		"status":   "in_progress",
		"head_sha": "abc123",
		"page":     "2",
		"per_page": "50",
	}
	for k, want := range wantPairs {
		if v.Get(k) != want {
			t.Errorf("query[%s] = %q, want %q", k, v.Get(k), want)
		}
	}
}

// ----- ListRuns response mapping -----

const sampleRunsJSON = `{
  "total_count": 2,
  "workflow_runs": [
    {
      "id": 101,
      "name": "CI",
      "display_title": "Fix bug",
      "event": "push",
      "head_branch": "main",
      "head_sha": "deadbeef",
      "status": "completed",
      "conclusion": "success",
      "run_attempt": 1,
      "created_at": "2025-01-02T03:04:05Z",
      "updated_at": "2025-01-02T03:10:05Z",
      "run_started_at": "2025-01-02T03:04:10Z",
      "html_url": "https://github.com/o/r/actions/runs/101"
    },
    {
      "id": 102,
      "name": "CI",
      "display_title": "Add feature",
      "event": "pull_request",
      "head_branch": "feat/x",
      "head_sha": "cafebabe",
      "status": "in_progress",
      "conclusion": null,
      "run_attempt": 2,
      "created_at": "2025-01-03T04:05:06Z",
      "updated_at": "2025-01-03T04:05:30Z",
      "run_started_at": null,
      "html_url": "https://github.com/o/r/actions/runs/102"
    }
  ]
}`

func TestListRuns_ResponseMapping(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, sampleRunsJSON)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	res, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if res.TotalCount != 2 {
		t.Fatalf("TotalCount = %d", res.TotalCount)
	}
	if len(res.Runs) != 2 {
		t.Fatalf("len(Runs) = %d", len(res.Runs))
	}
	r0 := res.Runs[0]
	if r0.ID != 101 || r0.Name != "CI" || r0.Status != "completed" || r0.Conclusion != "success" {
		t.Fatalf("run0 = %+v", r0)
	}
	if r0.HeadSHA != "deadbeef" || r0.Attempt != 1 {
		t.Fatalf("run0 head/attempt mismatch: %+v", r0)
	}
	if r0.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt zero")
	}
	if r0.StartedAt == nil {
		t.Fatalf("run0 StartedAt nil")
	}
	if res.Runs[1].StartedAt != nil {
		t.Fatalf("run1 StartedAt should be nil")
	}
	if res.NextPage != 0 {
		t.Fatalf("NextPage = %d, want 0", res.NextPage)
	}
}

// ----- Link header → NextPage -----

func TestListRuns_NextPageFromLink(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/x?page=2&per_page=30>; rel="next", <https://api.github.com/x?page=10>; rel="last"`)
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	res, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if res.NextPage != 2 {
		t.Fatalf("NextPage = %d, want 2", res.NextPage)
	}
}

// ----- ETag round-trip / 304 -----

func TestListRuns_ETagRoundTrip(t *testing.T) {
	t.Parallel()
	const etag = `W/"abc"`
	var hits int32
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	first, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("ListRuns 1: %v", err)
	}
	if first.ETag != etag {
		t.Fatalf("first ETag = %q", first.ETag)
	}
	if first.NotModified {
		t.Fatalf("first should not be NotModified")
	}

	second, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{IfNoneMatch: first.ETag})
	if err != nil {
		t.Fatalf("ListRuns 2: %v", err)
	}
	if !second.NotModified {
		t.Fatalf("second NotModified = false")
	}
	if second.Runs != nil {
		t.Fatalf("second Runs should be nil, got %v", second.Runs)
	}
	if second.ETag != etag {
		t.Fatalf("second ETag = %q", second.ETag)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("server hits = %d", hits)
	}
}

// ----- GetRun -----

func TestGetRun_Happy(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions/runs/42") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":42,"name":"CI","status":"completed","conclusion":"success","head_sha":"x","run_attempt":1,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:01Z"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	run, err := c.GetRun(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 42)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.ID != 42 {
		t.Fatalf("ID = %d", run.ID)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	_, err := c.GetRun(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ----- ListJobs (incl. steps + pagination) -----

const sampleJobsPage = `{
  "total_count": %d,
  "jobs": [
    {
      "id": 1,
      "run_id": 99,
      "name": "build",
      "status": "completed",
      "conclusion": "success",
      "started_at": "2025-01-01T00:00:00Z",
      "completed_at": "2025-01-01T00:01:00Z",
      "html_url": "https://github.com/o/r/actions/runs/99/jobs/1",
      "labels": ["ubuntu-latest"],
      "runner_name": "GitHub Actions 2",
      "runner_group_name": "GitHub Actions",
      "steps": [
        {"number":1,"name":"Set up","status":"completed","conclusion":"success","started_at":"2025-01-01T00:00:01Z","completed_at":"2025-01-01T00:00:05Z"},
        {"number":2,"name":"Build","status":"completed","conclusion":"success","started_at":"2025-01-01T00:00:06Z","completed_at":"2025-01-01T00:00:59Z"}
      ]
    }
  ]
}`

func TestListJobs_StepMapping(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, sampleJobsPage, 1)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	jobs, err := c.ListJobs(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 99)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d", len(jobs))
	}
	j := jobs[0]
	if j.RunID != 99 || j.Name != "build" || j.RunnerName != "GitHub Actions 2" || j.RunnerGroup != "GitHub Actions" {
		t.Fatalf("job mismatch: %+v", j)
	}
	if len(j.Labels) != 1 || j.Labels[0] != "ubuntu-latest" {
		t.Fatalf("labels: %v", j.Labels)
	}
	if len(j.Steps) != 2 {
		t.Fatalf("steps len = %d", len(j.Steps))
	}
	s := j.Steps[1]
	if s.Number != 2 || s.Name != "Build" || s.Status != "completed" || s.Conclusion != "success" {
		t.Fatalf("step = %+v", s)
	}
	if s.StartedAt == nil || s.CompletedAt == nil {
		t.Fatalf("step times nil")
	}
}

func TestListJobs_Pagination(t *testing.T) {
	t.Parallel()
	// Build a multi-page response: page 1 has 1 job + Link next; page 2 has 1 job no Link.
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s?page=2>; rel="next"`, r.URL.Path))
			_, _ = io.WriteString(w, `{"total_count":2,"jobs":[{"id":1,"run_id":99,"name":"a","status":"completed","conclusion":"success","steps":[]}]}`)
		case "2":
			_, _ = io.WriteString(w, `{"total_count":2,"jobs":[{"id":2,"run_id":99,"name":"b","status":"completed","conclusion":"success","steps":[]}]}`)
		default:
			t.Errorf("unexpected page %q", page)
		}
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	jobs, err := c.ListJobs(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 99)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d", len(jobs))
	}
	if jobs[0].ID != 1 || jobs[1].ID != 2 {
		t.Fatalf("ids = %d,%d", jobs[0].ID, jobs[1].ID)
	}
}

func TestListJobs_PaginationCap(t *testing.T) {
	t.Parallel()
	var pageHits int32
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&pageHits, 1)
		w.Header().Set("Content-Type", "application/json")
		// Always advertise next page → client must cap.
		w.Header().Set("Link", fmt.Sprintf(`<%s?page=999>; rel="next"`, r.URL.Path))
		_, _ = io.WriteString(w, `{"total_count":99999,"jobs":[{"id":1,"run_id":99,"name":"a","status":"completed","conclusion":"success","steps":[]}]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	_, err := c.ListJobs(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 99)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if got := atomic.LoadInt32(&pageHits); got != 5 {
		t.Fatalf("pageHits = %d, want 5", got)
	}
}

// ----- DownloadJobLog: redirect strips Authorization -----

func TestDownloadJobLog_RedirectStripsAuth(t *testing.T) {
	t.Parallel()
	body := []byte("hello-log-bytes")
	// Server #2: target of redirect.
	srv2, rec2 := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	// Server #1: API host, returns 302 to srv2.
	srv1, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv2.URL+"/log")
		w.WriteHeader(http.StatusFound)
	}))

	c := New(WithBaseURL(srv1.URL), WithTokenFunc(staticTokenFunc("supersecret")))
	got, err := c.DownloadJobLog(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 7)
	if err != nil {
		t.Fatalf("DownloadJobLog: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("body = %q", got)
	}
	snap := rec2.snapshot()
	if len(snap) != 1 {
		t.Fatalf("redirect target hits = %d", len(snap))
	}
	if h := snap[0].Header.Get("Authorization"); h != "" {
		t.Fatalf("Authorization leaked to redirect target: %q", h)
	}
}

// ----- DownloadJobLog: size cap -----

func TestDownloadJobLog_SizeCap(t *testing.T) {
	t.Parallel()
	// Source > 50 MB.
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Stream 51 MiB of zeros.
		buf := make([]byte, 1<<20) // 1 MiB
		for i := 0; i < 51; i++ {
			_, _ = w.Write(buf)
		}
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	got, err := c.DownloadJobLog(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 1)
	if !errors.Is(err, ErrLogTooLarge) {
		t.Fatalf("err = %v, want ErrLogTooLarge", err)
	}
	if len(got) == 0 {
		t.Fatalf("partial bytes empty")
	}
	if int64(len(got)) > maxLogBytes {
		t.Fatalf("returned %d bytes > cap %d", len(got), maxLogBytes)
	}
}

// ----- Status mapping (HTTP codes) -----

func TestStatusMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   int
		headers  map[string]string
		wantIs   error
		alsoIs   error // optional secondary errors.Is check
		wantBody string
	}{
		{"401", 401, nil, ErrUnauthorized, nil, `{"message":"bad creds"}`},
		{"403_norate", 403, map[string]string{"X-RateLimit-Remaining": "5"}, ErrForbidden, nil, `{"message":"nope"}`},
		{"403_rate", 403, map[string]string{"X-RateLimit-Remaining": "0"}, ErrRateLimited, ErrForbidden, `{"message":"rate"}`},
		{"404", 404, nil, ErrNotFound, nil, `{"message":"missing"}`},
		{"422", 422, nil, ErrUnprocessable, nil, `{"message":"validation"}`},
		{"429", 429, nil, ErrRateLimited, nil, `{"message":"slow down"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.wantBody)
			}))
			c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
			_, err := c.GetRun(context.Background(),
				domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 1)
			if !errors.Is(err, tc.wantIs) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.wantIs)
			}
			if tc.alsoIs != nil && !errors.Is(err, tc.alsoIs) {
				t.Fatalf("err = %v, want also errors.Is %v", err, tc.alsoIs)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.Status != tc.status {
				t.Fatalf("Status = %d", apiErr.Status)
			}
			if apiErr.Message == "" {
				t.Fatalf("empty message")
			}
		})
	}
}

// ----- Rate-limit Retry-After parsing -----

func TestRateLimit_RetryAfterHeader(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"message":"rate"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	_, err := c.GetRun(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 1)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("not APIError: %v", err)
	}
	if ae.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v", ae.RetryAfter)
	}
}

func TestRateLimit_RetryAfterFromReset(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	resetAt := now.Add(60 * time.Second).Unix()
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"message":"rate"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc),
		WithClock(func() time.Time { return now }))
	_, err := c.GetRun(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"}, 1)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("not APIError: %v", err)
	}
	if ae.RetryAfter < 59*time.Second || ae.RetryAfter > 61*time.Second {
		t.Fatalf("RetryAfter = %v", ae.RetryAfter)
	}
}

// ----- Single-flight -----

func TestSingleFlight_Dedupes(t *testing.T) {
	t.Parallel()
	var hits int32
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"workflow_runs":[{"id":1,"name":"x","status":"completed","conclusion":"success","head_sha":"a","run_attempt":1,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:01Z"}]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))

	const N = 8
	var wg sync.WaitGroup
	results := make([]ListRunsResult, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := c.ListRuns(context.Background(),
				domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
				ListRunsFilter{Branch: "main"})
			results[i] = res
			errs[i] = err
		}()
	}
	wg.Wait()
	if h := atomic.LoadInt32(&hits); h != 1 {
		t.Fatalf("server hits = %d, want 1", h)
	}
	for i, e := range errs {
		if e != nil {
			t.Fatalf("err[%d] = %v", i, e)
		}
		if len(results[i].Runs) != 1 || results[i].Runs[0].ID != 1 {
			t.Fatalf("results[%d] = %+v", i, results[i])
		}
	}
}

func TestSingleFlight_DifferentETagNotDeduped(t *testing.T) {
	t.Parallel()
	var hits int32
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = c.ListRuns(context.Background(),
			domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
			ListRunsFilter{IfNoneMatch: `"a"`})
	}()
	go func() {
		defer wg.Done()
		_, _ = c.ListRuns(context.Background(),
			domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
			ListRunsFilter{IfNoneMatch: `"b"`})
	}()
	wg.Wait()
	if h := atomic.LoadInt32(&hits); h != 2 {
		t.Fatalf("hits = %d, want 2", h)
	}
}

// ----- Auth header presence/absence + token redaction -----

func TestAuthHeader_PresentWhenTokenSet(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(staticTokenFunc("tok-xyz")))
	_, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("hits = %d", len(got))
	}
	if got[0].Header.Get("Authorization") != "Bearer tok-xyz" {
		t.Fatalf("Authorization = %q", got[0].Header.Get("Authorization"))
	}
	if got[0].Header.Get("User-Agent") == "" {
		t.Fatalf("User-Agent missing")
	}
}

func TestAuthHeader_AbsentWhenTokenEmpty(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc))
	_, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("hits = %d", len(got))
	}
	if h := got[0].Header.Get("Authorization"); h != "" {
		t.Fatalf("Authorization should be absent, got %q", h)
	}
}

func TestTokenNeverInErrorMessage(t *testing.T) {
	t.Parallel()
	const tok = "supersecrettoken12345"
	srv, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"message":"boom"}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(staticTokenFunc(tok)))
	_, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if strings.Contains(err.Error(), tok) {
		t.Fatalf("token leaked into error: %v", err)
	}
}

// ----- Required headers present -----

func TestRequiredHeaders(t *testing.T) {
	t.Parallel()
	srv, rec := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"workflow_runs":[]}`)
	}))
	c := New(WithBaseURL(srv.URL), WithTokenFunc(emptyTokenFunc), WithUserAgent("gh-peek/test"))
	_, err := c.ListRuns(context.Background(),
		domain.RepoRef{Host: "github.com", Owner: "o", Name: "r"},
		ListRunsFilter{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	h := rec.snapshot()[0].Header
	if h.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("Accept = %q", h.Get("Accept"))
	}
	if h.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Fatalf("X-GitHub-Api-Version missing")
	}
	if h.Get("User-Agent") != "gh-peek/test" {
		t.Fatalf("User-Agent = %q", h.Get("User-Agent"))
	}
}
