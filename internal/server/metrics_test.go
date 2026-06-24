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

	// Pre-populate a blogflow counter observation so blogflow_http_requests_total
	// has a data line in the Prometheus exposition format (counters with zero
	// observations produce no output lines).
	httpRequestsTotal.WithLabelValues("GET", "/_test/obs", "200").Inc()

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
	// Verify BlogFlow metrics, not just Go runtime metrics.
	if !strings.Contains(body, "blogflow_http_requests_total") {
		t.Fatal("expected blogflow prometheus metrics (e.g. blogflow_http_requests_total) in response body")
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
		"method": "PATCH", "path": "unmatched", "status": "2xx",
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(inner)
	req := httptest.NewRequest(http.MethodPatch, "/test-histogram", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := histogramCount(t, "blogflow_http_request_duration_seconds", map[string]string{
		"method": "PATCH", "path": "unmatched", "status": "2xx",
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

func TestMetricsMiddlewareInFlightGauge(t *testing.T) {
	// Not parallel: the global in-flight gauge is shared with other tests
	// that call MetricsMiddleware, so concurrent mutations cause flakes.

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(inner)

	before := gaugeValue(t, "blogflow_http_requests_in_flight")

	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "/in-flight-test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}()

	<-started // handler is executing, Inc() has been called

	during := gaugeValue(t, "blogflow_http_requests_in_flight")
	if during != before+1 {
		t.Fatalf("expected in-flight gauge to increase by 1 during request, before=%f during=%f", before, during)
	}

	close(release) // let the handler finish
	<-done         // wait for goroutine to fully complete (including defer Dec)

	after := gaugeValue(t, "blogflow_http_requests_in_flight")
	if after != before {
		t.Fatalf("expected in-flight gauge to return to %f after request, got %f", before, after)
	}
}

func TestMetricsMiddlewareStatusLabelOnDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		wantBucket string
	}{
		{"2xx", http.StatusOK, "2xx"},
		{"3xx", http.StatusMovedPermanently, "3xx"},
		{"4xx", http.StatusNotFound, "4xx"},
		{"5xx", http.StatusInternalServerError, "5xx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use OPTIONS + unique path suffix to isolate from other tests.
			labels := map[string]string{
				"method": "OPTIONS", "path": "unmatched", "status": tt.wantBucket,
			}
			before := histogramCount(t, "blogflow_http_request_duration_seconds", labels)

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			handler := MetricsMiddleware(inner)
			req := httptest.NewRequest(http.MethodOptions, "/status-label-test-"+tt.name, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			after := histogramCount(t, "blogflow_http_request_duration_seconds", labels)
			if diff := after - before; diff != 1 {
				t.Fatalf("expected histogram sample count with status=%s to increase by 1, got %d", tt.wantBucket, diff)
			}
		})
	}
}

func TestBlogBuckets(t *testing.T) {
	t.Parallel()

	want := []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}
	if len(blogBuckets) != len(want) {
		t.Fatalf("expected %d buckets, got %d", len(want), len(blogBuckets))
	}
	for i, b := range blogBuckets {
		if b != want[i] {
			t.Fatalf("bucket[%d] = %f, want %f", i, b, want[i])
		}
	}
}

func TestStatusBucketLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code int
		want string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{299, "2xx"},
		{301, "3xx"},
		{304, "3xx"},
		{400, "4xx"},
		{404, "4xx"},
		{499, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{100, "other"},
		{600, "other"},
	}

	for _, tt := range tests {
		got := statusBucketLabel(tt.code)
		if got != tt.want {
			t.Errorf("statusBucketLabel(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// --- helpers ---

func gaugeValue(t *testing.T, name string) float64 {
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
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

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
