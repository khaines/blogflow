package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func searchTestServer(t *testing.T, enabled bool) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	cfg.Search.Enabled = enabled
	s := New(cfg, nil)
	opts := testRouteOptions()
	if enabled {
		opts.SearchHandler = stubHandler("search-results")
	}
	s.RegisterRoutes(opts)
	return s
}

func TestSearchRoute_RegisteredWhenEnabled(t *testing.T) {
	s := searchTestServer(t, true)
	req := httptest.NewRequest(http.MethodGet, "/search?q=go", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /search = %d, want 200", resp.Code)
	}
	if body := resp.Body.String(); body != "search-results" {
		t.Errorf("body = %q, want search handler output", body)
	}
}

func TestSearchRoute_NotRegisteredWhenDisabled(t *testing.T) {
	s := searchTestServer(t, false)
	req := httptest.NewRequest(http.MethodGet, "/search?q=go", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /search with search disabled = %d, want 404", resp.Code)
	}
}

func TestSearchRoute_MethodNotAllowed(t *testing.T) {
	s := searchTestServer(t, true)
	req := httptest.NewRequest(http.MethodPost, "/search?q=go", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /search = %d, want 405", resp.Code)
	}
	// Go's ServeMux advertises GET (and the implicit HEAD it serves via GET).
	if allow := resp.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow header = %q, want it to include GET", allow)
	}
}

func TestSearchRoute_HeadServedByGet(t *testing.T) {
	s := searchTestServer(t, true)
	req := httptest.NewRequest(http.MethodHead, "/search?q=go", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("HEAD /search = %d, want 200", resp.Code)
	}
}

func TestSearchRoute_PanicsWhenEnabledWithoutHandler(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when search enabled but SearchHandler is nil")
		}
	}()
	cfg := defaultTestConfig()
	cfg.Search.Enabled = true
	s := New(cfg, nil)
	s.RegisterRoutes(testRouteOptions()) // no SearchHandler set
}

// TestSearchRoute_QueryNotLoggedAtInfo verifies the access log redacts the raw
// search query for /search (design §5.3/§7.1) while still logging query strings
// for other routes.
func TestSearchRoute_QueryNotLoggedAtInfo(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.Search.Enabled = true
	s := New(cfg, slog.New(slog.NewTextHandler(&buf, nil)))
	opts := testRouteOptions()
	opts.SearchHandler = stubHandler("results")
	s.RegisterRoutes(opts)

	// Search query must not appear in the access log.
	req := httptest.NewRequest(http.MethodGet, "/search?q=SENSITIVEQUERY", nil)
	s.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), req)
	if logged := buf.String(); strings.Contains(logged, "SENSITIVEQUERY") {
		t.Errorf("access log leaked the raw search query: %s", logged)
	}
	if logged := buf.String(); !strings.Contains(logged, "/search") {
		t.Errorf("expected /search path in access log: %s", buf.String())
	}

	// Non-search routes still log their query string (redaction is scoped).
	buf.Reset()
	req2 := httptest.NewRequest(http.MethodGet, "/posts?page=2", nil)
	s.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), req2)
	if logged := buf.String(); !strings.Contains(logged, "page=2") {
		t.Errorf("non-search query should still be logged: %s", logged)
	}
}
