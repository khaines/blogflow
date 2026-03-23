package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const defaultSidecarDir = "/data/content"

// SidecarStrategy watches for git-sync sidecar symlink swaps.
// git-sync atomically swaps a symlink in a shared volume; this strategy
// detects those swaps via fsnotify and triggers a content reload.
type SidecarStrategy struct {
	reloader ContentReloader
	logger   *slog.Logger
	dir      string
	debounce time.Duration
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
	done     chan struct{}
}

// NewSidecarStrategy creates a new sidecar-based sync strategy.
// The watched directory defaults to /data/content; call SetDir to override.
func NewSidecarStrategy(reloader ContentReloader, logger *slog.Logger) *SidecarStrategy {
	return &SidecarStrategy{
		reloader: reloader,
		logger:   logger,
		dir:      defaultSidecarDir,
		debounce: 500 * time.Millisecond,
		done:     make(chan struct{}),
	}
}

// SetDir configures the directory to watch for symlink swaps.
// Must be called before Start.
func (w *SidecarStrategy) SetDir(dir string) { w.dir = dir }

// Start begins watching for sidecar symlink swaps.
// It returns promptly; background work runs in a separate goroutine.
func (w *SidecarStrategy) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	if w.dir == "" {
		return fmt.Errorf("gitops: sidecar strategy has no directory configured — call SetDir before Start")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("gitops: create sidecar watcher: %w", err)
	}

	if err := watcher.Add(w.dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("gitops: sidecar watch %q: %w", w.dir, err)
	}

	w.watcher = watcher
	w.started = true
	w.logger.Info("sidecar strategy started", "dir", w.dir)
	go w.loop(ctx)
	return nil
}

// Stop gracefully shuts down the sidecar watcher.
func (w *SidecarStrategy) Stop(ctx context.Context) error {
	w.mu.Lock()
	watcher := w.watcher
	w.mu.Unlock()

	if watcher == nil {
		return nil
	}

	var stopErr error
	w.stopOnce.Do(func() {
		if err := watcher.Close(); err != nil {
			stopErr = fmt.Errorf("gitops: close sidecar watcher: %w", err)
		}
		select {
		case <-w.done:
		case <-ctx.Done():
			stopErr = ctx.Err()
		}
		w.logger.Info("sidecar strategy stopped")
	})
	return stopErr
}

// Name returns the strategy name.
func (w *SidecarStrategy) Name() string { return "sidecar" }

// isSymlinkEvent returns true for events that signal an atomic symlink swap.
func isSymlinkEvent(op fsnotify.Op) bool {
	return op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
}

func (w *SidecarStrategy) loop(ctx context.Context) {
	defer close(w.done)

	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if !isSymlinkEvent(event.Op) {
				continue
			}

			w.logger.Debug("symlink event detected", "op", event.Op, "path", event.Name)

			if timer == nil {
				timer = time.NewTimer(w.debounce)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.debounce)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("sidecar watcher error", "err", err)

		case <-timerC:
			w.logger.Info("symlink swap detected, reloading content")
			if err := w.reloader(); err != nil {
				w.logger.Error("content reload failed", "err", err)
			}
			timer = nil
			timerC = nil
		}
	}
}
