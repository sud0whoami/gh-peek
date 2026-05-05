package githubapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// recordedRequest captures the salient fields of an inbound request
// for assertion in tests.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// recordingHandler wraps an http.Handler, recording each request.
type recordingHandler struct {
	mu       sync.Mutex
	requests []recordedRequest
	inner    http.Handler
}

func newRecordingHandler(h http.Handler) *recordingHandler {
	return &recordingHandler{inner: h}
}

func (r *recordingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	rec := recordedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  req.URL.RawQuery,
		Header: req.Header.Clone(),
		Body:   body,
	}
	r.mu.Lock()
	r.requests = append(r.requests, rec)
	r.mu.Unlock()
	r.inner.ServeHTTP(w, req)
}

func (r *recordingHandler) snapshot() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

// newTestServer wires recordingHandler around the supplied handler and
// returns the server and recorder. The server is closed via t.Cleanup.
func newTestServer(t *testing.T, h http.Handler) (*httptest.Server, *recordingHandler) {
	t.Helper()
	rec := newRecordingHandler(h)
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	return srv, rec
}

// emptyTokenFunc returns "" (anonymous) and no error.
func emptyTokenFunc(string) (string, error) { return "", nil }

// staticTokenFunc returns the provided token for any host.
func staticTokenFunc(tok string) func(string) (string, error) {
	return func(string) (string, error) { return tok, nil }
}
