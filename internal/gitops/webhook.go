package gitops

import (
	"context"
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
func NewWebhookStrategy(cfg config.WebhookConfig, reloader ContentReloader, logger *slog.Logger) *WebhookStrategy {
	return &WebhookStrategy{config: cfg, reloader: reloader, logger: logger}
}

// Start begins listening for webhook callbacks.
// TODO: wire to server webhook route
func (w *WebhookStrategy) Start(ctx context.Context) error {
	w.logger.Info("webhook strategy started", "path", w.config.Path)
	return nil
}

// Stop gracefully shuts down the webhook listener.
func (w *WebhookStrategy) Stop() error {
	w.logger.Info("webhook strategy stopped")
	return nil
}

// Name returns the strategy name.
func (w *WebhookStrategy) Name() string { return "webhook" }
