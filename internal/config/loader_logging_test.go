package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoad_LogsConfigFileLoaded(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{
			Data: []byte("site:\n  title: \"Log Test\"\n"),
		},
	}
	loader := NewLoader(fsys, WithLogger(logger))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "config file loaded") {
		t.Errorf("expected 'config file loaded' log, got:\n%s", out)
	}
	if !strings.Contains(out, "path=site.yaml") {
		t.Errorf("expected path=site.yaml in log, got:\n%s", out)
	}
	if !strings.Contains(out, "duration=") {
		t.Errorf("expected duration in log, got:\n%s", out)
	}
}

func TestLoad_LogsDefaultsWhenNoConfig(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fsys := fstest.MapFS{}
	loader := NewLoader(fsys, WithLogger(logger))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "no config file found") {
		t.Errorf("expected 'no config file found' log, got:\n%s", out)
	}
}

func TestLoad_LogsEnvOverrides(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	t.Setenv("BLOGFLOW_SITE_TITLE", "Override Title")
	t.Setenv("BLOGFLOW_SERVER_PORT", "9090")

	fsys := fstest.MapFS{}
	loader := NewLoader(fsys, WithLogger(logger))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "env var overrides applied") {
		t.Errorf("expected 'env var overrides applied' log, got:\n%s", out)
	}
	if !strings.Contains(out, "count=2") {
		t.Errorf("expected count=2 in log, got:\n%s", out)
	}
	if !strings.Contains(out, "BLOGFLOW_SITE_TITLE") {
		t.Errorf("expected BLOGFLOW_SITE_TITLE in log, got:\n%s", out)
	}
	if !strings.Contains(out, "BLOGFLOW_SERVER_PORT") {
		t.Errorf("expected BLOGFLOW_SERVER_PORT in log, got:\n%s", out)
	}
}

func TestLoad_LogsEnvOverridesRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	t.Setenv("BLOGFLOW_WEBHOOK_SECRET", strings.Repeat("s", 32))
	t.Setenv("BLOGFLOW_SYNC_STRATEGY", "webhook")

	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{
			Data: []byte("sync:\n  strategy: webhook\n  webhook:\n    allowed_events: [push]\n    rate_limit: 10\n"),
		},
	}
	loader := NewLoader(fsys, WithLogger(logger))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "BLOGFLOW_WEBHOOK_SECRET=[REDACTED]") {
		t.Errorf("expected BLOGFLOW_WEBHOOK_SECRET=[REDACTED] in log, got:\n%s", out)
	}
	// The secret value itself must NOT appear
	if strings.Contains(out, strings.Repeat("s", 32)) {
		t.Errorf("secret value leaked in log output")
	}
}

func TestLoad_LogsValidationPassed(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fsys := fstest.MapFS{}
	loader := NewLoader(fsys, WithLogger(logger))
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "config validation passed") {
		t.Errorf("expected 'config validation passed' log, got:\n%s", out)
	}
}

func TestLoad_LogsValidationFailed(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{
			Data: []byte("server:\n  port: 0\n"),
		},
	}
	loader := NewLoader(fsys, WithLogger(logger))
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	out := buf.String()
	if !strings.Contains(out, "config validation failed") {
		t.Errorf("expected 'config validation failed' log, got:\n%s", out)
	}
}

func TestLoad_NilLoggerDoesNotPanic(t *testing.T) {
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
}
