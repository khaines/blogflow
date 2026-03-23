package gitops

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchedExtensions are the file extensions that trigger a content reload.
var watchedExtensions = map[string]bool{
	".md":   true,
	".html": true,
	".css":  true,
	".yaml": true,
}

// WatchStrategy monitors filesystem directories for changes using fsnotify.
// On change, it debounces events and calls the ContentReloader.
type WatchStrategy struct {
	reloader ContentReloader
	logger   *slog.Logger
	dirs     []string
	debounce time.Duration
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
	done     chan struct{}
}

// NewWatchStrategy creates a file watcher strategy.
// dirs are the directories to recursively watch for changes.
func NewWatchStrategy(reloader ContentReloader, logger *slog.Logger, dirs ...string) *WatchStrategy {
	return &WatchStrategy{
		reloader: reloader,
		logger:   logger,
		dirs:     dirs,
		debounce: 500 * time.Millisecond,
		done:     make(chan struct{}),
	}
}

// SetDirs configures the directories to watch. Must be called before Start
// when the strategy is constructed without directories (e.g. via the factory).
func (w *WatchStrategy) SetDirs(dirs ...string) { w.dirs = dirs }

// Start begins watching the filesystem for content changes.
// It returns promptly; background work runs in a separate goroutine.
// Start may be retried after a transient failure (e.g. missing dirs).
func (w *WatchStrategy) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	if len(w.dirs) == 0 {
		return fmt.Errorf("gitops: watch strategy has no directories configured — call SetDirs before Start")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("gitops: create watcher: %w", err)
	}

	for _, dir := range w.dirs {
		if err := addRecursive(watcher, dir); err != nil {
			watcher.Close()
			return fmt.Errorf("gitops: watch %q: %w", dir, err)
		}
	}

	w.watcher = watcher
	w.started = true
	go w.loop(ctx)
	return nil
}

// Stop gracefully shuts down the filesystem watcher.
func (w *WatchStrategy) Stop(ctx context.Context) error {
	var stopErr error
	w.stopOnce.Do(func() {
		if w.watcher == nil {
			return
		}
		if err := w.watcher.Close(); err != nil {
			stopErr = fmt.Errorf("gitops: close watcher: %w", err)
		}
		select {
		case <-w.done:
		case <-ctx.Done():
			stopErr = ctx.Err()
		}
	})
	return stopErr
}

// Name returns the strategy name.
func (w *WatchStrategy) Name() string { return "watch" }

func (w *WatchStrategy) loop(ctx context.Context) {
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
			_ = w.watcher.Close()
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Watch newly created directories.
			if event.Op&fsnotify.Create != 0 {
				w.tryWatchDir(event.Name)
			}

			if !isWatchedFile(event.Name) {
				continue
			}

			w.logger.Debug("file event", "op", event.Op, "path", event.Name)

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
			w.logger.Error("watcher error", "err", err)

		case <-timerC:
			w.logger.Info("content change detected, reloading")
			if err := w.reloader(); err != nil {
				w.logger.Error("reload failed", "err", err)
			}
			timer = nil
			timerC = nil
		}
	}
}

// tryWatchDir adds a newly created directory to the watcher.
func (w *WatchStrategy) tryWatchDir(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	if isIgnoredDir(filepath.Base(path)) {
		return
	}
	if err := addRecursive(w.watcher, path); err != nil {
		w.logger.Debug("could not watch new directory", "path", path, "err", err)
	}
}

// addRecursive walks a directory tree and adds all subdirectories to the watcher.
func addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if isIgnoredDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}

// isWatchedFile returns true if the file should trigger a reload.
func isWatchedFile(name string) bool {
	if isIgnoredFile(name) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	return watchedExtensions[ext]
}

// isIgnoredFile returns true for temporary/swap files and .git paths.
func isIgnoredFile(name string) bool {
	base := filepath.Base(name)
	if strings.HasSuffix(base, "~") {
		return true
	}
	ext := filepath.Ext(base)
	if ext == ".swp" || ext == ".tmp" {
		return true
	}
	return containsDotGit(name)
}

// ignoredDirs lists directory base names that should never be watched.
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".cache":       true,
	".idea":        true,
	".vscode":      true,
	"dist":         true,
	"_site":        true,
}

// isIgnoredDir returns true for directories that should not be watched.
func isIgnoredDir(name string) bool {
	return ignoredDirs[name]
}

// containsDotGit checks if any path component is ".git".
func containsDotGit(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}
