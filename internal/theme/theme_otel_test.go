package theme

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupThemeTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

func TestRender_CreatesSpan(t *testing.T) {
	exp := setupThemeTestTracer(t)

	fs := testFS(map[string]string{
		"templates/index.html": `{{define "content"}}<h1>Hello</h1>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	var b strings.Builder
	if err := e.Render(context.Background(), &b, "templates/index.html", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "theme.Render" {
			found = true
			attrs := make(map[string]any)
			for _, a := range s.Attributes {
				attrs[string(a.Key)] = a.Value.AsInterface()
			}
			if v, ok := attrs["theme.template_name"]; !ok || v != "templates/index.html" {
				t.Errorf("theme.template_name = %v, want 'templates/index.html'", v)
			}
		}
	}
	if !found {
		names := make([]string, len(spans))
		for i, s := range spans {
			names[i] = s.Name
		}
		t.Errorf("span 'theme.Render' not found; got: %s", strings.Join(names, ", "))
	}
}

func TestRender_ErrorSetsSpanStatus(t *testing.T) {
	exp := setupThemeTestTracer(t)

	fs := testFS(map[string]string{})
	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	var b strings.Builder
	err = e.Render(context.Background(), &b, "nonexistent.html", nil)
	if err == nil {
		t.Fatal("expected error rendering nonexistent template")
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "theme.Render" {
			found = true
			if s.Status.Code.String() != "Error" {
				t.Errorf("span status = %v, want Error", s.Status.Code)
			}
		}
	}
	if !found {
		t.Error("expected 'theme.Render' span on error path")
	}
}
