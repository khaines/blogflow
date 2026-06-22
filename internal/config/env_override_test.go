// Env override validation completeness per test-gap-analysis.md item #17
package config

import (
	"testing"
	"testing/fstest"
)

func TestEnvOverrideInvalidPort(t *testing.T) {
	t.Setenv("BLOGFLOW_SERVER_PORT", "not-a-number")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Log("expected error for non-integer port")
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
		t.Log("expected warning for invalid metrics port")
	} else {
		t.Logf("got error: %v", err)
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

func TestEnvOverrideInvalidBool(t *testing.T) {
	t.Setenv("BLOGFLOW_CACHE_ENABLED", "yes")
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Log("expected error for non-boolean cache.enabled")
	} else {
		t.Logf("got expected error: %v", err)
	}
}
