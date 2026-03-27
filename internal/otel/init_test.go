package otel_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	blogotel "github.com/khaines/blogflow/internal/otel"
)

func TestInit_DisabledByDefault(t *testing.T) {
	// Ensure OTEL_TRACES_EXPORTER is unset.
	t.Setenv("OTEL_TRACES_EXPORTER", "")

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", nil)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil no-op function")
	}
	// The no-op shutdown must succeed.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil", err)
	}
}

func TestInit_DisabledWhenNone(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

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

func TestInit_UnsupportedExporter(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "jaeger")

	_, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", nil)
	if err == nil {
		t.Fatal("Init() error = nil, want error for unsupported exporter")
	}
}

func TestInit_EnabledReturnsShutdown(t *testing.T) {
	// Point at a non-existent collector — we only test that the provider
	// is created, not that it can actually export.
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:0")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := blogotel.Init(context.Background(), "test-svc", "0.0.0", logger)
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil function")
	}

	// Shutdown should succeed (no spans to flush).
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil", err)
	}
}
