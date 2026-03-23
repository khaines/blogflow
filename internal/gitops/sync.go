// Package gitops provides content synchronization strategies for BlogFlow.
// Supports webhook (push), sidecar (pull/K8s), and watch (local dev) modes.
package gitops

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/khaines/blogflow/internal/config"
)

// ContentReloader is called when content changes are detected.
type ContentReloader func() error

// Strategy defines the interface for content sync mechanisms.
type Strategy interface {
	// Start begins watching/listening for content changes.
	// It returns promptly; background work runs in a separate goroutine
	// that respects the provided context for cancellation.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the sync mechanism.
	Stop(ctx context.Context) error
	// Name returns the strategy name for logging.
	Name() string
}

// NewStrategy creates the appropriate sync strategy based on config.
func NewStrategy(cfg *config.SyncConfig, reloader ContentReloader, logger *slog.Logger) (Strategy, error) {
	if cfg == nil {
		return nil, fmt.Errorf("gitops: sync config must not be nil")
	}

	if reloader == nil {
		return nil, fmt.Errorf("gitops: content reloader must not be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	switch cfg.Strategy {
	case "watch":
		return NewWatchStrategy(reloader, logger), nil
	case "webhook":
		return NewWebhookStrategy(cfg.Webhook, reloader, logger)
	case "sidecar":
		return NewSidecarStrategy(reloader, logger), nil
	default:
		return nil, fmt.Errorf("gitops: unknown sync strategy %q (must be watch, webhook, or sidecar)", cfg.Strategy)
	}
}
