// Env override validation completeness per test-gap-analysis.md item #17
package config

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEnvOverrideInvalidPort(t *testing.T) {
	t.Setenv("BLOGFLOW_SERVER_PORT", "not-a-number")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Errorf("expected error for non-integer port")
	} else {
		t.Logf("got expected error: %v", err)
	}
}

func TestEnvOverrideInvalidMetricsPort(t *testing.T) {
	t.Setenv("BLOGFLOW_SERVER_METRICS_PORT", "invalid")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Error("expected error for invalid metrics port, got nil")
	} else {
		t.Logf("got expected error: %v", err)
	}
}

func TestEnvOverrideValidServerPort(t *testing.T) {
	t.Setenv("BLOGFLOW_SERVER_PORT", "9090")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error for valid port: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("server.port = %d, want 9090", cfg.Server.Port)
	}
}

func TestEnvOverrideWebhookAllowedIPsParsesAndOverridesYAML(t *testing.T) {
	yamlContent := `
sync:
  webhook:
    allowed_ips:
      - "192.0.2.10"
`
	tests := []struct {
		name     string
		envValue string
		want     []string
		wantWarn bool
	}{
		{
			name:     "empty string disables allowlist",
			envValue: "",
			want:     nil,
			wantWarn: true,
		},
		{
			name:     "single value",
			envValue: "203.0.113.10",
			want:     []string{"203.0.113.10"},
		},
		{
			name:     "trailing comma",
			envValue: "203.0.113.10,",
			want:     []string{"203.0.113.10"},
		},
		{
			name:     "leading and trailing whitespace",
			envValue: " 203.0.113.10 ",
			want:     []string{"203.0.113.10"},
		},
		{
			name:     "interior empty entry",
			envValue: "203.0.113.10, , 198.51.100.0/24",
			want:     []string{"203.0.113.10", "198.51.100.0/24"},
		},
		{
			name:     "duplicates preserved",
			envValue: "203.0.113.10,203.0.113.10",
			want:     []string{"203.0.113.10", "203.0.113.10"},
		},
		{
			name:     "whitespace and commas disable allowlist",
			envValue: " ,  , ",
			want:     []string{},
			wantWarn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BLOGFLOW_SYNC_WEBHOOK_ALLOWED_IPS", tt.envValue)
			var logOutput bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))
			fsys := fstest.MapFS{
				"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
			}
			loader := NewLoader(fsys, WithLogger(logger))
			cfg, err := loader.Load()
			if err != nil {
				t.Fatalf("unexpected error for valid allowed IP env override: %v", err)
			}

			if !reflect.DeepEqual(cfg.Sync.Webhook.AllowedIPs, tt.want) {
				t.Errorf("sync.webhook.allowed_ips = %#v, want %#v", cfg.Sync.Webhook.AllowedIPs, tt.want)
			}
			hasWarn := strings.Contains(logOutput.String(), "BLOGFLOW_SYNC_WEBHOOK_ALLOWED_IPS is set but resolved to no entries")
			if hasWarn != tt.wantWarn {
				t.Fatalf("warning presence = %t, want %t; logs:\n%s", hasWarn, tt.wantWarn, logOutput.String())
			}
		})
	}
}

func TestEnvOverrideInvalidBool(t *testing.T) {
	t.Setenv("BLOGFLOW_CACHE_ENABLED", "yes")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Error("expected error for non-boolean cache.enabled, got nil")
	} else {
		t.Logf("got expected error: %v", err)
	}
}
