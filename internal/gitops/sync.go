// Package gitops provides content synchronization strategies for BlogFlow.
// Supports webhook (push), sidecar (pull/K8s), and watch (local dev) modes.
package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
// An IPResolver is required when the strategy is "webhook" (mandatory for
// preventing blind X-Forwarded-For trust); it may be omitted for other
// strategies (watch, sidecar, poll) that do not resolve client IPs.
func NewStrategy(cfg *config.SyncConfig, reloader ContentReloader, logger *slog.Logger, resolver ...IPResolver) (Strategy, error) {
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
		if len(resolver) == 0 || resolver[0] == nil {
			return nil, fmt.Errorf("gitops: webhook strategy requires an IP resolver (no trusted-proxy boundary)")
		}
		return NewWebhookStrategy(cfg.Webhook, reloader, logger, resolver[0])
	case "sidecar":
		return NewSidecarStrategy(reloader, logger), nil
	case "poll":
		return newPollFromConfig(cfg, reloader, logger)
	default:
		return nil, fmt.Errorf("gitops: unknown sync strategy %q (must be watch, webhook, sidecar, or poll)", cfg.Strategy)
	}
}

// newPollFromConfig parses PollInterval and constructs a PollStrategy.
// The puller must be wired post-construction via SetPuller before Start.
func newPollFromConfig(cfg *config.SyncConfig, reloader ContentReloader, logger *slog.Logger) (*PollStrategy, error) {
	interval, err := time.ParseDuration(cfg.PollInterval)
	if err != nil {
		return nil, fmt.Errorf("gitops: invalid poll_interval %q: %w", cfg.PollInterval, err)
	}
	return NewPollStrategy(interval, reloader, logger)
}
