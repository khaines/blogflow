package gitops

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func watchTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWatchStrategy_DetectsFileChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	w := NewWatchStrategy(reloader, watchTestLogger(), dir)
	w.debounce = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "post.md"), []byte("# Hello"), 0o644); err != nil { //nolint:gosec // G306: test files
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := called.Load(); got != 1 {
		t.Errorf("reloader called %d times, want 1", got)
	}
}

func TestWatchStrategy_Debounce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	w := NewWatchStrategy(reloader, watchTestLogger(), dir)
	w.debounce = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("post%d.md", i))
		if err := os.WriteFile(name, []byte("content"), 0o644); err != nil { //nolint:gosec // G306: test files
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce + buffer.
	time.Sleep(500 * time.Millisecond)

	if got := called.Load(); got != 1 {
		t.Errorf("reloader called %d times, want 1", got)
	}
}

func TestWatchStrategy_IgnoresTempFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	w := NewWatchStrategy(reloader, watchTestLogger(), dir)
	w.debounce = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	_ = os.WriteFile(filepath.Join(dir, ".post.md.swp"), []byte("swap"), 0o644) //nolint:gosec // G306: test files
	_ = os.WriteFile(filepath.Join(dir, "draft.tmp"), []byte("temp"), 0o644)    //nolint:gosec // G306: test files
	_ = os.WriteFile(filepath.Join(dir, "backup.md~"), []byte("backup"), 0o644) //nolint:gosec // G306: test files

	time.Sleep(300 * time.Millisecond)

	if got := called.Load(); got != 0 {
		t.Errorf("reloader called %d times, want 0", got)
	}
}

func TestWatchStrategy_StopClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w := NewWatchStrategy(func() error { return nil }, watchTestLogger(), dir)

	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit within 2 seconds")
	}
}

func TestWatchStrategy_StartErrorNoDirs(t *testing.T) {
	t.Parallel()

	w := NewWatchStrategy(func() error { return nil }, watchTestLogger())

	err := w.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when starting with no dirs")
	}
	if got := err.Error(); got != "gitops: watch strategy has no directories configured — call SetDirs before Start" {
		t.Fatalf("unexpected error: %s", got)
	}

	// After SetDirs, Start should succeed (retryable).
	dir := t.TempDir()
	w.SetDirs(dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start after SetDirs failed: %v", err)
	}
	defer func() { _ = w.Stop(context.Background()) }()
}

func TestWatchStrategy_ContextCancel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w := NewWatchStrategy(func() error { return nil }, watchTestLogger(), dir)

	ctx, cancel := context.WithCancel(context.Background())

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after context cancel")
	}

	// Stop should be safe even after context cancel.
	if err := w.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWatchStrategy_RecursiveSubdir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	w := NewWatchStrategy(reloader, watchTestLogger(), dir)
	w.debounce = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	// Let the watcher settle before creating the subdirectory.
	time.Sleep(100 * time.Millisecond)

	sub := filepath.Join(dir, "posts")
	if err := os.Mkdir(sub, 0o750); err != nil { //nolint:gosec // G301: test directory
		t.Fatal(err)
	}

	// Give the watcher time to pick up the new directory.
	time.Sleep(200 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(sub, "new-post.md"), []byte("# New Post"), 0o644); err != nil { //nolint:gosec // G306: test files
		t.Fatal(err)
	}

	// Wait for the debounce to fire.
	deadline := time.After(3 * time.Second)
	for called.Load() < 1 {
		select {
		case <-deadline:
			t.Fatalf("reloader not called within timeout; called %d times, want ≥1", called.Load())
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestWatchStrategy_ConcurrentStartStop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w := NewWatchStrategy(func() error { return nil }, watchTestLogger(), dir)

	// Run with -race to detect the data race on w.watcher.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = w.Start(context.Background())
	}()
	_ = w.Stop(context.Background())
	<-done
}
