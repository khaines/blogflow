package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Reload ---

func TestReload_PicksUpFileChanges(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Original"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("initial Load failed: %v", err)
	}
	if cfg.Site.Title != "Original" {
		t.Fatalf("expected title 'Original', got %q", cfg.Site.Title)
	}

	// Change the file on disk.
	writeYAML(t, dir, `site:
  title: "Updated"
  base_url: "http://localhost:8080"
`)
	cfg, err = loader.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if cfg.Site.Title != "Updated" {
		t.Errorf("expected title 'Updated' after Reload, got %q", cfg.Site.Title)
	}
	// Get/Config must reflect the new value.
	if loader.Get().Site.Title != "Updated" {
		t.Error("Get() did not return reloaded config")
	}
	if loader.Config().Site.Title != "Updated" {
		t.Error("Config() did not return reloaded config")
	}
}

// --- OnChange ---

func TestOnChange_CallbacksFireAfterReload(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "V1"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	var received []*Config
	loader.OnChange(func(c *Config) { received = append(received, c) })

	// Second callback to prove multiple work.
	var count int
	loader.OnChange(func(_ *Config) { count++ })

	writeYAML(t, dir, `site:
  title: "V2"
  base_url: "http://localhost:8080"
`)
	cfg, err := loader.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 callback invocation, got %d", len(received))
	}
	if received[0].Site.Title != "V2" {
		t.Errorf("callback received wrong title: %q", received[0].Site.Title)
	}
	if received[0] != cfg {
		t.Error("callback config pointer differs from Reload return")
	}
	if count != 1 {
		t.Errorf("second callback expected count=1, got %d", count)
	}
}

func TestOnChange_NotFiredOnLoadOnly(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Init"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))

	var fired bool
	loader.OnChange(func(_ *Config) { fired = true })

	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Error("OnChange should not fire on Load(), only Reload()")
	}
}

// --- Validation errors preserve old config ---

func TestReload_ValidationErrorPreservesOldConfig(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Good"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	var callbackFired bool
	loader.OnChange(func(_ *Config) { callbackFired = true })

	// Write an invalid config (port out of range).
	writeYAML(t, dir, `site:
  title: "Bad"
  base_url: "http://localhost:8080"
server:
  port: 0
`)
	_, err := loader.Reload()
	if err == nil {
		t.Fatal("expected Reload to fail on invalid config")
	}
	// Old config must be preserved.
	if loader.Config().Site.Title != "Good" {
		t.Errorf("expected old title 'Good' preserved, got %q", loader.Config().Site.Title)
	}
	if callbackFired {
		t.Error("OnChange should not fire when Reload fails validation")
	}
}

// --- Watch: debounce ---

func TestWatch_Debounce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Watch test in short mode")
	}

	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Start"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir), WithWatchDir(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32
	loader.OnChange(func(_ *Config) { reloadCount.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchErr := make(chan error, 1)
	go func() { watchErr <- loader.Watch(ctx) }()

	// Give the watcher time to start.
	time.Sleep(100 * time.Millisecond)

	// Rapid successive writes — should debounce into one reload.
	for i := range 5 {
		writeYAML(t, dir, `site:
  title: "Rapid`+string(rune('A'+i))+`"
  base_url: "http://localhost:8080"
`)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce (500ms) + margin.
	time.Sleep(800 * time.Millisecond)

	count := reloadCount.Load()
	if count < 1 {
		t.Error("expected at least 1 reload after rapid writes, got 0")
	}
	if count > 2 {
		t.Errorf("debounce failed: expected ≤2 reloads, got %d", count)
	}

	cancel()
	select {
	case err := <-watchErr:
		if err != nil && err != context.Canceled {
			t.Errorf("Watch returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancellation")
	}
}

// --- Watch: context cancellation ---

func TestWatch_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Watch test in short mode")
	}

	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Cancel"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir), WithWatchDir(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	watchErr := make(chan error, 1)
	go func() { watchErr <- loader.Watch(ctx) }()

	// Give watcher time to start then cancel immediately.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-watchErr:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancellation")
	}
}

// --- Watch: missing watch dir ---

func TestWatch_MissingWatchDir(t *testing.T) {
	loader := NewLoader(os.DirFS(t.TempDir()))
	err := loader.Watch(context.Background())
	if err == nil {
		t.Fatal("expected error when Watch called without WithWatchDir")
	}
}

// --- Config() getter ---

func TestConfig_ReturnsCurrentConfig(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Getter"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	// Before Load, Config returns defaults.
	if loader.Config().Site.Title != "My Blog" {
		t.Errorf("expected default title before Load, got %q", loader.Config().Site.Title)
	}
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}
	if loader.Config().Site.Title != "Getter" {
		t.Errorf("expected 'Getter' after Load, got %q", loader.Config().Site.Title)
	}
}

// --- Concurrent access ---

func TestReload_ConcurrentReadDuringReload(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Concurrent"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	// Spin up readers that continuously call Config().
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range 4 {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					cfg := loader.Config()
					if cfg == nil {
						t.Error("Config() returned nil")
						return
					}
				}
			}
		}()
	}

	// Concurrent reloads while readers are active.
	for range 20 {
		if _, err := loader.Reload(); err != nil {
			t.Fatalf("Reload failed: %v", err)
		}
	}
}

func TestReload_ConcurrentReloads(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `site:
  title: "Race"
  base_url: "http://localhost:8080"
`)
	loader := NewLoader(os.DirFS(dir))
	if _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	var callbackCount atomic.Int32
	loader.OnChange(func(cfg *Config) {
		// Callback must receive non-nil, valid config.
		if cfg == nil {
			t.Error("callback received nil config")
		}
	})
	loader.OnChange(func(_ *Config) { callbackCount.Add(1) })

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				_, _ = loader.Reload()
			}
		}()
	}
	wg.Wait()

	if c := callbackCount.Load(); c != 80 {
		t.Errorf("expected 80 callback invocations, got %d", c)
	}
}

// writeYAML is a test helper that writes site.yaml to dir.
func writeYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing site.yaml: %v", err)
	}
}
