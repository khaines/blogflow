package gitops

import (
	"context"
	"log/slog"
)

// SidecarStrategy watches for git-sync sidecar symlink swaps.
type SidecarStrategy struct {
	reloader ContentReloader
	logger   *slog.Logger
}

// NewSidecarStrategy creates a new sidecar-based sync strategy.
func NewSidecarStrategy(reloader ContentReloader, logger *slog.Logger) *SidecarStrategy {
	return &SidecarStrategy{reloader: reloader, logger: logger}
}

// Start begins watching for sidecar symlink swaps.
// TODO: symlink detection
func (w *SidecarStrategy) Start(ctx context.Context) error {
	w.logger.Info("sidecar strategy started")
	return nil
}

// Stop gracefully shuts down the sidecar watcher.
func (w *SidecarStrategy) Stop() error {
	w.logger.Info("sidecar strategy stopped")
	return nil
}

// Name returns the strategy name.
func (w *SidecarStrategy) Name() string { return "sidecar" }
