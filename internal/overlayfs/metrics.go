package overlayfs

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// overlayMetrics holds all Prometheus metrics for the overlay filesystem.
// When nil on OverlayFS, all instrumentation is a no-op (zero overhead).
type overlayMetrics struct {
	resolveDuration *prometheus.HistogramVec
	layerHitTotal   *prometheus.CounterVec
	missTotal       prometheus.Counter
	negCacheHit     prometheus.Counter
	negCacheSize    prometheus.GaugeFunc
	pathRejected    *prometheus.CounterVec
}

func newOverlayMetrics(reg prometheus.Registerer, negCacheSize func() float64) (*overlayMetrics, error) {
	m := &overlayMetrics{
		resolveDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "blogflow_overlay_resolve_duration_seconds",
				Help:    "Time to resolve a file through the layer stack.",
				Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
			},
			[]string{"op"},
		),
		layerHitTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blogflow_overlay_layer_hit_total",
				Help: "Number of times each layer served a file.",
			},
			[]string{"layer"},
		),
		missTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "blogflow_overlay_miss_total",
				Help: "Files not found in any layer.",
			},
		),
		negCacheHit: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "blogflow_overlay_negcache_hit_total",
				Help: "Negative cache hits (avoided layer checks).",
			},
		),
		negCacheSize: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "blogflow_overlay_negcache_size",
				Help: "Current number of entries in the negative cache.",
			},
			negCacheSize,
		),
		pathRejected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blogflow_overlay_path_rejected_total",
				Help: "Path validation rejections.",
			},
			[]string{"reason"},
		),
	}

	collectors := []prometheus.Collector{
		m.resolveDuration, m.layerHitTotal, m.missTotal,
		m.negCacheHit, m.negCacheSize, m.pathRejected,
	}
	var registered []prometheus.Collector
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			// Roll back previously registered collectors.
			for _, r := range registered {
				reg.Unregister(r)
			}
			return nil, fmt.Errorf("overlayfs: register metric: %w", err)
		}
		registered = append(registered, c)
	}

	return m, nil
}

// Option configures an OverlayFS instance.
type Option func(*OverlayFS) error

// WithMetrics enables Prometheus metrics collection on the overlay filesystem.
// When not set, all instrumentation is a no-op with zero overhead.
// Returns an error if metric registration fails (e.g., duplicate registration).
func WithMetrics(reg prometheus.Registerer) Option {
	return func(o *OverlayFS) error {
		m, err := newOverlayMetrics(reg, func() float64 {
			return float64(o.negCacheSize.Load())
		})
		if err != nil {
			return err
		}
		o.metrics = m
		return nil
	}
}

// classifyInvalidPath returns a reason label for a rejected path.
func classifyInvalidPath(name string) string {
	if len(name) > 0 && name[0] == '/' {
		return "absolute"
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '.' && (i == 0 || name[i-1] == '/') {
			rest := name[i:]
			if rest == ".." || (len(rest) > 2 && rest[1] == '.' && rest[2] == '/') {
				return "traversal"
			}
		}
	}
	return "invalid"
}
