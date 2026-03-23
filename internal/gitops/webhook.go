package gitops

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kenhaines/blogflow/internal/config"
)

// WebhookStrategy listens for HTTP webhook callbacks to trigger content reload.
type WebhookStrategy struct {
	config   config.WebhookConfig
	reloader ContentReloader
	logger   *slog.Logger
}

// NewWebhookStrategy creates a new webhook-based sync strategy.
// Path must be non-empty and start with "/".
func NewWebhookStrategy(cfg config.WebhookConfig, reloader ContentReloader, logger *slog.Logger) (*WebhookStrategy, error) {
	if cfg.Path == "" || cfg.Path[0] != '/' {
		return nil, fmt.Errorf("gitops: webhook path must be non-empty and start with '/' (got %q)", cfg.Path)
	}

	return &WebhookStrategy{config: cfg, reloader: reloader, logger: logger}, nil
}

// Start begins listening for webhook callbacks.
// It returns promptly; background work runs in a separate goroutine.
// TODO: wire to server webhook route
func (w *WebhookStrategy) Start(ctx context.Context) error {
	w.logger.Warn("webhook strategy started (stub)", "path", w.config.Path)
	return nil
}

// Stop gracefully shuts down the webhook listener.
func (w *WebhookStrategy) Stop(ctx context.Context) error {
	w.logger.Info("webhook strategy stopped")
	return nil
}

// Name returns the strategy name.
func (w *WebhookStrategy) Name() string { return "webhook" }
