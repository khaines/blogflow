package main

import (
	"io/fs"
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// testFS returns a minimal content filesystem for reload tests.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte("---\ntitle: Hello\ndate: 2024-01-01T00:00:00Z\n---\nHello world\n"),
		},
	}
}

// testThemeFS returns a minimal theme filesystem for theme.Engine.
func testThemeFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/base.html": &fstest.MapFile{
			Data: []byte(`{{define "templates/base.html"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/post.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}post{{end}}`),
		},
	}
}

// testConfigFS returns a minimal config filesystem for config.Loader.
func testConfigFS() fstest.MapFS {
	return fstest.MapFS{
		"site.yaml": &fstest.MapFile{
			Data: []byte("site:\n  title: Test Blog\n  base_url: http://localhost\n"),
		},
	}
}

// mergeFS combines two MapFS into one.
func mergeFS(a, b fstest.MapFS) fstest.MapFS {
	m := make(fstest.MapFS, len(a)+len(b))
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

// switchableFS delegates to an underlying fs.FS that can be swapped at runtime.
// When broken is true, Open returns a fs.PathError for every path.
type switchableFS struct {
	inner  fs.FS
	broken bool
}

func (s *switchableFS) Open(name string) (fs.File, error) {
	if s.broken {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return s.inner.Open(name)
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

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)

	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, contentFS, deps, cache, nil, nil, slog.Default())

	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty after reload, got %d entries", cache.Len())
	}
}

func TestContentReloaderNilCache(t *testing.T) {
	t.Parallel()

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)

	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, contentFS, deps, nil, nil, nil, slog.Default())

	// Must not panic with nil cache.
	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}
}

func TestContentReloaderUpdatesIndex(t *testing.T) {
	t.Parallel()

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)

	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, nil)
	reloader := newContentReloader(scanner, contentFS, deps, nil, nil, nil, slog.Default())

	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}

	if deps.LoadIndex() == idx {
		t.Error("expected deps index to be updated to a new pointer after reload")
	}

	if len(deps.LoadIndex().Posts) != 1 {
		t.Errorf("expected 1 post after reload, got %d", len(deps.LoadIndex().Posts))
	}
}

func TestContentReloaderTriggersThemeReload(t *testing.T) {
	t.Parallel()

	combined := mergeFS(testFS(), testThemeFS())
	engine, err := theme.NewEngine(combined)
	if err != nil {
		t.Fatalf("theme.NewEngine: %v", err)
	}

	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)
	idx, err := scanner.Scan(combined)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, engine)
	reloader := newContentReloader(scanner, combined, deps, nil, nil, engine, slog.Default())

	// Theme reload should succeed without error.
	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}
}

func TestContentReloaderTriggersConfigReload(t *testing.T) {
	t.Parallel()

	cfgFS := testConfigFS()
	loader := config.NewLoader(cfgFS, config.WithLogger(slog.Default()))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("initial load: %v", err)
	}

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)
	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(loader.Get(), idx, nil)
	reloader := newContentReloader(scanner, contentFS, deps, nil, loader, nil, slog.Default())

	if err := reloader(); err != nil {
		t.Fatalf("reloader returned error: %v", err)
	}

	cfg := loader.Get()
	if cfg == nil {
		t.Fatal("expected config to be non-nil after reload")
	}
	if cfg.Site.Title != "Test Blog" {
		t.Errorf("expected title 'Test Blog', got %q", cfg.Site.Title)
	}
}

func TestContentReloaderThemeFailureDoesNotBlockContentScan(t *testing.T) {
	t.Parallel()

	// Create a switchable FS that starts valid and can be broken later.
	sfs := &switchableFS{inner: testThemeFS()}
	engine, err := theme.NewEngine(sfs)
	if err != nil {
		t.Fatalf("theme.NewEngine: %v", err)
	}
	// Break the FS so the next Reload() fails.
	sfs.broken = true

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)
	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(nil, idx, engine)
	reloader := newContentReloader(scanner, contentFS, deps, nil, nil, engine, slog.Default())

	// Reloader must NOT return an error — theme failure is logged, not propagated.
	if err := reloader(); err != nil {
		t.Fatalf("reloader should not return error on theme failure, got: %v", err)
	}

	// Content should still be updated despite theme reload failure.
	if len(deps.LoadIndex().Posts) != 1 {
		t.Errorf("expected 1 post after reload, got %d", len(deps.LoadIndex().Posts))
	}
}

func TestContentReloaderConfigFailureDoesNotBlockContentScan(t *testing.T) {
	t.Parallel()

	// Create a switchable FS that starts valid and can be broken later.
	sfs := &switchableFS{inner: testConfigFS()}
	loader := config.NewLoader(sfs, config.WithLogger(slog.Default()))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	// Break the FS so the next Reload() fails.
	sfs.broken = true

	contentFS := testFS()
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, "posts", "pages", 200, nil)
	idx, err := scanner.Scan(contentFS)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	deps := handlers.NewDeps(loader.Get(), idx, nil)
	reloader := newContentReloader(scanner, contentFS, deps, nil, loader, nil, slog.Default())

	// Reloader must NOT return an error — config failure is logged, not propagated.
	if err := reloader(); err != nil {
		t.Fatalf("reloader should not return error on config failure, got: %v", err)
	}

	// Content should still be updated.
	if len(deps.LoadIndex().Posts) != 1 {
		t.Errorf("expected 1 post after reload, got %d", len(deps.LoadIndex().Posts))
	}
}
