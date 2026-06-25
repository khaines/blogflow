// Webhook secret logging redaction per test-gap-analysis.md item #16
// This test directly exercises WebhookConfig.LogValue() to verify
// that the secret is masked as [REDACTED] when the config is logged.
// It is mutation-verifiable: breaking LogValue() will cause this test to fail.
package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestWebhookConfig_LogValueRedaction(t *testing.T) {
	t.Parallel()

	secret := "very-long-webhook-secret-minimum-32-bytes-ok"

	wc := WebhookConfig{
		Path:   "/hook",
		Secret: secret,
	}

	// Exercise LogValue by logging WebhookConfig as an slog field.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	// Log the WebhookConfig as an attribute — this calls WebhookConfig.LogValue().
	logger.Info("webhook config", "webhook", slog.AnyValue(wc))

	logs := buf.String()

	// Mutation check: [REDACTED] MUST appear (proves LogValue was exercised).
	if !strings.Contains(logs, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in logs (LogValue() not called or broken); got:\n%s", logs)
	}

	// Mutation check: raw secret MUST NOT appear (proves LogValue() masked it).
	if strings.Contains(logs, secret) {
		t.Fatalf("raw secret leaked in logs (LogValue() did not mask Secret); got:\n%s", logs)
	}
}

// TestWebhookConfig_LogValue_empty verifies LogValue works with an empty secret (no panic, no crash).
func TestWebhookConfig_LogValueEmpty(t *testing.T) {
	t.Parallel()

	wc := WebhookConfig{
		Path:   "/hook",
		Secret: "",
	}

	// Call LogValue directly — should return a valid slog.Value without panicking.
	v := wc.LogValue()

	// The value should not contain any secret-like data.
	anyStr := v.String()
	if strings.Contains(anyStr, "very-long-webhook-secret") {
		t.Errorf("unexpected secret in empty config: %s", anyStr)
	}
}
