package handlers_test

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/server/handlers"
)

// TestListHandler_ConfigReload verifies that after SetConfig, the list
// handler serves the updated site title.
func TestListHandler_ConfigReload(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
		makePost("b", "Beta", nil),
		makePost("c", "Gamma", nil),
	}
	deps := testDeps(t, posts, nil)

	// Initial request — testDeps sets PostsPerPage=2, so 3 posts → 2 pages.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	initial := rec.Body.String()
	if !strings.Contains(initial, "posts=2|page=1|total=2") {
		t.Fatalf("expected 2-per-page pagination in initial response, got: %s", initial)
	}

	// Swap config: PostsPerPage 2→1 changes pagination from 2 pages to 3.
	newCfg := config.Default()
	newCfg.Content.PostsPerPage = 1
	deps.SetConfig(newCfg)

	// Second request should reflect the new pagination (1 post per page, 3 total pages).
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec2, req2)

	body := rec2.Body.String()
	if !strings.Contains(body, "posts=1|page=1|total=3") {
		t.Errorf("expected pagination change after config reload, got: %s", body)
	}
}

// TestFeedHandler_ConfigReload verifies that the feed handler picks up
// a config change (site title + base URL) after SetConfig.
func TestFeedHandler_ConfigReload(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(2)
	deps := handlers.NewDeps(cfg, idx, nil)
	h := handlers.NewFeedHandler(deps)

	// Initial request.
	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var feed1 handlers.AtomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed1); err != nil {
		t.Fatalf("invalid Atom XML: %v", err)
	}
	if feed1.Title != "Test Blog" {
		t.Fatalf("initial title = %q, want %q", feed1.Title, "Test Blog")
	}

	// Reload config with new title and base URL.
	newCfg := testConfig()
	newCfg.Site.Title = "Updated Blog"
	newCfg.Site.BaseURL = "https://new.example.com"
	deps.SetConfig(newCfg)

	// Second request should reflect updated values.
	req2 := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	var feed2 handlers.AtomFeed
	if err := xml.Unmarshal(rec2.Body.Bytes(), &feed2); err != nil {
		t.Fatalf("invalid Atom XML: %v", err)
	}
	if feed2.Title != "Updated Blog" {
		t.Errorf("title after reload = %q, want %q", feed2.Title, "Updated Blog")
	}
	if feed2.ID != "https://new.example.com/" {
		t.Errorf("ID after reload = %q, want %q", feed2.ID, "https://new.example.com/")
	}
}

// TestSitemapHandler_ConfigReload verifies that the sitemap handler
// picks up a base URL change after SetConfig.
func TestSitemapHandler_ConfigReload(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(1)
	deps := handlers.NewDeps(cfg, idx, nil)
	h := handlers.NewSitemapHandler(deps)

	// Initial request.
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var sm1 handlers.URLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &sm1); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	if sm1.URLs[0].Loc != "https://example.com/" {
		t.Fatalf("initial home loc = %q", sm1.URLs[0].Loc)
	}

	// Reload config with new base URL.
	newCfg := testConfig()
	newCfg.Site.BaseURL = "https://reloaded.example.com"
	deps.SetConfig(newCfg)

	// Second request should reflect updated base URL.
	req2 := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	var sm2 handlers.URLSet
	if err := xml.Unmarshal(rec2.Body.Bytes(), &sm2); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	if sm2.URLs[0].Loc != "https://reloaded.example.com/" {
		t.Errorf("home loc after reload = %q, want %q", sm2.URLs[0].Loc, "https://reloaded.example.com/")
	}
}
