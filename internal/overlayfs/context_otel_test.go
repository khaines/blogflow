package overlayfs

import (
	"context"
	"testing"
	"testing/fstest"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupOTelTest installs a test TracerProvider and returns the span recorder.
// It restores the previous provider after the test.
func setupOTelTest(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func findSpan(sr *tracetest.SpanRecorder, name string) (tracetest.SpanStub, bool) {
	for _, s := range tracetest.SpanStubsFromReadOnlySpans(sr.Ended()) {
		if s.Name == name {
			return s, true
		}
	}
	return tracetest.SpanStub{}, false
}

func spanAttr(s tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, a := range s.Attributes {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestOTel_Open_CreatesSpan(t *testing.T) {
	sr := setupOTelTest(t)
	theme := fstest.MapFS{"style.css": {Data: []byte("body{}")}}
	cfs := newTestContextOverlay(nil, theme)

	f, err := cfs.Open(context.Background(), "style.css")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s, ok := findSpan(sr, "overlayfs.open")
	if !ok {
		t.Fatal("expected overlayfs.open span")
	}

	v, ok := spanAttr(s, "path")
	if !ok || v.AsString() != "style.css" {
		t.Errorf("path attr = %q, want %q", v.AsString(), "style.css")
	}

	v, ok = spanAttr(s, "layer.name")
	if !ok {
		t.Fatal("expected layer.name attribute")
	}
	if v.AsString() != "theme" {
		t.Errorf("layer.name = %q, want %q", v.AsString(), "theme")
	}

	v, ok = spanAttr(s, "layer.index")
	if !ok {
		t.Fatal("expected layer.index attribute")
	}
	if v.AsInt64() != 0 {
		t.Errorf("layer.index = %d, want 0", v.AsInt64())
	}
}

func TestOTel_ReadFile_CreatesSpan(t *testing.T) {
	sr := setupOTelTest(t)
	layer := fstest.MapFS{"file.txt": {Data: []byte("hello")}}
	cfs := newTestContextOverlay(nil, layer)

	_, err := cfs.ReadFile(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}

	s, ok := findSpan(sr, "overlayfs.readfile")
	if !ok {
		t.Fatal("expected overlayfs.readfile span")
	}

	v, _ := spanAttr(s, "path")
	if v.AsString() != "file.txt" {
		t.Errorf("path = %q, want %q", v.AsString(), "file.txt")
	}

	v, ok = spanAttr(s, "layer.name")
	if !ok || v.AsString() != "theme" {
		t.Errorf("layer.name = %q, want %q", v.AsString(), "theme")
	}
}

func TestOTel_Stat_CreatesSpan(t *testing.T) {
	sr := setupOTelTest(t)
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	_, err := cfs.Stat(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}

	s, ok := findSpan(sr, "overlayfs.stat")
	if !ok {
		t.Fatal("expected overlayfs.stat span")
	}

	v, _ := spanAttr(s, "path")
	if v.AsString() != "file.txt" {
		t.Errorf("path = %q, want %q", v.AsString(), "file.txt")
	}
}

func TestOTel_ReadDir_CreatesSpan(t *testing.T) {
	sr := setupOTelTest(t)
	layer := fstest.MapFS{"dir/a.txt": {Data: []byte("a")}}
	cfs := newTestContextOverlay(nil, layer)

	_, err := cfs.ReadDir(context.Background(), "dir")
	if err != nil {
		t.Fatal(err)
	}

	s, ok := findSpan(sr, "overlayfs.readdir")
	if !ok {
		t.Fatal("expected overlayfs.readdir span")
	}

	v, _ := spanAttr(s, "path")
	if v.AsString() != "dir" {
		t.Errorf("path = %q, want %q", v.AsString(), "dir")
	}

	// ReadDir should NOT have layer.name since directories span layers.
	if _, ok := spanAttr(s, "layer.name"); ok {
		t.Error("readdir should not have layer.name attribute")
	}
}

func TestOTel_OpenFile_CreatesSpan(t *testing.T) {
	sr := setupOTelTest(t)
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	f, _, err := cfs.OpenFile(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	s, ok := findSpan(sr, "overlayfs.openfile")
	if !ok {
		t.Fatal("expected overlayfs.openfile span")
	}

	v, _ := spanAttr(s, "path")
	if v.AsString() != "file.txt" {
		t.Errorf("path = %q, want %q", v.AsString(), "file.txt")
	}
}

func TestOTel_Open_ErrorSetsSpanStatus(t *testing.T) {
	sr := setupOTelTest(t)
	cfs := newTestContextOverlay(nil, fstest.MapFS{})

	_, err := cfs.Open(context.Background(), "missing.txt")
	if err == nil {
		t.Fatal("expected error")
	}

	s, ok := findSpan(sr, "overlayfs.open")
	if !ok {
		t.Fatal("expected overlayfs.open span")
	}
	if s.Status.Code != codes.Error {
		t.Errorf("status = %v, want Error", s.Status.Code)
	}
}

func TestOTel_ReadFile_FallbackLayerResolution(t *testing.T) {
	sr := setupOTelTest(t)
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"base.html": {Data: []byte("<html>")}}
	cfs := newTestContextOverlay(nil, theme, defaults)

	_, err := cfs.ReadFile(context.Background(), "base.html")
	if err != nil {
		t.Fatal(err)
	}

	s, ok := findSpan(sr, "overlayfs.readfile")
	if !ok {
		t.Fatal("expected overlayfs.readfile span")
	}

	v, ok := spanAttr(s, "layer.name")
	if !ok {
		t.Fatal("expected layer.name attribute")
	}
	if v.AsString() != "content" {
		t.Errorf("layer.name = %q, want %q", v.AsString(), "content")
	}

	v, ok = spanAttr(s, "layer.index")
	if !ok {
		t.Fatal("expected layer.index attribute")
	}
	if v.AsInt64() != 1 {
		t.Errorf("layer.index = %d, want 1", v.AsInt64())
	}
}
