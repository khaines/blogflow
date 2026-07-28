package main

import (
	"context"
	"html/template"
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/search"
	"github.com/khaines/blogflow/internal/server/handlers"
)

// discardLogger returns a logger that discards all output, for quiet tests.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discard{}, nil))
}

// TestContentReloader_ScanFailureRetainsSnapshot verifies that when a content
// rescan fails, the previously published snapshot remains active and the
// generation counter does not advance (design §3.2 #14 / §3.3).
func TestContentReloader_ScanFailureRetainsSnapshot(t *testing.T) {
	// Duplicate explicit slug makes scanner.Scan return an error in strict mode.
	badFS := fstest.MapFS{
		"posts/a.md": &fstest.MapFile{Data: []byte("---\ntitle: A\nslug: dup\ndate: 2026-01-01T00:00:00Z\n---\nA")},
		"posts/b.md": &fstest.MapFile{Data: []byte("---\ntitle: B\nslug: dup\ndate: 2026-01-02T00:00:00Z\n---\nB")},
	}
	scanner := content.NewScanner(content.NewRenderer(), "posts", "pages", 200, nil)

	deps := handlers.NewDeps(config.Default(), searchTestIndex(), nil)
	origSnap := deps.LoadSnapshot()
	origGen := origSnap.Generation
	origPosts := deps.PostCount()

	reloader := newContentReloader(scanner, badFS, deps, nil, nil, nil, discardLogger())
	if err := reloader(); err == nil {
		t.Fatal("expected reloader to return an error when content scan fails")
	}

	snap := deps.LoadSnapshot()
	if snap != origSnap {
		t.Error("snapshot pointer changed after a failed scan; the previous snapshot should be retained")
	}
	if snap.Generation != origGen {
		t.Errorf("generation advanced after failed scan: %d -> %d", origGen, snap.Generation)
	}
	if deps.PostCount() != origPosts {
		t.Errorf("post count changed after failed scan: %d -> %d", origPosts, deps.PostCount())
	}
}

func searchTestIndex() *content.Index {
	idx := &content.Index{BySlug: make(map[string]*content.Post)}
	p := &content.Post{Slug: "hello", Content: template.HTML("<p>overlay filesystem</p>")} //nolint:gosec // test data
	p.Title = "Hello overlay"
	p.Date = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	idx.Posts = []*content.Post{p}
	idx.BySlug[p.Slug] = p
	return idx
}

// TestPublishContentSnapshot_BuildsSearchWhenEnabled verifies the wiring helper
// builds and publishes a queryable search index when buildSearch is true.
func TestPublishContentSnapshot_BuildsSearchWhenEnabled(t *testing.T) {
	deps := handlers.NewDeps(config.Default(), &content.Index{}, nil)
	idx := searchTestIndex()
	sc := config.Default().Search
	sc.Enabled = true

	publishContentSnapshot(context.Background(), deps, idx, true, sc, slog.New(slog.NewTextHandler(discard{}, nil)))

	snap := deps.LoadSnapshot()
	if snap.Search == nil {
		t.Fatal("expected non-nil search index when buildSearch is true")
	}
	resp, err := snap.Search.Search(context.Background(), snap.Content, "overlay", search.SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 result, got %d", resp.Total)
	}
}

// TestPublishContentSnapshot_NilSearchWhenDisabled verifies content is published
// with a nil search index when search is not built (router-build disabled).
func TestPublishContentSnapshot_NilSearchWhenDisabled(t *testing.T) {
	deps := handlers.NewDeps(config.Default(), &content.Index{}, nil)
	idx := searchTestIndex()

	publishContentSnapshot(context.Background(), deps, idx, false, config.SearchConfig{}, slog.New(slog.NewTextHandler(discard{}, nil)))

	snap := deps.LoadSnapshot()
	if snap.Search != nil {
		t.Fatal("expected nil search index when buildSearch is false")
	}
	if snap.Content != idx {
		t.Error("content should still be published")
	}
	if deps.PostCount() != 1 {
		t.Errorf("PostCount = %d, want 1", deps.PostCount())
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
