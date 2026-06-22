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

func TestCSPViaMiddlewareOnMainMux(t *testing.T) {
	t.Parallel()
	cfg := defaultTestConfig()
	cfg.Server.MetricsPort = 9090
	s := New(cfg, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(resp, req)
	csp := resp.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("CSP header missing on /metrics response")
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
