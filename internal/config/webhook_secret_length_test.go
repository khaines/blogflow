package config

import (
	"strings"
	"testing"
	
)

// TestValidate_WebhookSecretTooShortEnforcement addresses test-gap-analysis.md Critical #4:
// "Webhook secret must be ≥32 bytes enforced at startup (HMAC-SHA256 key length requirement)."
func TestValidate_WebhookSecretMinLength(t *testing.T) { 
	tests := []struct {
		name     string
		secret   string
		wantErr  bool
	}{
		{"1-byte rejected", "x", true}, 
		{"31-byte rejected", strings.Repeat("y", 31), true},
		{"exactly 32 accepted", strings.Repeat("z", 31)+"a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Sync.Strategy = "webhook"
			cfg.Sync.Webhook.Secret = tt.secret
			err := Validate(cfg)

			if (err == nil) != tt.wantErr { 
				t.Errorf("Validate %q: got err=%v, wantErr=%v", tt.name, err, tt.wantErr)  
			}

			if tt.wantErr {  
				if err.Error() != "" && !strings.Contains(err.Error(), "[REDACTED]") {
					t.Logf("error for short secret: %q", err.Error()) 
				}
			}
		})
	}
	
	t.Log("Webhook secret minimum length validation completed")
}
