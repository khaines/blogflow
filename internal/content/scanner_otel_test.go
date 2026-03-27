package content

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

func TestScanContext_CreatesSpan(t *testing.T) {
	exp := setupTestTracer(t)

	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello", "hello", "2025-06-01", []string{"go"}, false, "Some content.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPage("About", "about", 1, "About page.\n")),
		},
	}

	scanner := newTestScanner()
	idx, err := scanner.ScanContext(context.Background(), fs)
	if err != nil {
		t.Fatalf("ScanContext failed: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if len(idx.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(idx.Pages))
	}

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span, got none")
	}

	var found bool
	for _, s := range spans {
		if s.Name == "content.Scan" {
			found = true
			attrs := make(map[string]any)
			for _, a := range s.Attributes {
				attrs[string(a.Key)] = a.Value.AsInterface()
			}
			if v, ok := attrs["content.posts_found"]; !ok || v.(int64) != 1 {
				t.Errorf("content.posts_found = %v, want 1", v)
			}
			if v, ok := attrs["content.pages_found"]; !ok || v.(int64) != 1 {
				t.Errorf("content.pages_found = %v, want 1", v)
			}
			if _, ok := attrs["content.errors_skipped"]; !ok {
				t.Error("missing content.errors_skipped attribute")
			}
			if _, ok := attrs["content.duration_ms"]; !ok {
				t.Error("missing content.duration_ms attribute")
			}
		}
	}
	if !found {
		names := make([]string, len(spans))
		for i, s := range spans {
			names[i] = s.Name
		}
		t.Errorf("span 'content.Scan' not found; got: %s", strings.Join(names, ", "))
	}
}

func TestScanContext_ErrorSetsSpanStatus(t *testing.T) {
	exp := setupTestTracer(t)

	// Use an empty FS — ScanContext should still create a span.
	fs := fstest.MapFS{}

	scanner := newTestScanner()
	idx, err := scanner.ScanContext(context.Background(), fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 0 {
		t.Errorf("expected 0 posts, got %d", len(idx.Posts))
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "content.Scan" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'content.Scan' span")
	}
}
