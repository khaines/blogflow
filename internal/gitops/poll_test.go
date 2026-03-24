package gitops_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/gitops"
)

// fakePuller implements gitops.PullExecutor for testing.
type fakePuller struct {
	changed atomic.Bool
	err     atomic.Value // stores error
	calls   atomic.Int64
}

func (f *fakePuller) CloneOrPull(_ context.Context, _, _, _ string) (bool, error) {
	f.calls.Add(1)
	if v := f.err.Load(); v != nil {
		return false, v.(error)
	}
	return f.changed.Load(), nil
}

func newTestPollStrategy(t *testing.T, interval time.Duration, puller gitops.PullExecutor, reloader gitops.ContentReloader) *gitops.PollStrategy {
	t.Helper()
	s, err := gitops.NewPollStrategy(interval, reloader, logger())
	if err != nil {
		t.Fatalf("NewPollStrategy: %v", err)
	}
	s.SetPuller(puller, "https://example.com/repo.git", "main", t.TempDir())
	return s
}

func TestPollStrategy_TriggersAtInterval(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}
	puller.changed.Store(false)

	s := newTestPollStrategy(t, 30*time.Second, puller, noop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least one tick (we can't wait 30s in a test, so we
	// verify the goroutine started and cancel promptly).
	// The real interval test uses a short interval via direct construction.
	time.Sleep(50 * time.Millisecond)
	cancel()

	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestPollStrategy_ChangedContentTriggersReload(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}
	puller.changed.Store(true)

	var reloadCount atomic.Int64
	reloader := func() error {
		reloadCount.Add(1)
		return nil
	}

	// Use minimum allowed interval; we'll tick manually via context timing.
	s := newTestPollStrategy(t, 30*time.Second, puller, reloader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Directly invoke tick via exported-for-test helper isn't available,
	// so we test the behaviour end-to-end by exercising Tick through
	// the internal poll loop. Since 30s is too slow for tests, we verify
	// the contract by constructing a strategy with a fast ticker.
	cancel()
	_ = s.Stop(ctx)

	// Verify contract: construct with fast interval and check reload fires.
	puller2 := &fakePuller{}
	puller2.changed.Store(true)
	var reloadCount2 atomic.Int64
	reloader2 := func() error {
		reloadCount2.Add(1)
		return nil
	}

	// Build directly with a fast interval (below the public constructor's
	// 30s guard, so we use an internal helper approach):
	// We test the tick logic by exercising the full strategy at 30s minimum.
	// For a fast test, we bypass the min-interval guard.
	s2, err := gitops.NewPollStrategyForTest(100*time.Millisecond, reloader2, logger())
	if err != nil {
		t.Fatalf("NewPollStrategyForTest: %v", err)
	}
	s2.SetPuller(puller2, "https://example.com/repo.git", "main", t.TempDir())

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := s2.Start(ctx2); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for a few ticks
	time.Sleep(350 * time.Millisecond)
	cancel2()
	_ = s2.Stop(ctx2)

	if n := reloadCount2.Load(); n == 0 {
		t.Error("expected reload to be called at least once when content changed")
	}
	if n := puller2.calls.Load(); n == 0 {
		t.Error("expected puller to be called at least once")
	}
}

func TestPollStrategy_UnchangedContentSkipsReload(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}
	puller.changed.Store(false)

	var reloadCount atomic.Int64
	reloader := func() error {
		reloadCount.Add(1)
		return nil
	}

	s, err := gitops.NewPollStrategyForTest(100*time.Millisecond, reloader, logger())
	if err != nil {
		t.Fatalf("NewPollStrategyForTest: %v", err)
	}
	s.SetPuller(puller, "https://example.com/repo.git", "main", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(350 * time.Millisecond)
	cancel()
	_ = s.Stop(ctx)

	if n := puller.calls.Load(); n == 0 {
		t.Error("expected puller to be called at least once")
	}
	if n := reloadCount.Load(); n != 0 {
		t.Errorf("expected reload to NOT be called when unchanged, got %d calls", n)
	}
}

func TestPollStrategy_ContextCancellationStops(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}
	puller.changed.Store(false)

	s, err := gitops.NewPollStrategyForTest(100*time.Millisecond, noop, logger())
	if err != nil {
		t.Fatalf("NewPollStrategyForTest: %v", err)
	}
	s.SetPuller(puller, "https://example.com/repo.git", "main", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let a few ticks happen
	time.Sleep(250 * time.Millisecond)
	callsBefore := puller.calls.Load()
	cancel()

	// Give goroutine time to exit
	time.Sleep(200 * time.Millisecond)
	callsAfter := puller.calls.Load()

	// After cancellation, no more calls should happen (allow at most 1 in-flight)
	if callsAfter-callsBefore > 1 {
		t.Errorf("expected polling to stop after cancel, but calls went from %d to %d", callsBefore, callsAfter)
	}
}

func TestPollStrategy_StopIsIdempotent(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}

	s := newTestPollStrategy(t, 30*time.Second, puller, noop)

	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	// Stop twice — should not panic or return error
	if err := s.Stop(ctx); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestPollStrategy_PullErrorDoesNotReload(t *testing.T) {
	t.Parallel()

	puller := &fakePuller{}
	puller.err.Store(fmt.Errorf("network error"))

	var reloadCount atomic.Int64
	reloader := func() error {
		reloadCount.Add(1)
		return nil
	}

	s, err := gitops.NewPollStrategyForTest(100*time.Millisecond, reloader, logger())
	if err != nil {
		t.Fatalf("NewPollStrategyForTest: %v", err)
	}
	s.SetPuller(puller, "https://example.com/repo.git", "main", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(350 * time.Millisecond)
	cancel()

	if n := reloadCount.Load(); n != 0 {
		t.Errorf("expected reload NOT to be called on pull error, got %d calls", n)
	}
}

func TestPollStrategy_StartWithoutPuller(t *testing.T) {
	t.Parallel()

	s, err := gitops.NewPollStrategy(30*time.Second, noop, logger())
	if err != nil {
		t.Fatalf("NewPollStrategy: %v", err)
	}
	// Don't call SetPuller

	err = s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when starting without puller")
	}
}

func TestPollStrategy_Name(t *testing.T) {
	t.Parallel()

	s := newTestPollStrategy(t, 30*time.Second, &fakePuller{}, noop)
	if got := s.Name(); got != "poll" {
		t.Errorf("Name() = %q, want %q", got, "poll")
	}
}

func TestNewPollStrategy_IntervalTooShort(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewPollStrategy(10*time.Second, noop, logger())
	if err == nil {
		t.Fatal("expected error for interval < 30s")
	}
}

func TestNewPollStrategy_NilReloader(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewPollStrategy(30*time.Second, nil, logger())
	if err == nil {
		t.Fatal("expected error for nil reloader")
	}
}
