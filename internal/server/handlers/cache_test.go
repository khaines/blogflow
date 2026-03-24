package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/server/handlers"
)

// --------------- Feed caching tests ---------------

func TestFeedHandler_CacheHeaders(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", cc, "public, max-age=3600")
	}
	if etag := rec.Header().Get("ETag"); etag == "" {
		t.Error("ETag header missing")
	}
	if lm := rec.Header().Get("Last-Modified"); lm == "" {
		t.Error("Last-Modified header missing")
	} else if _, err := http.ParseTime(lm); err != nil {
		t.Errorf("Last-Modified is not a valid HTTP date: %q", lm)
	}
}

func TestFeedHandler_ETag304(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	// First request to obtain ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag header missing from initial response")
	}

	// Conditional request with If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Error("expected empty body on 304")
	}
}

func TestFeedHandler_IfModifiedSince304(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	// Newest post date is 2025-01-15 12:00:00 UTC (from testIndex).
	future := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req.Header.Set("If-Modified-Since", future.UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec.Code)
	}
}

func TestFeedHandler_IfModifiedSince200WhenNewer(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	// Date well before the newest post.
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req.Header.Set("If-Modified-Since", old.UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --------------- Sitemap caching tests ---------------

func TestSitemapHandler_CacheHeaders(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(2)
	h := handlers.NewSitemapHandler(handlers.NewDeps(cfg, idx, nil))

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", cc, "public, max-age=3600")
	}
	if etag := rec.Header().Get("ETag"); etag == "" {
		t.Error("ETag header missing")
	}
	if lm := rec.Header().Get("Last-Modified"); lm == "" {
		t.Error("Last-Modified header missing")
	}
}

func TestSitemapHandler_ETag304(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(2)
	h := handlers.NewSitemapHandler(handlers.NewDeps(cfg, idx, nil))

	// First request to obtain ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag header missing from initial response")
	}

	// Conditional request.
	req2 := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec2.Code)
	}
}

func TestSitemapHandler_IfModifiedSince304(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(2)
	h := handlers.NewSitemapHandler(handlers.NewDeps(cfg, idx, nil))

	future := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.Header.Set("If-Modified-Since", future.UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec.Code)
	}
}

// --------------- RFC 7232 compliance tests ---------------

func TestFeedHandler_IfNoneMatchMultiValue(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	// Get the real ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	etag := rec1.Header().Get("ETag")

	// Send multi-value If-None-Match containing the real ETag.
	req2 := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req2.Header.Set("If-None-Match", `"bogus", `+etag+`, "other"`)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for multi-value If-None-Match, got %d", rec2.Code)
	}
}

func TestFeedHandler_IfNoneMatchWildcard(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req.Header.Set("If-None-Match", "*")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for wildcard If-None-Match, got %d", rec.Code)
	}
}

func TestFeedHandler_IfNoneMatchMiss_IgnoresIfModifiedSince(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(handlers.NewDeps(cfg, idx, nil))

	// Send mismatched ETag AND a future If-Modified-Since.
	// Per RFC 7232 §3.3, If-Modified-Since MUST be ignored when
	// If-None-Match is present. So the response MUST be 200.
	future := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req.Header.Set("If-None-Match", `"wrong-etag"`)
	req.Header.Set("If-Modified-Since", future.UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (ETag miss ignores IMS), got %d", rec.Code)
	}
}
