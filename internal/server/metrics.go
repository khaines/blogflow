package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blogflow_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blogflow_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

// MetricsHandler returns the Prometheus metrics HTTP handler.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// MetricsMiddleware records request count and duration for each HTTP request.
// It uses the matched route pattern (Go 1.22+) to avoid high-cardinality labels.
// Requests to /metrics are passed through without recording to avoid
// inflating metrics on every Prometheus scrape.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip recording metrics for the /metrics endpoint itself.
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Use the route pattern from Go 1.22+ ServeMux to keep cardinality low.
		pattern := r.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.statusCode)

		httpRequestsTotal.WithLabelValues(r.Method, pattern, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, pattern).Observe(duration)
	})
}
