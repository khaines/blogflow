package handlers_test

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
	"github.com/kenhaines/blogflow/internal/server/handlers"
)

func testConfig() *config.Config {
	cfg := config.Default()
	cfg.Site.Title = "Test Blog"
	cfg.Site.Description = "A test blog"
	cfg.Site.BaseURL = "https://example.com"
	cfg.Site.Author.Name = "Alice"
	cfg.Site.Author.Email = "alice@example.com"
	cfg.Feed.Items = 20
	cfg.Feed.Type = "atom"
	return cfg
}

func testIndex(n int) *content.Index {
	idx := &content.Index{
		BySlug:     make(map[string]*content.Post),
		ByTag:      make(map[string][]*content.Post),
		ByYear:     make(map[int][]*content.Post),
		PageBySlug: make(map[string]*content.Post),
	}
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := range n {
		p := &content.Post{
			Slug:    fmt.Sprintf("post-%d", i+1),
			Summary: fmt.Sprintf("Summary for post %d", i+1),
			Content: template.HTML(fmt.Sprintf("<p>Content %d</p>", i+1)), //nolint:gosec
		}
		p.Title = fmt.Sprintf("Post %d", i+1)
		p.Date = base.AddDate(0, 0, -i)
		idx.Posts = append(idx.Posts, p)
		idx.BySlug[p.Slug] = p
	}
	return idx
}

func TestFeedHandler_Atom(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(3)
	h := handlers.NewFeedHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/atom+xml; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	var feed handlers.AtomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid Atom XML: %v", err)
	}
	if feed.Title != "Test Blog" {
		t.Errorf("title = %q, want %q", feed.Title, "Test Blog")
	}
	if feed.Author.Name != "Alice" {
		t.Errorf("author = %q, want %q", feed.Author.Name, "Alice")
	}
	if len(feed.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(feed.Entries))
	}
	if feed.Entries[0].Title != "Post 1" {
		t.Errorf("first entry title = %q, want %q", feed.Entries[0].Title, "Post 1")
	}
	if feed.Entries[0].Link.Href != "https://example.com/posts/post-1" {
		t.Errorf("first entry link = %q", feed.Entries[0].Link.Href)
	}
	if len(feed.Links) != 2 {
		t.Fatalf("feed links = %d, want 2", len(feed.Links))
	}
	if feed.Links[0].Rel != "self" || feed.Links[0].Type != "application/atom+xml" {
		t.Errorf("self link = %+v", feed.Links[0])
	}
	if feed.Links[1].Rel != "alternate" || feed.Links[1].Type != "text/html" {
		t.Errorf("alternate link = %+v", feed.Links[1])
	}
}

func TestFeedHandler_RSS(t *testing.T) {
	cfg := testConfig()
	cfg.Feed.Type = "rss"
	idx := testIndex(3)
	h := handlers.NewFeedHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/rss+xml; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}

	var feed handlers.RSSFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid RSS XML: %v", err)
	}
	if feed.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", feed.Version)
	}
	if feed.Channel.Title != "Test Blog" {
		t.Errorf("channel title = %q, want %q", feed.Channel.Title, "Test Blog")
	}
	if feed.Channel.Link != "https://example.com" {
		t.Errorf("channel link = %q", feed.Channel.Link)
	}
	if len(feed.Channel.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(feed.Channel.Items))
	}
	for i, item := range feed.Channel.Items {
		wantLink := fmt.Sprintf("https://example.com/posts/post-%d", i+1)
		if item.Link != wantLink {
			t.Errorf("item[%d] link = %q, want %q", i, item.Link, wantLink)
		}
		if item.GUID != wantLink {
			t.Errorf("item[%d] guid = %q, want %q", i, item.GUID, wantLink)
		}
		if item.PubDate == "" {
			t.Errorf("item[%d] pubDate is empty", i)
		}
		// Validate RFC1123Z format by parsing.
		if _, err := time.Parse(time.RFC1123Z, item.PubDate); err != nil {
			t.Errorf("item[%d] pubDate %q is not RFC1123Z: %v", i, item.PubDate, err)
		}
	}
}

func TestFeedHandler_EmptyIndex(t *testing.T) {
	cfg := testConfig()
	idx := testIndex(0)
	h := handlers.NewFeedHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var feed handlers.AtomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid Atom XML: %v", err)
	}
	if len(feed.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(feed.Entries))
	}
}

func TestFeedHandler_LimitItems(t *testing.T) {
	cfg := testConfig()
	cfg.Feed.Items = 2
	idx := testIndex(5)
	h := handlers.NewFeedHandler(cfg, idx)

	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var feed handlers.AtomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid Atom XML: %v", err)
	}
	if len(feed.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(feed.Entries))
	}
}
