// Package gitops provides content synchronization strategies for BlogFlow.
// Supports webhook (push), sidecar (pull/K8s), and watch (local dev) modes.
package gitops

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kenhaines/blogflow/internal/config"
)

// ContentReloader is called when content changes are detected.
type ContentReloader func() error

// Strategy defines the interface for content sync mechanisms.
type Strategy interface {
	// Start begins watching/listening for content changes.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the sync mechanism.
	Stop() error
	// Name returns the strategy name for logging.
	Name() string
}

// NewStrategy creates the appropriate sync strategy based on config.
func NewStrategy(cfg *config.SyncConfig, reloader ContentReloader, logger *slog.Logger) (Strategy, error) {
	if logger == nil {
		logger = slog.Default()
	}

	switch cfg.Strategy {
	case "watch":
		return NewWatchStrategy(reloader, logger), nil
	case "webhook":
		return NewWebhookStrategy(cfg.Webhook, reloader, logger), nil
	case "sidecar":
		return NewSidecarStrategy(reloader, logger), nil
	default:
		return nil, fmt.Errorf("gitops: unknown sync strategy %q (must be watch, webhook, or sidecar)", cfg.Strategy)
	}
}
