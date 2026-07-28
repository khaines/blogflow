package search

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// searchBuckets are histogram buckets tuned for search latency, which the
// design targets at p50 ≤ 10 ms / p95 ≤ 50 ms / p99 ≤ 150 ms.
var searchBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}

// resultCountBuckets bucket the number of results returned per query.
var resultCountBuckets = []float64{0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000}

var (
	queriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blogflow_search_queries_total",
			Help: "Total search requests handled, by outcome status (ok, invalid, error).",
		},
		[]string{"status"},
	)

	queryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blogflow_search_query_duration_seconds",
			Help:    "End-to-end search handler latency in seconds, by status.",
			Buckets: searchBuckets,
		},
		[]string{"status"},
	)

	resultsTotal = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "blogflow_search_results_total",
			Help:    "Distribution of result counts per successful query.",
			Buckets: resultCountBuckets,
		},
	)

	zeroResultsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "blogflow_search_zero_results_total",
			Help: "Count of valid queries returning zero results.",
		},
	)

	indexDocuments = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_search_index_documents",
			Help: "Number of posts in the active search index.",
		},
	)

	indexTokens = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_search_index_tokens",
			Help: "Number of indexed token occurrences in the active search index.",
		},
	)

	indexRebuildDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blogflow_search_index_rebuild_duration_seconds",
			Help:    "Search index rebuild duration during content scans, by status.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"status"},
	)

	indexMemoryBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_search_index_memory_bytes",
			Help: "Active logical index byte estimate (distinct from actual Go heap).",
		},
	)

	indexRebuildPeakBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_search_index_rebuild_peak_bytes",
			Help: "Peak logical bytes observed during the latest rebuild.",
		},
	)

	indexTruncatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blogflow_search_index_truncated_total",
			Help: "Count of rebuilds that published a truncated index, by reason.",
		},
		[]string{"reason"},
	)

	snapshotGeneration = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blogflow_search_snapshot_generation",
			Help: "Current published snapshot generation.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		queriesTotal,
		queryDuration,
		resultsTotal,
		zeroResultsTotal,
		indexDocuments,
		indexTokens,
		indexRebuildDuration,
		indexMemoryBytes,
		indexRebuildPeakBytes,
		indexTruncatedTotal,
		snapshotGeneration,
	)
}

// Query outcome statuses used as metric labels.
const (
	StatusOK      = "ok"
	StatusInvalid = "invalid"
	StatusError   = "error"
)

// RecordQuery records a completed search request's outcome and latency.
func RecordQuery(status string, d time.Duration) {
	queriesTotal.WithLabelValues(status).Inc()
	queryDuration.WithLabelValues(status).Observe(d.Seconds())
}

// RecordResults records the result-count distribution for a successful query
// and increments the zero-result counter when appropriate.
func RecordResults(count int) {
	resultsTotal.Observe(float64(count))
	if count == 0 {
		zeroResultsTotal.Inc()
	}
}

// ObserveRebuild publishes gauges and the rebuild histogram for a successful
// index build. peakBytes is the peak logical estimate while both the old and
// new generations were live.
func ObserveRebuild(si *SearchIndex, d time.Duration, peakBytes int64) {
	indexRebuildDuration.WithLabelValues(StatusOK).Observe(d.Seconds())
	if si == nil {
		return
	}
	indexDocuments.Set(float64(len(si.docs)))
	indexTokens.Set(float64(si.tokenCount))
	indexMemoryBytes.Set(float64(si.logicalBytes))
	indexRebuildPeakBytes.Set(float64(peakBytes))
	snapshotGeneration.Set(float64(si.generation))
	if si.truncated {
		indexTruncatedTotal.WithLabelValues(string(si.reason)).Inc()
	}
}

// RecordRebuildFailure records a failed search index build for a generation.
// Content still serves with a nil search index; the /search route returns 503
// until the next successful build. It advances the snapshot-generation gauge and
// zeroes the active-index gauges so dashboards reflect that no search index is
// currently published (rather than showing the last successful values).
func RecordRebuildFailure(gen uint64, d time.Duration) {
	indexRebuildDuration.WithLabelValues(StatusError).Observe(d.Seconds())
	indexDocuments.Set(0)
	indexTokens.Set(0)
	indexMemoryBytes.Set(0)
	indexRebuildPeakBytes.Set(0)
	snapshotGeneration.Set(float64(gen))
}
