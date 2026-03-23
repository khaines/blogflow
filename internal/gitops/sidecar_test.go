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

func sidecarTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSidecarStrategy_SymlinkSwapTriggersReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create initial symlink target and symlink.
	target1 := filepath.Join(dir, "repo-v1")
	if err := os.Mkdir(target1, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "current")
	if err := os.Symlink(target1, link); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	s := NewSidecarStrategy(reloader, sidecarTestLogger())
	s.SetDir(dir)
	s.debounce = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	// Simulate atomic symlink swap (remove old, create new).
	target2 := filepath.Join(dir, "repo-v2")
	if err := os.Mkdir(target2, 0o750); err != nil {
		t.Fatal(err)
	}
	newLink := filepath.Join(dir, "current.tmp")
	if err := os.Symlink(target2, newLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newLink, link); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := called.Load(); got < 1 {
		t.Errorf("reloader called %d times, want >= 1", got)
	}
}

func TestSidecarStrategy_Debounce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var called atomic.Int32
	reloader := ContentReloader(func() error {
		called.Add(1)
		return nil
	})

	s := NewSidecarStrategy(reloader, sidecarTestLogger())
	s.SetDir(dir)
	s.debounce = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	// Rapid symlink-like events: create and remove files quickly.
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("current-%d", i))
		if err := os.WriteFile(name, []byte("v"), 0o644); err != nil { //nolint:gosec // G306: test files
			t.Fatal(err)
		}
		_ = os.Remove(name)
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce + buffer.
	time.Sleep(500 * time.Millisecond)

	if got := called.Load(); got != 1 {
		t.Errorf("reloader called %d times, want 1", got)
	}
}

func TestSidecarStrategy_ContextCancel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s := NewSidecarStrategy(func() error { return nil }, sidecarTestLogger())
	s.SetDir(dir)

	ctx, cancel := context.WithCancel(context.Background())

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after context cancel")
	}

	// Stop should be safe even after context cancel.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarStrategy_StopIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s := NewSidecarStrategy(func() error { return nil }, sidecarTestLogger())
	s.SetDir(dir)

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// First stop.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit within 2 seconds")
	}

	// Second stop should not error.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarStrategy_StopBeforeStart(t *testing.T) {
	t.Parallel()

	s := NewSidecarStrategy(func() error { return nil }, sidecarTestLogger())

	// Stop without Start should be safe.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarStrategy_Name(t *testing.T) {
	t.Parallel()

	s := NewSidecarStrategy(func() error { return nil }, sidecarTestLogger())
	if got := s.Name(); got != "sidecar" {
		t.Errorf("Name() = %q, want %q", got, "sidecar")
	}
}

func TestSidecarStrategy_StartErrorNoDir(t *testing.T) {
	t.Parallel()

	s := NewSidecarStrategy(func() error { return nil }, sidecarTestLogger())
	s.SetDir("")

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when starting with empty dir")
	}
}
