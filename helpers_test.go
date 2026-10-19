package exante

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testCred = Credentials{ClientID: "client-1", ApplicationID: "app-1", SharedKey: "secret"}

// seen is one request as the test server received it.
type seen struct {
	Method string
	URI    string
	Header http.Header
	Body   []byte
}

// recorder is a handler that records every request and answers with a fixed status and body.
type recorder struct {
	status int
	body   []byte
	// location, when set, is sent as the Location header.
	location string

	mu   sync.Mutex
	reqs []seen
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.reqs = append(r.reqs, seen{Method: req.Method, URI: req.RequestURI, Header: req.Header.Clone(), Body: body})
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.location != "" {
		w.Header().Set("Location", r.location)
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body)
}

func (r *recorder) all() []seen {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seen(nil), r.reqs...)
}

// only returns the single request the recorder saw, failing the test otherwise.
func (r *recorder) only(t *testing.T) seen {
	t.Helper()
	reqs := r.all()
	if len(reqs) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(reqs))
	}
	return reqs[0]
}

// reply returns a handler that records every request and answers each with status and body.
func reply(status int, body string) *recorder { return &recorder{status: status, body: []byte(body)} }

// hangUntilGone reads the request body, which lets the server notice a client that went away, and then blocks until
// it does.
var hangUntilGone = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	<-r.Context().Done()
})

// newClient returns a client for the test base URL and the test credentials.
func newClient(t *testing.T, opts ...Option) *Client {
	t.Helper()
	c, err := New("https://api.test", testCred, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

// newTestClient starts an in-memory test server with h and returns a client wired to it.
func newTestClient(t *testing.T, h http.Handler, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewTestServer(t, h)
	return newClient(t, append([]Option{WithHTTPClient(srv.Client())}, opts...)...)
}

// newTestClientOver returns a client whose requests go through rt.
func newTestClientOver(t *testing.T, rt http.RoundTripper, opts ...Option) *Client {
	t.Helper()
	return newClient(t, append([]Option{WithHTTPClient(&http.Client{Transport: rt})}, opts...)...)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}
