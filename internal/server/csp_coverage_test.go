// CSP coverage gap tests per test-gap-analysis.md security requirements
// Addresses Critical item #1: CSP missing on non-HTML endpoints (404s, /metrics)
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCSPTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	s := New(cfg, nil)
	s.RegisterRoutes(testRouteOptions())
	return s
}

func TestCSPOn404(t *testing.T) {
	t.Parallel()
	s := newCSPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent-page", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resp.Header().Get("Content-Security-Policy")
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
	csp := resp.Header().Get("Content-Security-Policy")
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

	csp := resp.Header().Get("Content-Security-Policy")
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
	csp := resp.Header().Get("Content-Security-Policy")
	directives := []string{"default-src", "script-src", "style-src"}
	for _, d := range directives {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP missing directive: %s", d)
		}
	}
}
