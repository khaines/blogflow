package config

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupConfigTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

func TestReload_CreatesSpan(t *testing.T) {
	exp := setupConfigTestTracer(t)

	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Test"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	_, err := loader.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "config.Reload" {
			found = true
			attrs := make(map[string]any)
			for _, a := range s.Attributes {
				attrs[string(a.Key)] = a.Value.AsInterface()
			}
			if v, ok := attrs["config.path"]; !ok || v != "site.yaml" {
				t.Errorf("config.path = %v, want 'site.yaml'", v)
			}
			if v, ok := attrs["config.success"]; !ok || v != true {
				t.Errorf("config.success = %v, want true", v)
			}
		}
	}
	if !found {
		names := make([]string, len(spans))
		for i, s := range spans {
			names[i] = s.Name
		}
		t.Errorf("span 'config.Reload' not found; got: %s", strings.Join(names, ", "))
	}
}

func TestReload_ErrorSetsSpanStatus(t *testing.T) {
	exp := setupConfigTestTracer(t)

	dir := t.TempDir()
	// Write valid YAML first so Load() succeeds.
	writeYAML(t, dir, `site:
  title: "Test"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	// Write invalid YAML so Reload fails.
	writeYAML(t, dir, `site:
  title: ""
  base_url: "not-a-url"
`)
	_, err := loader.Reload()
	if err == nil {
		t.Fatal("expected Reload to fail with invalid config")
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "config.Reload" {
			found = true
			if s.Status.Code.String() != "Error" {
				t.Errorf("span status = %v, want Error", s.Status.Code)
			}
			attrs := make(map[string]any)
			for _, a := range s.Attributes {
				attrs[string(a.Key)] = a.Value.AsInterface()
			}
			if v, ok := attrs["config.success"]; !ok || v != false {
				t.Errorf("config.success = %v, want false", v)
			}
		}
	}
	if !found {
		t.Error("expected 'config.Reload' span on error path")
	}
}
