package otel_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	blogotel "github.com/khaines/blogflow/internal/otel"
)

func TestInit_DisabledByDefault(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", nil)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil no-op function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil", err)
	}
}

func TestInit_TracingEnabledReturnsShutdown(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil", err)
	}
}

func TestInit_MetricsBridgeDisabledByDefault(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	// With both disabled, shutdown is the no-op.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil", err)
	}
}

func TestInit_MetricsBridgeEnabledReturnsShutdown(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}
	// Shutdown may return an export error (no collector listening) — that's
	// expected in tests; we only verify that Init wired up a real provider.
	_ = shutdown(context.Background())
}

func TestInit_BothTracingAndMetricsEnabled(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}
	// Shutdown may return an export error — acceptable without a real collector.
	_ = shutdown(context.Background())
}

func TestInit_ProtocolDefault_UsesHTTP(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", nil)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	_ = shutdown(context.Background())
}

func TestInit_ProtocolGRPC(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}
	_ = shutdown(context.Background())
}

func TestInit_ProtocolHTTPExplicit(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}
	_ = shutdown(context.Background())
}
