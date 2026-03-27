// Package otel provides OpenTelemetry initialization and slog integration
// for BlogFlow. Tracing and metrics are opt-in: when OTEL_TRACES_EXPORTER
// and OTEL_METRICS_EXPORTER are unset or empty, Init returns a no-op
// shutdown with zero overhead.
package otel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	prometheusbridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Init configures the global OpenTelemetry tracer and meter providers.
//
// Tracing is enabled when OTEL_TRACES_EXPORTER is set (e.g. "otlp").
// Metrics bridging is enabled when OTEL_METRICS_EXPORTER is set (e.g. "otlp").
//
// When metrics are enabled, a Prometheus bridge translates existing Prometheus
// metrics (registered with the default gatherer) into OTel OTLP export. This
// provides dual-export: Prometheus scraping via /metrics continues to work,
// and OTel-compatible backends receive the same metrics via OTLP.
//
// The returned shutdown function flushes and releases all enabled providers.
func Init(ctx context.Context, serviceName, version string, logger *slog.Logger) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }

	res, err := buildResource(serviceName, version)
	if err != nil {
		return noop, err
	}

	var shutdowns []func(context.Context) error

	// --- Tracing ---
	if exp := os.Getenv("OTEL_TRACES_EXPORTER"); exp != "" {
		tp, tErr := initTracing(ctx, res)
		if tErr != nil {
			return noop, tErr
		}
		shutdowns = append(shutdowns, tp.Shutdown)
		if logger != nil {
			logger.Info("otel: tracing enabled", "exporter", exp, "service", serviceName)
		}
	} else if logger != nil {
		logger.Debug("otel: tracing disabled (OTEL_TRACES_EXPORTER not set)")
	}

	// --- Metrics ---
	if exp := os.Getenv("OTEL_METRICS_EXPORTER"); exp != "" {
		mp, mErr := initMetrics(ctx, res)
		if mErr != nil {
			_ = runShutdowns(ctx, shutdowns)
			return noop, mErr
		}
		shutdowns = append(shutdowns, mp.Shutdown)
		if logger != nil {
			logger.Info("otel: metrics bridge enabled", "exporter", exp, "service", serviceName)
		}
	} else if logger != nil {
		logger.Debug("otel: metrics bridge disabled (OTEL_METRICS_EXPORTER not set)")
	}

	if len(shutdowns) == 0 {
		return noop, nil
	}

	return func(ctx context.Context) error {
		return runShutdowns(ctx, shutdowns)
	}, nil
}

func buildResource(serviceName, version string) (*resource.Resource, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}
	return res, nil
}

func initTracing(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel: create OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func initMetrics(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	exp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel: create OTLP metric exporter: %w", err)
	}

	// Bridge existing Prometheus metrics into OTel via the default gatherer.
	bridge := prometheusbridge.NewMetricProducer()

	reader := metric.NewPeriodicReader(exp,
		metric.WithInterval(30*time.Second),
		metric.WithProducer(bridge),
	)

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(reader),
	)

	otel.SetMeterProvider(mp)

	return mp, nil
}

// runShutdowns calls each function in reverse order (LIFO) so the
// last-initialised provider is shut down first, matching the OTel SDK's
// recommended teardown sequence.
func runShutdowns(ctx context.Context, fns []func(context.Context) error) error {
	var first error
	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}
