package gitops

import (
	"context"
	"log/slog"
)

// WatchStrategy monitors the filesystem for changes using fsnotify.
type WatchStrategy struct {
	reloader ContentReloader
	logger   *slog.Logger
}

// NewWatchStrategy creates a new filesystem watch strategy.
func NewWatchStrategy(reloader ContentReloader, logger *slog.Logger) *WatchStrategy {
	return &WatchStrategy{reloader: reloader, logger: logger}
}

// Start begins watching the filesystem for content changes.
// TODO: fsnotify integration
func (w *WatchStrategy) Start(ctx context.Context) error {
	w.logger.Info("watch strategy started")
	return nil
}

// Stop gracefully shuts down the filesystem watcher.
func (w *WatchStrategy) Stop() error {
	w.logger.Info("watch strategy stopped")
	return nil
}

// Name returns the strategy name.
func (w *WatchStrategy) Name() string { return "watch" }
