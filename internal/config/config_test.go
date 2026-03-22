package config

import (
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// --- Defaults ---

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.IdleTimeout != 120*time.Second {
		t.Errorf("expected default idle timeout 120s, got %v", cfg.Server.IdleTimeout)
	}
	if cfg.Site.Title != "My Blog" {
		t.Errorf("expected default title 'My Blog', got %q", cfg.Site.Title)
	}
	if cfg.Feed.Type != "atom" {
		t.Errorf("expected default feed type 'atom', got %q", cfg.Feed.Type)
	}
	if cfg.Sync.Strategy != "watch" {
		t.Errorf("expected default sync strategy 'watch', got %q", cfg.Sync.Strategy)
	}
	if cfg.Cache.Enabled != true {
		t.Error("expected cache enabled by default")
	}
	if cfg.Content.PostsPerPage != 10 {
		t.Errorf("expected default posts_per_page 10, got %d", cfg.Content.PostsPerPage)
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Default() config should be valid, got: %v", err)
	}
}

// --- Load: defaults only ---

func TestLoad_DefaultsOnly(t *testing.T) {
	// Empty FS — no site.yaml, falls back to defaults.
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() with no site.yaml failed: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Site.Title != "My Blog" {
		t.Errorf("expected title 'My Blog', got %q", cfg.Site.Title)
	}
}

// --- Load: YAML override ---

func TestLoad_YAMLOverride(t *testing.T) {
	yamlContent := `
server:
  port: 9090
site:
  title: "Custom Blog"
`
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
	}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090 from YAML, got %d", cfg.Server.Port)
	}
	if cfg.Site.Title != "Custom Blog" {
		t.Errorf("expected title 'Custom Blog', got %q", cfg.Site.Title)
	}
	// Fields not in YAML should retain defaults.
	if cfg.Feed.Type != "atom" {
		t.Errorf("expected feed type 'atom' (default), got %q", cfg.Feed.Type)
	}
}

// --- Load: env override ---

func TestLoad_EnvOverride(t *testing.T) {
	fsys := fstest.MapFS{}
	t.Setenv("BLOGFLOW_SITE_TITLE", "Env Title")
	t.Setenv("BLOGFLOW_SERVER_PORT", "3000")

	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Site.Title != "Env Title" {
		t.Errorf("expected title 'Env Title', got %q", cfg.Site.Title)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
}

// --- Load: env precedence > yaml > defaults ---

func TestLoad_EnvPrecedence(t *testing.T) {
	yamlContent := `
server:
  port: 9090
site:
  title: "YAML Title"
`
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
	}
	t.Setenv("BLOGFLOW_SERVER_PORT", "4000")

	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	// Env overrides YAML.
	if cfg.Server.Port != 4000 {
		t.Errorf("expected port 4000 (env), got %d", cfg.Server.Port)
	}
	// YAML overrides defaults.
	if cfg.Site.Title != "YAML Title" {
		t.Errorf("expected title 'YAML Title' (yaml), got %q", cfg.Site.Title)
	}
	// Defaults apply when neither env nor YAML set a field.
	if cfg.Feed.Type != "atom" {
		t.Errorf("expected feed type 'atom' (default), got %q", cfg.Feed.Type)
	}
}

// --- Load: strict unmarshal rejects unknown fields ---

func TestLoad_StrictUnmarshal(t *testing.T) {
	yamlContent := `
servr:
  port: 8080
`
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
	}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for unknown YAML field, got nil")
	}
	if !strings.Contains(err.Error(), "servr") {
		t.Errorf("expected error to mention 'servr', got: %v", err)
	}
}

// --- Load: secret in YAML rejected ---

func TestLoad_SecretInYAML(t *testing.T) {
	tests := []struct {
		name    string
		content string
		pattern string
	}{
		{
			name:    "GitHub token",
			content: "site:\n  title: \"ghp_AAAAAAAABBBBBBBBCCCCCCCCDDDDDDDDEEEE\"",
			pattern: "ghp_",
		},
		{
			name:    "Private key",
			content: "site:\n  title: \"-----BEGIN RSA PRIVATE KEY-----\"",
			pattern: "private key",
		},
		{
			name:    "Env var placeholder",
			content: "sync:\n  webhook:\n    path: \"${BLOGFLOW_WEBHOOK_SECRET}\"",
			pattern: "env var placeholder",
		},
		{
			name:    "AWS key",
			content: "site:\n  title: \"AKIAIOSFODNN7EXAMPLE\"",
			pattern: "AWS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"site.yaml": &fstest.MapFile{Data: []byte(tt.content)},
			}
			loader := NewLoader(fsys)
			_, err := loader.Load()
			if err == nil {
				t.Fatal("expected error for secret in YAML, got nil")
			}
			var secretErr *SecretInYAMLError
			if !isSecretError(err) {
				t.Errorf("expected SecretInYAMLError, got: %T: %v", err, err)
			}
			_ = secretErr
		})
	}
}

func isSecretError(err error) bool {
	_, ok := err.(*SecretInYAMLError)
	return ok
}

// --- Load: YAML anchors/aliases rejected ---

func TestLoad_AnchorAlias(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "bare anchor",
			content: "defaults: &defaults\n  port: 8080\nserver:\n  <<: *defaults",
			wantErr: true,
		},
		{
			name:    "bare alias",
			content: "server:\n  port: *ref",
			wantErr: true,
		},
		{
			name:    "quoted glob not rejected",
			content: "sync:\n  webhook:\n    branch_filter: \"feature/*\"",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"site.yaml": &fstest.MapFile{Data: []byte(tt.content)},
			}
			loader := NewLoader(fsys)
			_, err := loader.Load()
			if tt.wantErr && err == nil {
				t.Fatal("expected error for anchor/alias, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "anchor") && !strings.Contains(err.Error(), "alias") {
				t.Errorf("expected error to mention anchor/alias, got: %v", err)
			}
		})
	}
}

// --- Load: file size limit ---

func TestLoad_FileSizeLimit(t *testing.T) {
	bigData := make([]byte, 2*1024*1024) // 2 MB
	for i := range bigData {
		bigData[i] = ' '
	}
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: bigData},
	}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for oversized config, got nil")
	}
	if !strings.Contains(err.Error(), "1 MB") {
		t.Errorf("expected error to mention '1 MB', got: %v", err)
	}
}

// --- Validate: invalid port ---

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 99999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Server.Port = tt.port
			err := Validate(cfg)
			if err == nil {
				t.Fatal("expected validation error for invalid port")
			}
			cfgErr, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T", err)
			}
			found := false
			for _, fe := range cfgErr.Errors {
				if fe.Field == "server.port" {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected server.port field error")
			}
		})
	}
}

// --- Validate: path traversal ---

func TestValidate_PathTraversal(t *testing.T) {
	cfg := Default()
	cfg.Content.PostsDir = "../evil"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for path traversal")
	}
	if !strings.Contains(err.Error(), "config validation failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Validate: invalid strategy ---

func TestValidate_InvalidStrategy(t *testing.T) {
	cfg := Default()
	cfg.Sync.Strategy = "poll"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid strategy")
	}
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	found := false
	for _, fe := range cfgErr.Errors {
		if fe.Field == "sync.strategy" {
			found = true
			if !strings.Contains(fe.Message, "watch") {
				t.Errorf("expected message to list valid strategies, got: %q", fe.Message)
			}
		}
	}
	if !found {
		t.Error("expected sync.strategy field error")
	}
}

// --- Validate: webhook secret too short ---

func TestValidate_WebhookSecretTooShort(t *testing.T) {
	cfg := Default()
	cfg.Sync.Strategy = "webhook"
	cfg.Sync.Webhook.Secret = "short"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for short webhook secret")
	}
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	found := false
	for _, fe := range cfgErr.Errors {
		if fe.Field == "sync.webhook.secret" {
			found = true
			if fe.Value != "[REDACTED]" {
				t.Errorf("expected Value to be [REDACTED], got: %v", fe.Value)
			}
		}
	}
	if !found {
		t.Error("expected sync.webhook.secret field error")
	}
}

// --- Get: never nil ---

func TestGet_NeverNil(t *testing.T) {
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	// Get() before Load() should return non-nil (defaults).
	cfg := loader.Get()
	if cfg == nil {
		t.Fatal("Get() returned nil before Load()")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port, got %d", cfg.Server.Port)
	}
}

// --- Get: concurrent safety ---

func TestGet_Concurrent(t *testing.T) {
	fsys := fstest.MapFS{}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := loader.Get()
			if cfg == nil {
				t.Error("Get() returned nil during concurrent access")
			}
		}()
	}
	wg.Wait()
}

// --- Validate: absolute content paths rejected ---

func TestValidate_AbsolutePath(t *testing.T) {
	cfg := Default()
	cfg.Content.PostsDir = "/etc/evil"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for absolute path")
	}
}

// --- Validate: webhook strategy with valid long secret passes ---

func TestValidate_WebhookValidSecret(t *testing.T) {
	cfg := Default()
	cfg.Sync.Strategy = "webhook"
	cfg.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected no error for valid webhook config, got: %v", err)
	}
}

// --- Load: bool env var override ---

func TestLoad_BoolEnvOverride(t *testing.T) {
	fsys := fstest.MapFS{}
	t.Setenv("BLOGFLOW_CACHE_ENABLED", "false")

	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Cache.Enabled != false {
		t.Error("expected cache disabled via env var")
	}
}

// --- Load: duration parsing from YAML ---

func TestLoad_DurationParsing(t *testing.T) {
	yamlContent := `
server:
  port: 8080
  read_timeout: "15s"
  write_timeout: "30s"
  idle_timeout: "60s"
`
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
	}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected read timeout 15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("expected write timeout 30s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Errorf("expected idle timeout 60s, got %v", cfg.Server.IdleTimeout)
	}
}

// --- Validate: invalid feed type ---

func TestValidate_InvalidFeedType(t *testing.T) {
	cfg := Default()
	cfg.Feed.Type = "json"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid feed type")
	}
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	found := false
	for _, fe := range cfgErr.Errors {
		if fe.Field == "feed.type" {
			found = true
		}
	}
	if !found {
		t.Error("expected feed.type field error")
	}
}

// --- Load: empty config file uses defaults ---

func TestLoad_EmptyConfigFile(t *testing.T) {
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte("")},
	}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() with empty file failed: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

// --- Load: partial site.yaml retains defaults for missing fields ---

func TestLoad_PartialYAML(t *testing.T) {
	yamlContent := `
site:
  title: "Partial Blog"
`
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: []byte(yamlContent)},
	}
	loader := NewLoader(fsys)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Site.Title != "Partial Blog" {
		t.Errorf("expected title 'Partial Blog', got %q", cfg.Site.Title)
	}
	// Other fields should be defaults.
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Cache.TTL != 1*time.Hour {
		t.Errorf("expected default cache TTL 1h, got %v", cfg.Cache.TTL)
	}
}
