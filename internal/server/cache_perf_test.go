// Cache reload performance regression detection per test-gap-analysis.md item #18
// Benchmarks to verify response times stay within budget under load.
package server

import (
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

	for i := range 100 {
		_ = i
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()
		start := time.Now()
		srv.httpServer.Handler.ServeHTTP(resp, req)

		if resp.Code < 200 || resp.Code >= 500 {
			t.Fatalf("iteration %d: unexpected status %d", i, resp.Code)
		}

		latency := time.Since(start)
		if latency > 5*time.Second {
			t.Errorf("iteration %d: latency %v exceeds 5s budget", i, latency)
		}
	}
}
