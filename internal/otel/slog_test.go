package otel_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	blogotel "github.com/khaines/blogflow/internal/otel"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceHandler_InjectsTraceFields(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := blogotel.TraceHandler(inner)
	logger := slog.New(handler)

	// Create a span context with a known trace ID and span ID.
	traceID := trace.TraceID{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	}
	spanID := trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.InfoContext(ctx, "with span")

	output := buf.String()
	if !strings.Contains(output, "trace_id=0102030405060708090a0b0c0d0e0f10") {
		t.Errorf("expected trace_id in log output, got: %s", output)
	}
	if !strings.Contains(output, "span_id=1112131415161718") {
		t.Errorf("expected span_id in log output, got: %s", output)
	}
}

func TestTraceHandler_OmitsFieldsWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := blogotel.TraceHandler(inner)
	logger := slog.New(handler)

	logger.InfoContext(context.Background(), "no span")

	output := buf.String()
	if strings.Contains(output, "trace_id") {
		t.Errorf("unexpected trace_id in log output without span: %s", output)
	}
	if strings.Contains(output, "span_id") {
		t.Errorf("unexpected span_id in log output without span: %s", output)
	}
}

func TestTraceHandler_WithAttrsPreserved(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := blogotel.TraceHandler(inner)
	derived := handler.WithAttrs([]slog.Attr{slog.String("component", "test")})
	logger := slog.New(derived)

	logger.InfoContext(context.Background(), "attrs test")

	output := buf.String()
	if !strings.Contains(output, "component=test") {
		t.Errorf("expected component attr in output, got: %s", output)
	}
}

func TestTraceHandler_WithGroupPreserved(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := blogotel.TraceHandler(inner)
	derived := handler.WithGroup("mygroup")
	logger := slog.New(derived)

	logger.InfoContext(context.Background(), "group test", "key", "val")

	output := buf.String()
	if !strings.Contains(output, "mygroup.key=val") {
		t.Errorf("expected grouped key in output, got: %s", output)
	}
}
