package config

import (
	"errors"
	"fmt"
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

func TestLoad_SecretInYAML(t *testing.T) { //nolint:gosec // test data intentionally contains secret patterns
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
			if !isSecretError(err) {
				t.Errorf("expected SecretInYAMLError, got: %T: %v", err, err)
			}
		})
	}
}

func isSecretError(err error) bool {
	var secretErr *SecretInYAMLError
	return errors.As(err, &secretErr)
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

// --- H1: ConfigError.Error() includes field details ---

func TestConfigError_ErrorIncludesFields(t *testing.T) {
	cfg := Default()
	cfg.Server.Port = 0
	cfg.Sync.Strategy = "invalid"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "server.port") {
		t.Errorf("expected error to mention server.port, got: %s", msg)
	}
	if !strings.Contains(msg, "sync.strategy") {
		t.Errorf("expected error to mention sync.strategy, got: %s", msg)
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

// --- CRITICAL: env override parse error is propagated ---

func TestLoad_EnvOverrideError(t *testing.T) {
	fsys := fstest.MapFS{}
	t.Setenv("BLOGFLOW_SERVER_PORT", "notanumber")

	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for invalid BLOGFLOW_SERVER_PORT, got nil")
	}
	if !strings.Contains(err.Error(), "environment overrides") {
		t.Errorf("expected error about environment overrides, got: %v", err)
	}
}

// --- H2: zero timeouts rejected ---

func TestValidate_ZeroTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Config)
		field string
	}{
		{"zero read timeout", func(c *Config) { c.Server.ReadTimeout = 0 }, "server.read_timeout"},
		{"zero write timeout", func(c *Config) { c.Server.WriteTimeout = 0 }, "server.write_timeout"},
		{"zero idle timeout", func(c *Config) { c.Server.IdleTimeout = 0 }, "server.idle_timeout"},
		{"negative read timeout", func(c *Config) { c.Server.ReadTimeout = -1 * time.Second }, "server.read_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.setup(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatal("expected validation error")
			}
			cfgErr, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T", err)
			}
			found := false
			for _, fe := range cfgErr.Errors {
				if fe.Field == tt.field {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s field error", tt.field)
			}
		})
	}
}

// --- M1: field validation bounds ---

func TestValidate_FieldBounds(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Config)
		field string
	}{
		{"posts_per_page too low", func(c *Config) { c.Content.PostsPerPage = 0 }, "content.posts_per_page"},
		{"posts_per_page too high", func(c *Config) { c.Content.PostsPerPage = 101 }, "content.posts_per_page"},
		{"summary_length too low", func(c *Config) { c.Content.SummaryLength = 49 }, "content.summary_length"},
		{"summary_length too high", func(c *Config) { c.Content.SummaryLength = 1001 }, "content.summary_length"},
		{"cache max_entries negative", func(c *Config) { c.Cache.MaxEntries = -1 }, "cache.max_entries"},
		{"cache max_entries too high", func(c *Config) { c.Cache.MaxEntries = 100001 }, "cache.max_entries"},
		{"feed items too low", func(c *Config) { c.Feed.Items = 0 }, "feed.items"},
		{"feed items too high", func(c *Config) { c.Feed.Items = 101 }, "feed.items"},
		{
			"webhook path no leading slash",
			func(c *Config) {
				c.Sync.Strategy = "webhook"
				c.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
				c.Sync.Webhook.Path = "api/webhook"
			},
			"sync.webhook.path",
		},
		{
			"webhook rate_limit too low",
			func(c *Config) {
				c.Sync.Strategy = "webhook"
				c.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
				c.Sync.Webhook.RateLimit = 0
			},
			"sync.webhook.rate_limit",
		},
		{
			"webhook rate_limit too high",
			func(c *Config) {
				c.Sync.Strategy = "webhook"
				c.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
				c.Sync.Webhook.RateLimit = 101
			},
			"sync.webhook.rate_limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.setup(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("expected validation error for %s", tt.field)
			}
			cfgErr, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T", err)
			}
			found := false
			for _, fe := range cfgErr.Errors {
				if fe.Field == tt.field {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s field error, got: %v", tt.field, cfgErr.Errors)
			}
		})
	}
}

// --- H7: expanded secret patterns ---

func TestLoad_SecretPatterns_ConnectionStrings(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"postgres URI", "site:\n  base_url: \"postgres://user:pass@host/db\""},
		{"mysql URI", "site:\n  base_url: \"mysql://user:pass@host/db\""},
		{"redis URI", "site:\n  base_url: \"redis://user:pass@host/db\""},
		{"dsn URI", "site:\n  base_url: \"dsn://user:pass@host/db\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"site.yaml": &fstest.MapFile{Data: []byte(tt.content)},
			}
			loader := NewLoader(fsys)
			_, err := loader.Load()
			if err == nil {
				t.Fatal("expected error for connection string in YAML, got nil")
			}
			if !isSecretError(err) {
				t.Errorf("expected SecretInYAMLError, got: %T: %v", err, err)
			}
		})
	}
}

func TestLoad_SecretPatterns_SensitiveKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"password key", "password: mysecretpassword"},
		{"secret key", "secret: mysecretvalue"},
		{"token key", "token: mytoken123"},
		{"credential key", "credential: mycred"},
		{"apikey key", "apikey: myapikey"},
		{"api_key key", "api_key: myapikey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"site.yaml": &fstest.MapFile{Data: []byte(tt.content)},
			}
			loader := NewLoader(fsys)
			_, err := loader.Load()
			if err == nil {
				t.Fatal("expected error for sensitive key in YAML, got nil")
			}
			if !isSecretError(err) {
				t.Errorf("expected SecretInYAMLError, got: %T: %v", err, err)
			}
		})
	}
}

// --- M2: slog.LogValuer redacts secret ---

func TestConfig_LogValue(t *testing.T) {
	cfg := Default()
	cfg.Sync.Webhook.Secret = "super-secret-value"
	val := cfg.LogValue()
	s := val.String()
	if strings.Contains(s, "super-secret-value") {
		t.Error("LogValue() should redact webhook secret")
	}
}

// --- H-NEW-2: Cache.TTL bounds ---

func TestValidate_CacheTTL(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
	}{
		{"zero TTL", func(c *Config) { c.Cache.Enabled = true; c.Cache.TTL = 0 }, true},
		{"negative TTL", func(c *Config) { c.Cache.Enabled = true; c.Cache.TTL = -1 * time.Second }, true},
		{"over 24h", func(c *Config) { c.Cache.Enabled = true; c.Cache.TTL = 25 * time.Hour }, true},
		{"exactly 24h", func(c *Config) { c.Cache.Enabled = true; c.Cache.TTL = 24 * time.Hour }, false},
		{"valid 1h", func(c *Config) { c.Cache.Enabled = true; c.Cache.TTL = 1 * time.Hour }, false},
		{"disabled cache skips check", func(c *Config) { c.Cache.Enabled = false; c.Cache.TTL = 0 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.setup(cfg)
			err := Validate(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				cfgErr, ok := err.(*ConfigError)
				if !ok {
					t.Fatalf("expected *ConfigError, got %T", err)
				}
				found := false
				for _, fe := range cfgErr.Errors {
					if fe.Field == "cache.ttl" {
						found = true
					}
				}
				if !found {
					t.Errorf("expected cache.ttl field error, got: %v", cfgErr.Errors)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// --- M-NEW-1: AllowedEvents validated against known set ---

func TestValidate_AllowedEventsUnknown(t *testing.T) {
	cfg := Default()
	cfg.Sync.Strategy = "webhook"
	cfg.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
	cfg.Sync.Webhook.AllowedEvents = []string{"push", "unknown_event"}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for unknown event")
	}
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	found := false
	for _, fe := range cfgErr.Errors {
		if fe.Field == "sync.webhook.allowed_events" && fe.Value == "unknown_event" {
			found = true
		}
	}
	if !found {
		t.Error("expected sync.webhook.allowed_events field error for unknown_event")
	}
}

func TestValidate_AllowedEventsAllValid(t *testing.T) {
	cfg := Default()
	cfg.Sync.Strategy = "webhook"
	cfg.Sync.Webhook.Secret = "this-is-a-very-long-secret-that-is-at-least-32-bytes"
	cfg.Sync.Webhook.AllowedEvents = []string{"push", "ping", "pull_request", "release", "workflow_dispatch"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected no error for all valid events, got: %v", err)
	}
}

// --- M-NEW-2: BaseURL validated ---

func TestValidate_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com", false},
		{"valid with port", "http://localhost:8080", false},
		{"missing scheme", "example.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"empty host", "http://", true},
		{"empty string", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Site.BaseURL = tt.baseURL
			err := Validate(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				cfgErr, ok := err.(*ConfigError)
				if !ok {
					t.Fatalf("expected *ConfigError, got %T", err)
				}
				found := false
				for _, fe := range cfgErr.Errors {
					if fe.Field == "site.base_url" {
						found = true
					}
				}
				if !found {
					t.Errorf("expected site.base_url field error, got: %v", cfgErr.Errors)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// --- M-NEW-3: Feed validation conditional on Enabled ---

func TestValidate_FeedDisabledSkipsValidation(t *testing.T) {
	cfg := Default()
	cfg.Feed.Enabled = false
	cfg.Feed.Type = "invalid"
	cfg.Feed.Items = 0
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected no error when feed is disabled, got: %v", err)
	}
}

// --- M-NEW-4: DateFormat non-empty ---

func TestValidate_DateFormatEmpty(t *testing.T) {
	cfg := Default()
	cfg.Content.DateFormat = ""
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty date_format")
	}
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	found := false
	for _, fe := range cfgErr.Errors {
		if fe.Field == "content.date_format" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected content.date_format field error, got: %v", cfgErr.Errors)
	}
}

// --- L-NEW-2: isSecretError uses errors.As ---

func TestIsSecretError_WrappedError(t *testing.T) {
	base := &SecretInYAMLError{Field: "test", Pattern: "test"}
	wrapped := fmt.Errorf("wrapped: %w", base)
	if !isSecretError(wrapped) {
		t.Error("isSecretError should find SecretInYAMLError via errors.As on wrapped errors")
	}
}
