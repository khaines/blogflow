package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// blogBuckets provides histogram buckets tuned for blog serving workloads.
// Most responses complete in < 10 ms, so we add fine granularity from 500 µs
// to 1 s instead of using the default Prometheus buckets (5 ms – 10 s).
var blogBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}

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
			Buckets: blogBuckets,
		},
		[]string{"method", "path", "status"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_http_requests_in_flight",
			Help: "Number of HTTP requests currently being served.",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestsInFlight)
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

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

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
		statusBucket := statusBucketLabel(wrapped.statusCode)

		httpRequestsTotal.WithLabelValues(r.Method, pattern, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, pattern, statusBucket).Observe(duration)
	})
}

// statusBucketLabel maps an HTTP status code to a low-cardinality bucket
// string ("2xx", "3xx", "4xx", "5xx"). Codes outside 200–599 return "other".
func statusBucketLabel(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}
