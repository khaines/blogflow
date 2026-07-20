// CSP coverage gap tests per test-gap-analysis.md security requirements
// Addresses Critical item #1: CSP missing on non-HTML endpoints (404s, /metrics)
package server

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/server/handlers"
)

func newCSPTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	cfg.Site.Title = "CSP Test Blog"
	cfg.Site.Description = "CSP coverage test blog"
	cfg.Site.BaseURL = "https://example.com"
	cfg.Site.Author.Name = "CSP Tester"
	cfg.Feed.Type = "atom"
	cfg.Feed.Items = 20

	s := New(cfg, nil)
	opts := testRouteOptions()
	deps := handlers.NewDeps(cfg, cspTestIndex(), nil)
	opts.FeedHandler = handlers.NewFeedHandler(deps).ServeHTTP
	opts.SitemapHandler = handlers.NewSitemapHandler(deps).ServeHTTP
	s.RegisterRoutes(opts)
	return s
}

func cspTestIndex() *content.Index {
	idx := &content.Index{
		BySlug:     make(map[string]*content.Post),
		ByTag:      make(map[string][]*content.Post),
		ByYear:     make(map[int][]*content.Post),
		PageBySlug: make(map[string]*content.Post),
	}
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for i := range 2 {
		p := &content.Post{
			Slug:    fmt.Sprintf("csp-post-%d", i+1),
			Summary: fmt.Sprintf("Summary for CSP post %d", i+1),
			Content: template.HTML(fmt.Sprintf("<p>CSP content %d</p>", i+1)), //nolint:gosec
		}
		p.Title = fmt.Sprintf("CSP Post %d", i+1)
		p.Date = base.AddDate(0, 0, -i)
		idx.Posts = append(idx.Posts, p)
		idx.BySlug[p.Slug] = p
	}
	return idx
}

func resultCSP(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.Header.Get("Content-Security-Policy")
}

func assertCompleteCSP(t *testing.T, csp, endpoint string) {
	t.Helper()
	if csp == "" {
		t.Fatalf("CSP header missing on %s response", endpoint)
	}
	directives := []string{
		"default-src", "script-src", "style-src", "img-src", "font-src",
		"connect-src", "frame-ancestors", "base-uri", "form-action", "object-src",
	}
	for _, d := range directives {
		if !strings.Contains(csp, d) {
			t.Errorf("%s CSP missing directive: %s", endpoint, d)
		}
	}
}

func TestCSPOn404(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent-page", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resultCSP(t, resp)
	if csp == "" {
		t.Fatal("CSP header missing on 404 handler response")
	}
	if n := resp.Code; n != http.StatusNotFound {
		t.Errorf("status = %d, want 404", n)
	}
}

func TestCSPOnSeparateMetricsPort(t *testing.T) {
	t.Parallel()
	cfg := defaultTestConfig()
	cfg.Server.MetricsPort = 9090
	s := New(cfg, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resultCSP(t, resp)
	if csp == "" {
		t.Fatal("CSP header missing on main server response when MetricsPort is set")
	}
}

// TestCSPViaMiddlewareOnMetricsServer verifies that the dedicated metrics
// port also carries security headers (CSP, X-Frame-Options, etc.) via the
// same middleware chain as the main server.
func TestCSPViaMiddlewareOnMetricsServer(t *testing.T) {
	t.Parallel()
	cfg := defaultTestConfig()
	cfg.Server.MetricsPort = 9090
	s := New(cfg, nil)

	ms := s.MetricsServer()
	if ms == nil {
		t.Fatal("MetricsServer() should not be nil when MetricsPort > 0")
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()
	ms.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("/metrics status = %d, want %d", resp.Code, http.StatusOK)
	}

	// Assert Prometheus format content in the body.
	body := resp.Body.String()
	if body == "" {
		t.Fatal("/metrics body is empty")
	}
	// Prometheus exposition format requires # HELP or # TYPE lines.
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Errorf("/metrics body does not contain valid Prometheus format markers (expected # HELP or # TYPE, got: %q)", body[:min(200, len(body))])
	}

	csp := resultCSP(t, resp)
	if csp == "" {
		t.Fatal("CSP header missing on /metrics response from dedicated metrics server")
	}

	// Also verify other security headers arrive identically on metrics port.
	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=(), payment=(), usb=(), browsing-topics=(), interest-cohort=()",
	}
	for key, want := range wantHeaders {
		got := resp.Header().Get(key)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestCSPDirectiveCompleteness(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	assertCompleteCSP(t, resultCSP(t, resp), "/")
}

func TestCSPOnMainMuxMetrics(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("/metrics status = %d, want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	if body == "" {
		t.Fatal("/metrics body is empty")
	}
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Errorf("/metrics body does not contain valid Prometheus format markers (expected # HELP or # TYPE, got: %q)", body[:min(200, len(body))])
	}
	assertCompleteCSP(t, resultCSP(t, resp), "/metrics")
}

func TestCSPOnXMLHandlers(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "feed", path: "/feed.xml"},
		{name: "sitemap", path: "/sitemap.xml"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			resp := httptest.NewRecorder()
			s.httpServer.Handler.ServeHTTP(resp, req)
			if resp.Code < 200 || resp.Code >= 300 {
				t.Errorf("%s status = %d, want 2xx", tc.path, resp.Code)
			}
			assertCompleteCSP(t, resultCSP(t, resp), tc.path)
		})
	}
}

func TestCSPOnHealthzEndpoint(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resultCSP(t, resp)
	if csp == "" {
		t.Fatal("CSP header missing on /healthz response")
	}
}

func TestCSPOnReadyzEndpoint(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resultCSP(t, resp)
	if csp == "" {
		t.Fatal("CSP header missing on /readyz response")
	}
}
