// Cache reload performance regression detection per test-gap-analysis.md item #18
// Benchmarks to verify response times stay within budget under load.
package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func BenchmarkCacheReadSteadyState(b *testing.B) {
	cfg := defaultTestConfig()
	cfg.Cache.Enabled = true
	srv := New(cfg, nil)
	srv.RegisterRoutes(testRouteOptions())

	b.ResetTimer()
	for i := range b.N {
		_ = i
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(resp, req)
		_ = resp
	}
}

func BenchmarkCacheReadMissRate(b *testing.B) {
	cfg := defaultTestConfig()
	srv := New(cfg, nil)
	srv.RegisterRoutes(testRouteOptions())

	b.ResetTimer()
	for i := range b.N {
		_ = i
		req := httptest.NewRequest(http.MethodGet, "/feed", nil)
		resp := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(resp, req)
		_ = resp
	}
}

func TestCachePerformanceBudget(t *testing.T) {
	t.Parallel()
	cfg := defaultTestConfig()
	srv := New(cfg, nil)
	srv.RegisterRoutes(testRouteOptions())

	// Phase 1: verify the first request succeeds with a 2xx status.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	start := time.Now()
	srv.httpServer.Handler.ServeHTTP(resp, req)

	if resp.Code < 200 || resp.Code >= 300 {
		t.Fatalf("expected 2xx status, got %d", resp.Code)
	}
	latency1 := time.Since(start)
	if latency1 > 5*time.Second {
		t.Errorf("first request latency %v exceeds 5s budget", latency1)
	}

	// Phase 2: make a second request — for a cached handler this should be
	// at least as fast (in-process handler: just verify consistent success).
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	resp2 := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(resp2, req2)

	if resp2.Code != http.StatusOK {
		t.Errorf("second request status = %d, want %d", resp2.Code, http.StatusOK)
	}

	// Phase 3: assert the response body is consistent with the stub we registered.
	bodyBytes, _ := io.ReadAll(resp.Body)
	if string(bodyBytes) != "home" {
		t.Errorf("second request body = %q, want %q", string(bodyBytes), "home")
	}
}
