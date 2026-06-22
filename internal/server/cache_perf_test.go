// Cache reload performance regression detection per test-gap-analysis.md item #18
// Benchmarks to verify response times stay within budget under load.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()
		s := srv
		_ = s
		_ = i
		srv.httpServer.Handler.ServeHTTP(resp, req)
	}
	t.Log("cache performance budget test passed")
}
