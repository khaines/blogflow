package handlers_test

import (
	"encoding/xml"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kenhaines/blogflow/internal/content"
	"github.com/kenhaines/blogflow/internal/server/handlers"
)

func TestSitemapHandler(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(2)

	// Add a page.
	page := &content.Post{
		Slug:    "about",
		Summary: "About page",
		Content: template.HTML("<p>About</p>"), //nolint:gosec
	}
	page.Title = "About"
	page.Date = time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	idx.Pages = append(idx.Pages, page)
	idx.PageBySlug["about"] = page

	h := handlers.NewSitemapHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/xml; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	var urlset handlers.URLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &urlset); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}

	// home + 2 posts + 1 page = 4
	if len(urlset.URLs) != 4 {
		t.Fatalf("urls = %d, want 4", len(urlset.URLs))
	}

	// First URL is home page with lastmod from most recent post.
	if urlset.URLs[0].Loc != "https://example.com/" {
		t.Errorf("home loc = %q", urlset.URLs[0].Loc)
	}
	if urlset.URLs[0].Priority != "1.0" {
		t.Errorf("home priority = %q, want 1.0", urlset.URLs[0].Priority)
	}
	if urlset.URLs[0].LastMod != "2025-01-15" {
		t.Errorf("home lastmod = %q, want 2025-01-15", urlset.URLs[0].LastMod)
	}

	// Posts.
	if urlset.URLs[1].Loc != "https://example.com/posts/post-1" {
		t.Errorf("post loc = %q", urlset.URLs[1].Loc)
	}
	if urlset.URLs[1].LastMod != "2025-01-15" {
		t.Errorf("post-1 lastmod = %q, want 2025-01-15", urlset.URLs[1].LastMod)
	}

	// Page.
	if urlset.URLs[3].Loc != "https://example.com/about" {
		t.Errorf("page loc = %q", urlset.URLs[3].Loc)
	}
}

func TestSitemapHandler_Empty(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(0)
	h := handlers.NewSitemapHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var urlset handlers.URLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &urlset); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	// Only home URL.
	if len(urlset.URLs) != 1 {
		t.Fatalf("urls = %d, want 1", len(urlset.URLs))
	}
	if urlset.URLs[0].Loc != "https://example.com/" {
		t.Errorf("home loc = %q", urlset.URLs[0].Loc)
	}
}
