package main

import (
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/server/handlers"
)

// testFS returns a minimal content filesystem for reload tests.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte("---\ntitle: Hello\ndate: 2024-01-01T00:00:00Z\n---\nHello world\n"),
		},
	}
}

func TestContentReloaderFlushesCache(t *testing.T) {
	t.Parallel()

	cache := content.NewCache(config.CacheConfig{
		Enabled:    true,
		TTL:        time.Hour,
		MaxEntries: 100,
	})
	cache.Set("post:hello", []byte("<p>stale</p>"))
	cache.Set("post:world", []byte("<p>also stale</p>"))
	if cache.Len() != 2 {
		t.Fatalf("expected 2 cache entries, got %d", cache.Len())
	}

	fs := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200)

	idx, err := scanner.Scan(fs)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, fs, deps, cache, slog.Default())

	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty after reload, got %d entries", cache.Len())
	}
}

func TestContentReloaderNilCache(t *testing.T) {
	t.Parallel()

	fs := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200)

	idx, err := scanner.Scan(fs)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, fs, deps, nil, slog.Default())

	// Must not panic with nil cache.
	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}
}

func TestContentReloaderUpdatesIndex(t *testing.T) {
	t.Parallel()

	fs := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200)

	idx, err := scanner.Scan(fs)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, fs, deps, nil, slog.Default())

	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}

	// deps index should be a fresh index (different pointer).
	if deps.LoadIndex() == idx {
		t.Error("expected deps index to be updated to a new pointer after reload")
	}

	if len(deps.LoadIndex().Posts) != 1 {
		t.Errorf("expected 1 post after reload, got %d", len(deps.LoadIndex().Posts))
	}
}
