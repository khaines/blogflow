package config

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestWebhookSecretLengthBoundary(t *testing.T) {
	t.Parallel()

	// Start with a fully valid default config for the base.
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)

	// Build on top of Default() to ensure all other fields are valid.
	validConfig := Default()

	// Test 31-byte secret: should fail (requirement is >= 32 bytes).
	config31 := validConfig
	config31.Sync.Strategy = "webhook"
	config31.Sync.Webhook.Secret = strings.Repeat("x", 31) // exactly 31 bytes

	// First load to get a working config through the loader (no YAML changes needed)
	_, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}

	err31 := Validate(config31)
	if err31 == nil {
		t.Error("expected validation error for 31-byte webhook secret, got nil")
	} else {
		cfgErr, ok := err31.(*ConfigError)
		if !ok {
			t.Errorf("expected *ConfigError, got %T", err31)
		} else {
			found := false
			for _, fe := range cfgErr.Errors {
				if fe.Field == "sync.webhook.secret" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected sync.webhook.secret field error, got errors: %v", cfgErr.Errors)
			}
		}
	}

	// Test 32-byte secret: should pass.
	config32 := Default()
	config32.Sync.Strategy = "webhook"
	config32.Sync.Webhook.Secret = strings.Repeat("y", 32) // exactly 32 bytes

	err32 := Validate(config32)
	if err32 != nil {
		t.Errorf("expected no validation error for 32-byte webhook secret, got: %v", err32)
	}

	// Test 33-byte secret: should also pass (just over the boundary).
	config33 := Default()
	config33.Sync.Strategy = "webhook"
	config33.Sync.Webhook.Secret = strings.Repeat("z", 33) // 33 bytes

	err33 := Validate(config33)
	if err33 != nil {
		t.Errorf("expected no validation error for 33-byte webhook secret, got: %v", err33)
	}

	// Verify Default() server timeouts are valid (regression guard).
	d := Default()
	if d.Server.ReadTimeout != 5*time.Second {
		t.Errorf("Default().Server.ReadTimeout = %v, want 5s", d.Server.ReadTimeout)
	}
	if d.Server.WriteTimeout != 10*time.Second {
		t.Errorf("Default().Server.WriteTimeout = %v, want 10s", d.Server.WriteTimeout)
	}
	if d.Server.IdleTimeout != 120*time.Second {
		t.Errorf("Default().Server.IdleTimeout = %v, want 120s", d.Server.IdleTimeout)
	}
}
