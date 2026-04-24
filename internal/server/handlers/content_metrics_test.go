package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// spanFromRecorder starts a span, runs fn with the span's context injected
// into the request, and returns the finished span for attribute assertions.
func spanFromRecorder(t *testing.T, r *http.Request, fn func(*http.Request)) tracetest.SpanStub {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(r.Context(), "test-span")
	defer span.End()

	fn(r.WithContext(ctx))

	span.End()
	stubs := exporter.GetSpans()
	if len(stubs) == 0 {
		t.Fatal("expected at least one span")
	}
	return stubs[0]
}

func TestRecordContentView_PostCounter(t *testing.T) {
	before := testutil.ToFloat64(contentViewsTotal.WithLabelValues("post", "hello"))

	req := httptest.NewRequest(http.MethodGet, "/posts/hello", nil)
	RecordContentView(req, "post", "hello", "Hello World", []string{"go", "blog"})

	after := testutil.ToFloat64(contentViewsTotal.WithLabelValues("post", "hello"))
	if after-before != 1 {
		t.Errorf("expected counter to increment by 1, got delta %f", after-before)
	}
}

func TestRecordContentView_PageCounter(t *testing.T) {
	before := testutil.ToFloat64(contentViewsTotal.WithLabelValues("page", "about"))

	req := httptest.NewRequest(http.MethodGet, "/pages/about", nil)
	RecordContentView(req, "page", "about", "About Me", nil)

	after := testutil.ToFloat64(contentViewsTotal.WithLabelValues("page", "about"))
	if after-before != 1 {
		t.Errorf("expected counter to increment by 1, got delta %f", after-before)
	}
}

func TestRecordContentView_TagCounter(t *testing.T) {
	before := testutil.ToFloat64(contentViewsTotal.WithLabelValues("tag", "golang"))

	req := httptest.NewRequest(http.MethodGet, "/tags/golang", nil)
	RecordContentView(req, "tag", "golang", "", nil)

	after := testutil.ToFloat64(contentViewsTotal.WithLabelValues("tag", "golang"))
	if after-before != 1 {
		t.Errorf("expected counter to increment by 1, got delta %f", after-before)
	}
}

func TestRecordContentView_ListCounter(t *testing.T) {
	before := testutil.ToFloat64(contentViewsTotal.WithLabelValues("list", ""))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	RecordContentView(req, "list", "", "Posts", nil)

	after := testutil.ToFloat64(contentViewsTotal.WithLabelValues("list", ""))
	if after-before != 1 {
		t.Errorf("expected counter to increment by 1, got delta %f", after-before)
	}
}

func TestRecordContentView_MultipleIncrements(t *testing.T) {
	// Use a unique slug to avoid interference from other tests.
	slug := "multi-test-post"
	before := testutil.ToFloat64(contentViewsTotal.WithLabelValues("post", slug))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/posts/"+slug, nil)
		RecordContentView(req, "post", slug, "Multi Test", nil)
	}

	after := testutil.ToFloat64(contentViewsTotal.WithLabelValues("post", slug))
	if after-before != 5 {
		t.Errorf("expected counter to increment by 5, got delta %f", after-before)
	}
}

func TestRecordContentView_SpanAttributes_Post(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/posts/hello", nil)
	stub := spanFromRecorder(t, req, func(r *http.Request) {
		RecordContentView(r, "post", "hello-span", "Hello Span", []string{"go", "testing"})
	})

	assertAttr(t, stub.Attributes, "content.type", "post")
	assertAttr(t, stub.Attributes, "content.slug", "hello-span")
	assertAttr(t, stub.Attributes, "content.title", "Hello Span")
	assertSliceAttr(t, stub.Attributes, "content.tags", []string{"go", "testing"})
}

func TestRecordContentView_SpanAttributes_NoTags(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/pages/about", nil)
	stub := spanFromRecorder(t, req, func(r *http.Request) {
		RecordContentView(r, "page", "about-span", "About Span", nil)
	})

	assertAttr(t, stub.Attributes, "content.type", "page")
	assertAttr(t, stub.Attributes, "content.slug", "about-span")
	assertAttr(t, stub.Attributes, "content.title", "About Span")
	assertNoAttr(t, stub.Attributes, "content.tags")
}

func TestRecordContentView_SpanAttributes_EmptySlug(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	stub := spanFromRecorder(t, req, func(r *http.Request) {
		RecordContentView(r, "list", "", "Posts", nil)
	})

	assertAttr(t, stub.Attributes, "content.type", "list")
	assertAttr(t, stub.Attributes, "content.slug", "")
	assertAttr(t, stub.Attributes, "content.title", "Posts")
	assertNoAttr(t, stub.Attributes, "content.tags")
}

// assertAttr checks that the attribute list contains the given key with the
// expected string value.
func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, want string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if got := a.Value.AsString(); got != want {
				t.Errorf("attribute %s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}

// assertSliceAttr checks that the attribute list contains the given key with
// the expected string slice value.
func assertSliceAttr(t *testing.T, attrs []attribute.KeyValue, key string, want []string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			got := a.Value.AsStringSlice()
			if len(got) != len(want) {
				t.Errorf("attribute %s has %d elements, want %d", key, len(got), len(want))
				return
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("attribute %s[%d] = %q, want %q", key, i, got[i], want[i])
				}
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}

// assertNoAttr checks that the attribute key is not present.
func assertNoAttr(t *testing.T, attrs []attribute.KeyValue, key string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			t.Errorf("attribute %s should not be present, got %v", key, a.Value)
			return
		}
	}
}
