// Package otel provides OpenTelemetry initialization and slog integration
// for BlogFlow. Tracing is opt-in: when OTEL_TRACES_EXPORTER is unset or
// empty the Init function returns a no-op shutdown with zero overhead.
package otel

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Init configures the global OpenTelemetry tracer provider and propagator.
//
// When OTEL_TRACES_EXPORTER is unset or empty, Init is a no-op: it returns
// a nil-error and a shutdown function that does nothing. This guarantees zero
// overhead in default (non-instrumented) deployments.
//
// When enabled, Init creates an OTLP/HTTP exporter that honours standard
// environment variables (OTEL_EXPORTER_OTLP_ENDPOINT, etc.), sets a
// W3C TraceContext + Baggage propagator, and installs the provider globally.
// The returned shutdown function flushes pending spans and releases resources.
func Init(ctx context.Context, serviceName, version string, logger *slog.Logger) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }

	exporter := os.Getenv("OTEL_TRACES_EXPORTER")
	if exporter == "" {
		if logger != nil {
			logger.Debug("otel: tracing disabled (OTEL_TRACES_EXPORTER not set)")
		}
		return noop, nil
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, fmt.Errorf("otel: create OTLP HTTP exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return noop, fmt.Errorf("otel: create resource: %w", err)
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

	if logger != nil {
		logger.Info("otel: tracing enabled", "exporter", exporter, "service", serviceName)
	}

	return tp.Shutdown, nil
}
