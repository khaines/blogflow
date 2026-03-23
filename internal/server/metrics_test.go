package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsEndpointReturns200(t *testing.T) {
	t.Parallel()

	handler := MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "application/openmetrics-text") {
		t.Fatalf("unexpected content type: %s", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "go_goroutines") {
		t.Fatal("expected prometheus metrics in response body")
	}
}

func TestMetricsMiddlewareIncrementsCounter(t *testing.T) {
	t.Parallel()

	// Use PUT to isolate from other tests sharing the global registry.
	before := counterValue(t, "blogflow_http_requests_total", map[string]string{
		"method": "PUT", "path": "unmatched", "status": "200",
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(inner)
	req := httptest.NewRequest(http.MethodPut, "/test-counter", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := counterValue(t, "blogflow_http_requests_total", map[string]string{
		"method": "PUT", "path": "unmatched", "status": "200",
	})

	if diff := after - before; diff != 1 {
		t.Fatalf("expected counter to increase by 1, got %f", diff)
	}
}

func TestMetricsMiddlewareRecordsHistogram(t *testing.T) {
	t.Parallel()

	// Use PATCH to isolate from other tests sharing the global registry.
	before := histogramCount(t, "blogflow_http_request_duration_seconds", map[string]string{
		"method": "PATCH", "path": "unmatched",
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(inner)
	req := httptest.NewRequest(http.MethodPatch, "/test-histogram", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := histogramCount(t, "blogflow_http_request_duration_seconds", map[string]string{
		"method": "PATCH", "path": "unmatched",
	})

	if diff := after - before; diff != 1 {
		t.Fatalf("expected histogram sample count to increase by 1, got %d", diff)
	}
}

func TestMetricsMiddlewareSkipsMetricsPath(t *testing.T) {
	t.Parallel()

	// Use DELETE to isolate from other tests sharing the global registry.
	before := counterValue(t, "blogflow_http_requests_total", map[string]string{
		"method": "DELETE", "path": "unmatched", "status": "200",
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(inner)
	req := httptest.NewRequest(http.MethodDelete, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := counterValue(t, "blogflow_http_requests_total", map[string]string{
		"method": "DELETE", "path": "unmatched", "status": "200",
	})

	if diff := after - before; diff != 0 {
		t.Fatalf("expected no counter increment for /metrics, got %f", diff)
	}
}

// --- helpers ---

func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, m := range fam.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func histogramCount(t *testing.T, name string, labels map[string]string) uint64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, m := range fam.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func matchLabels(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(pairs) != len(want) {
		return false
	}
	for _, lp := range pairs {
		v, ok := want[lp.GetName()]
		if !ok || v != lp.GetValue() {
			return false
		}
	}
	return true
}
