package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khaines/blogflow/internal/envfile"
	"gopkg.in/yaml.v3"
)

const maxConfigFileSize = 1 << 20 // 1 MB

// ConfigError aggregates one or more validation failures.
type ConfigError struct { //nolint:revive // config.ConfigError is conventional for domain errors
	Errors []FieldError
}

func (e *ConfigError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fe.Error()
	}
	return fmt.Sprintf("config validation failed (%d errors): %s", len(e.Errors), strings.Join(msgs, "; "))
}

// FieldError describes a single validation failure.
type FieldError struct {
	Field   string
	Value   any
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// SecretInYAMLError is returned when a YAML file contains a value
// matching known secret patterns.
type SecretInYAMLError struct {
	Field   string
	Pattern string
}

func (e *SecretInYAMLError) Error() string {
	return fmt.Sprintf("secret pattern detected in config file: %s (pattern: %s) — use environment variables instead", e.Field, e.Pattern)
}

// Loader loads, validates, and manages configuration.
type Loader struct {
	configFS  fs.FS
	logger    *slog.Logger
	current   atomic.Pointer[Config]
	configDir string       // OS path for fsnotify watching
	reloadMu  sync.Mutex   // serializes Reload to prevent stale-callback races
	mu        sync.RWMutex // protects callbacks
	callbacks []func(*Config)
}

// LoaderOption configures optional Loader behaviour.
type LoaderOption func(*Loader)

// WithWatchDir sets the OS-level directory that Watch() monitors for
// site.yaml changes. Required only if Watch() will be called.
func WithWatchDir(dir string) LoaderOption {
	return func(l *Loader) { l.configDir = dir }
}

// WithLogger sets the structured logger for config operations.
// If not set, a no-op logger is used.
func WithLogger(logger *slog.Logger) LoaderOption {
	return func(l *Loader) { l.logger = logger }
}

// NewLoader creates a config loader backed by the given filesystem.
// The FS should be a 2-layer overlay (config + defaults) or a single
// defaults FS. NewLoader eagerly loads defaults so Get() never returns nil.
func NewLoader(configFS fs.FS, opts ...LoaderOption) *Loader {
	l := &Loader{configFS: configFS, logger: slog.New(discardHandler{})}
	for _, opt := range opts {
		opt(l)
	}
	// Eagerly store defaults so Get() is never nil.
	l.current.Store(Default())
	return l
}

// discardHandler is a slog.Handler that discards all log records.
type discardHandler struct{}

func (discardHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (discardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (discardHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return discardHandler{} }
func (discardHandler) WithGroup(_ string) slog.Handler               { return discardHandler{} }

// Get returns the current immutable Config. Safe for concurrent use.
// Never returns nil — defaults are loaded eagerly in NewLoader.
func (l *Loader) Get() *Config {
	return l.current.Load()
}

// Config returns the current immutable Config atomically. Alias for Get().
func (l *Loader) Config() *Config {
	return l.current.Load()
}

// Load reads site.yaml through the provided FS, applies env var
// overrides, validates the result, and stores it atomically.
func (l *Loader) Load() (*Config, error) {
	start := time.Now()
	cfg := Default()

	data, err := fs.ReadFile(l.configFS, "site.yaml")
	if err != nil && !isNotExist(err) {
		return nil, fmt.Errorf("reading site.yaml: %w", err)
	}

	if err == nil {
		if len(data) > maxConfigFileSize {
			return nil, fmt.Errorf("config file exceeds 1 MB limit (size: %d bytes)", len(data))
		}

		if secretErr := scanForSecrets(data); secretErr != nil {
			return nil, secretErr
		}

		if aliasErr := scanForAnchorsAliases(data); aliasErr != nil {
			return nil, aliasErr
		}

		if len(bytes.TrimSpace(data)) > 0 {
			dec := yaml.NewDecoder(bytes.NewReader(data))
			dec.KnownFields(true)
			if err := dec.Decode(cfg); err != nil {
				return nil, fmt.Errorf("parsing site.yaml: %w", err)
			}
		}

		l.logger.Info("config file loaded",
			"path", "site.yaml",
			"size_bytes", len(data),
			"duration", time.Since(start),
		)
	} else {
		l.logger.Debug("no config file found, using defaults")
	}

	applied, envErr := applyEnvOverrides(cfg, l.logger)
	if envErr != nil {
		return nil, fmt.Errorf("applying environment overrides: %w", envErr)
	}
	if len(applied) > 0 {
		sort.Strings(applied)
		redacted := make([]string, len(applied))
		for i, name := range applied {
			if secretEnvVars[name] {
				redacted[i] = name + "=[REDACTED]"
			} else {
				redacted[i] = name
			}
		}
		l.logger.Info("env var overrides applied",
			"count", len(applied),
			"vars", redacted,
		)
	}

	if err := Validate(cfg); err != nil {
		l.logger.Warn("config validation failed", "error", err)
		return nil, err
	}

	l.logger.Info("config validation passed",
		"duration", time.Since(start),
	)

	l.current.Store(cfg)
	return cfg, nil
}

// secretPatterns are scanned against raw YAML bytes before parsing.
var secretPatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), "GitHub personal access token (ghp_)"},
	{regexp.MustCompile(`gho_[A-Za-z0-9]`), "GitHub OAuth token (gho_)"},
	{regexp.MustCompile(`ghu_[A-Za-z0-9]`), "GitHub user-to-server token (ghu_)"},
	{regexp.MustCompile(`ghs_[A-Za-z0-9]`), "GitHub server-to-server token (ghs_)"},
	{regexp.MustCompile(`ghr_[A-Za-z0-9]`), "GitHub refresh token (ghr_)"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9]`), "GitHub fine-grained PAT (github_pat_)"},
	{regexp.MustCompile(`glpat-[A-Za-z0-9]`), "GitLab token (glpat-)"},
	{regexp.MustCompile(`xoxb-[A-Za-z0-9]`), "Slack bot token (xoxb-)"},
	{regexp.MustCompile(`xoxp-[A-Za-z0-9]`), "Slack user token (xoxp-)"},
	{regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`), "API key (sk-)"},
	{regexp.MustCompile(`-----BEGIN.*PRIVATE KEY-----`), "private key"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS access key"},
	{regexp.MustCompile(`\$\{BLOGFLOW_\w+\}`), "env var placeholder in YAML (use env vars directly)"},
	{regexp.MustCompile(`dsn://`), "DSN connection string (dsn://)"},
	{regexp.MustCompile(`postgres://`), "PostgreSQL connection string (postgres://)"},
	{regexp.MustCompile(`mysql://`), "MySQL connection string (mysql://)"},
	{regexp.MustCompile(`redis://`), "Redis connection string (redis://)"},
	{regexp.MustCompile(`(?im)^\s*\w*(password|secret|token|credential|apikey|api_key)\w*\s*:\s*\S`), "sensitive YAML key with inline value"},
}

func scanForSecrets(data []byte) error {
	for _, sp := range secretPatterns {
		if sp.pattern.Match(data) {
			return &SecretInYAMLError{
				Field:   "yaml content",
				Pattern: sp.desc,
			}
		}
	}
	return nil
}

// scanForAnchorsAliases rejects YAML containing bare anchor (&) or
// alias (*) tokens to prevent billion-laughs attacks. Characters
// inside quoted strings are not flagged.
func scanForAnchorsAliases(data []byte) error {
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		stripped := bytes.TrimSpace(line)
		if len(stripped) == 0 || stripped[0] == '#' {
			continue
		}

		// Walk through the line tracking quote state
		inSingle := false
		inDouble := false
		for i := 0; i < len(stripped); i++ {
			ch := stripped[i]
			switch {
			case ch == '\'' && !inDouble:
				inSingle = !inSingle
			case ch == '"' && !inSingle:
				inDouble = !inDouble
			case (ch == '&' || ch == '*') && !inSingle && !inDouble:
				// Check this is a YAML anchor/alias: preceded by
				// whitespace (or start-of-line) and followed by a
				// word character.
				prevOK := i == 0 || stripped[i-1] == ' ' || stripped[i-1] == '\t' || stripped[i-1] == ':'
				nextOK := i+1 < len(stripped) && isWordChar(stripped[i+1])
				if prevOK && nextOK {
					kind := "anchor"
					if ch == '*' {
						kind = "alias"
					}
					return fmt.Errorf("YAML %s detected in config file — anchors and aliases are not allowed", kind)
				}
			}
		}
	}
	return nil
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// envMap maps BLOGFLOW_* env var names to setter functions.
var envMap = map[string]func(*Config, string) error{
	"BLOGFLOW_SITE_TITLE": func(c *Config, v string) error {
		c.Site.Title = v
		return nil
	},
	"BLOGFLOW_SITE_DESCRIPTION": func(c *Config, v string) error {
		c.Site.Description = v
		return nil
	},
	"BLOGFLOW_SITE_BASE_URL": func(c *Config, v string) error {
		c.Site.BaseURL = v
		return nil
	},
	"BLOGFLOW_SERVER_PORT": func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_PORT as int: %w", err)
		}
		c.Server.Port = n
		return nil
	},
	"BLOGFLOW_SERVER_READ_TIMEOUT": func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_READ_TIMEOUT as duration: %w", err)
		}
		c.Server.ReadTimeout = d
		return nil
	},
	"BLOGFLOW_SERVER_WRITE_TIMEOUT": func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_WRITE_TIMEOUT as duration: %w", err)
		}
		c.Server.WriteTimeout = d
		return nil
	},
	"BLOGFLOW_SERVER_IDLE_TIMEOUT": func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_IDLE_TIMEOUT as duration: %w", err)
		}
		c.Server.IdleTimeout = d
		return nil
	},
	"BLOGFLOW_CACHE_ENABLED": func(c *Config, v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_CACHE_ENABLED as bool: %w", err)
		}
		c.Cache.Enabled = b
		return nil
	},
	"BLOGFLOW_SYNC_STRATEGY": func(c *Config, v string) error {
		c.Sync.Strategy = v
		return nil
	},
	"BLOGFLOW_SYNC_REPO": func(c *Config, v string) error {
		c.Sync.Repo = v
		return nil
	},
	"BLOGFLOW_SYNC_BRANCH": func(c *Config, v string) error {
		c.Sync.Branch = v
		return nil
	},
	"BLOGFLOW_WEBHOOK_SECRET": func(c *Config, v string) error {
		c.Sync.Webhook.Secret = v
		return nil
	},
	"BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT": func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT as int: %w", err)
		}
		c.Sync.Webhook.RateLimit = n
		return nil
	},
	"BLOGFLOW_SYNC_POLL_INTERVAL": func(c *Config, v string) error {
		c.Sync.PollInterval = v
		return nil
	},
	"BLOGFLOW_SYNC_SPARSE_DIRS": func(c *Config, v string) error {
		if v == "" {
			c.Sync.SparseDirs = nil
			return nil
		}
		parts := strings.Split(v, ",")
		dirs := make([]string, 0, len(parts))
		for _, p := range parts {
			if d := strings.TrimSpace(p); d != "" {
				dirs = append(dirs, d)
			}
		}
		c.Sync.SparseDirs = dirs
		return nil
	},
	"BLOGFLOW_SYNC_CLONE_DEPTH": func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SYNC_CLONE_DEPTH as int: %w", err)
		}
		c.Sync.CloneDepth = n
		return nil
	},
	"BLOGFLOW_FEED_TYPE": func(c *Config, v string) error {
		c.Feed.Type = v
		return nil
	},
	"BLOGFLOW_SERVER_TLS_TERMINATED": func(c *Config, v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_TLS_TERMINATED as bool: %w", err)
		}
		c.Server.TLSTerminated = b
		return nil
	},
	"BLOGFLOW_SERVER_HSTS_MAX_AGE": func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_HSTS_MAX_AGE as int: %w", err)
		}
		c.Server.HSTSMaxAge = n
		return nil
	},
	"BLOGFLOW_SERVER_METRICS_PORT": func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("cannot parse env var BLOGFLOW_SERVER_METRICS_PORT as int: %w", err)
		}
		c.Server.MetricsPort = n
		return nil
	},
	"BLOGFLOW_SITE_HOMEPAGE": func(c *Config, v string) error {
		c.Site.Homepage = v
		return nil
	},
	"BLOGFLOW_CONTENT_POSTS_DIR": func(c *Config, v string) error {
		c.Content.PostsDir = v
		return nil
	},
	"BLOGFLOW_CONTENT_PAGES_DIR": func(c *Config, v string) error {
		c.Content.PagesDir = v
		return nil
	},
}

// secretEnvVars identifies env vars that should be redacted in logs.
var secretEnvVars = map[string]bool{
	"BLOGFLOW_WEBHOOK_SECRET": true,
}

func applyEnvOverrides(cfg *Config, log *slog.Logger) ([]string, error) {
	var applied []string
	for name, setter := range envMap {
		var v string
		var ok bool

		if secretEnvVars[name] {
			var err error
			v, ok, err = envfile.ReadEnvOrFile(name, log)
			if err != nil {
				return nil, err
			}
		} else {
			v, ok = os.LookupEnv(name)
		}

		if !ok {
			continue
		}
		if err := setter(cfg, v); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// Package-level validation maps.
var (
	validStrategies    = map[string]bool{"watch": true, "webhook": true, "sidecar": true, "poll": true}
	validFeedTypes     = map[string]bool{"atom": true, "rss": true}
	validAllowedEvents = map[string]bool{
		"push": true, "ping": true, "pull_request": true,
		"release": true, "workflow_dispatch": true,
	}
)

// Validate checks a Config for structural and semantic correctness.
func Validate(cfg *Config) error {
	var errs []FieldError

	// Server.Port: 1-65535
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		errs = append(errs, FieldError{
			Field:   "server.port",
			Value:   cfg.Server.Port,
			Message: "must be between 1 and 65535",
		})
	}

	// Server.MetricsPort: 0 = disabled (metrics on main port); 1-65535 = separate listener
	if cfg.Server.MetricsPort != 0 {
		if cfg.Server.MetricsPort < 1 || cfg.Server.MetricsPort > 65535 {
			errs = append(errs, FieldError{
				Field:   "server.metrics_port",
				Value:   cfg.Server.MetricsPort,
				Message: "must be between 1 and 65535",
			})
		}
		if cfg.Server.MetricsPort == cfg.Server.Port {
			errs = append(errs, FieldError{
				Field:   "server.metrics_port",
				Value:   cfg.Server.MetricsPort,
				Message: "must be different from server.port",
			})
		}
	}

	// Server timeouts: must be > 0
	if cfg.Server.ReadTimeout <= 0 {
		errs = append(errs, FieldError{
			Field:   "server.read_timeout",
			Value:   cfg.Server.ReadTimeout,
			Message: "must be greater than 0",
		})
	}
	if cfg.Server.WriteTimeout <= 0 {
		errs = append(errs, FieldError{
			Field:   "server.write_timeout",
			Value:   cfg.Server.WriteTimeout,
			Message: "must be greater than 0",
		})
	}
	if cfg.Server.IdleTimeout <= 0 {
		errs = append(errs, FieldError{
			Field:   "server.idle_timeout",
			Value:   cfg.Server.IdleTimeout,
			Message: "must be greater than 0",
		})
	}

	// Site.BaseURL: must have http/https scheme and non-empty host
	if u, err := url.Parse(cfg.Site.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, FieldError{
			Field:   "site.base_url",
			Value:   cfg.Site.BaseURL,
			Message: "must be a valid URL with http or https scheme and a non-empty host",
		})
	}

	// Site.Homepage: must be "post_list", "page:<slug>", or "static:<path>"
	if hp := cfg.Site.Homepage; hp != "" && hp != "post_list" {
		switch {
		case strings.HasPrefix(hp, "page:") && strings.TrimPrefix(hp, "page:") != "":
			// valid
		case strings.HasPrefix(hp, "static:"):
			sp := strings.TrimPrefix(hp, "static:")
			if sp == "" {
				errs = append(errs, FieldError{
					Field:   "site.homepage",
					Value:   hp,
					Message: `static: requires a non-empty file path`,
				})
			} else if !fs.ValidPath(sp) {
				errs = append(errs, FieldError{
					Field:   "site.homepage",
					Value:   hp,
					Message: `static path must be a clean, relative fs.FS path (no "..", no leading "/")`,
				})
			}
		default:
			errs = append(errs, FieldError{
				Field:   "site.homepage",
				Value:   hp,
				Message: `must be "post_list", "page:<slug>", or "static:<path>" with a non-empty value`,
			})
		}
	}

	// Content paths: no absolute paths, no ".."
	for _, pc := range []struct {
		field string
		value string
	}{
		{"content.posts_dir", cfg.Content.PostsDir},
		{"content.pages_dir", cfg.Content.PagesDir},
		{"content.media_dir", cfg.Content.MediaDir},
	} {
		if filepath.IsAbs(pc.value) {
			errs = append(errs, FieldError{
				Field:   pc.field,
				Value:   pc.value,
				Message: "must be a relative path (no leading /)",
			})
		}
		if strings.Contains(pc.value, "..") {
			errs = append(errs, FieldError{
				Field:   pc.field,
				Value:   pc.value,
				Message: "must not contain '..' (path traversal)",
			})
		}
	}

	// Content.PostsPerPage: 1-100
	if cfg.Content.PostsPerPage < 1 || cfg.Content.PostsPerPage > 100 {
		errs = append(errs, FieldError{
			Field:   "content.posts_per_page",
			Value:   cfg.Content.PostsPerPage,
			Message: "must be between 1 and 100",
		})
	}

	// Content.DateFormat: must not be empty
	if cfg.Content.DateFormat == "" {
		errs = append(errs, FieldError{
			Field:   "content.date_format",
			Value:   cfg.Content.DateFormat,
			Message: "must not be empty",
		})
	}

	// Content.SummaryLength: 50-1000
	if cfg.Content.SummaryLength < 50 || cfg.Content.SummaryLength > 1000 {
		errs = append(errs, FieldError{
			Field:   "content.summary_length",
			Value:   cfg.Content.SummaryLength,
			Message: "must be between 50 and 1000",
		})
	}

	// ThemeConfig.Path: if non-empty, no absolute paths, no ".."
	if cfg.Theme.Path != "" {
		if filepath.IsAbs(cfg.Theme.Path) {
			errs = append(errs, FieldError{
				Field:   "theme.path",
				Value:   cfg.Theme.Path,
				Message: "must be a relative path (no leading /)",
			})
		}
		if strings.Contains(cfg.Theme.Path, "..") {
			errs = append(errs, FieldError{
				Field:   "theme.path",
				Value:   cfg.Theme.Path,
				Message: "must not contain '..' (path traversal)",
			})
		}
	}

	// Cache.MaxEntries: 0-100000
	if cfg.Cache.MaxEntries < 0 || cfg.Cache.MaxEntries > 100000 {
		errs = append(errs, FieldError{
			Field:   "cache.max_entries",
			Value:   cfg.Cache.MaxEntries,
			Message: "must be between 0 and 100000",
		})
	}

	// Cache.TTL: must be > 0 and <= 24h when cache is enabled
	if cfg.Cache.Enabled {
		if cfg.Cache.TTL <= 0 || cfg.Cache.TTL > 24*time.Hour {
			errs = append(errs, FieldError{
				Field:   "cache.ttl",
				Value:   cfg.Cache.TTL,
				Message: "must be > 0 and <= 24h when cache is enabled",
			})
		}
	}

	// Sync.Strategy: must be watch, webhook, sidecar, or poll
	if !validStrategies[cfg.Sync.Strategy] {
		errs = append(errs, FieldError{
			Field:   "sync.strategy",
			Value:   cfg.Sync.Strategy,
			Message: "must be one of: watch, webhook, sidecar, poll",
		})
	}

	// Sync.PollInterval: if set, must be a valid duration >= 30s
	if cfg.Sync.PollInterval != "" {
		d, err := time.ParseDuration(cfg.Sync.PollInterval)
		if err != nil {
			errs = append(errs, FieldError{
				Field:   "sync.poll_interval",
				Value:   cfg.Sync.PollInterval,
				Message: "must be a valid Go duration (e.g. \"5m\")",
			})
		} else if d < 30*time.Second {
			errs = append(errs, FieldError{
				Field:   "sync.poll_interval",
				Value:   cfg.Sync.PollInterval,
				Message: "must be >= 30s to avoid excessive load",
			})
		}
	}

	// Sync.Strategy "poll" requires poll_interval
	if cfg.Sync.Strategy == "poll" && cfg.Sync.PollInterval == "" {
		errs = append(errs, FieldError{
			Field:   "sync.poll_interval",
			Value:   cfg.Sync.PollInterval,
			Message: "must be set when strategy is \"poll\"",
		})
	}

	// Sync.CloneDepth: must be >= 1
	if cfg.Sync.CloneDepth < 1 {
		errs = append(errs, FieldError{
			Field:   "sync.clone_depth",
			Value:   cfg.Sync.CloneDepth,
			Message: "must be >= 1",
		})
	}

	// Webhook-specific validation only when strategy is webhook
	if cfg.Sync.Strategy == "webhook" {
		if len(cfg.Sync.Webhook.Secret) < 32 {
			errs = append(errs, FieldError{
				Field:   "sync.webhook.secret",
				Value:   "[REDACTED]",
				Message: "must be >= 32 bytes (256 bits)",
			})
		}
		if len(cfg.Sync.Webhook.AllowedEvents) == 0 {
			errs = append(errs, FieldError{
				Field:   "sync.webhook.allowed_events",
				Value:   cfg.Sync.Webhook.AllowedEvents,
				Message: "must not be empty when strategy is webhook",
			})
		}
		for _, ev := range cfg.Sync.Webhook.AllowedEvents {
			if !validAllowedEvents[ev] {
				errs = append(errs, FieldError{
					Field:   "sync.webhook.allowed_events",
					Value:   ev,
					Message: "unknown event; allowed: push, ping, pull_request, release, workflow_dispatch",
				})
			}
		}
		if !strings.HasPrefix(cfg.Sync.Webhook.Path, "/") {
			errs = append(errs, FieldError{
				Field:   "sync.webhook.path",
				Value:   cfg.Sync.Webhook.Path,
				Message: "must start with /",
			})
		}
		if cfg.Sync.Webhook.RateLimit < 1 || cfg.Sync.Webhook.RateLimit > 100 {
			errs = append(errs, FieldError{
				Field:   "sync.webhook.rate_limit",
				Value:   cfg.Sync.Webhook.RateLimit,
				Message: "must be between 1 and 100",
			})
		}
		// MaxBodySize: 0 means default (1 MB); if explicitly set, must be 1 B–10 MB.
		const maxAllowedBodySize int64 = 10 << 20 // 10 MB
		if cfg.Sync.Webhook.MaxBodySize < 0 || cfg.Sync.Webhook.MaxBodySize > maxAllowedBodySize {
			errs = append(errs, FieldError{
				Field:   "sync.webhook.max_body_size",
				Value:   cfg.Sync.Webhook.MaxBodySize,
				Message: "must be between 0 (default 1 MB) and 10485760 (10 MB)",
			})
		}
		// AllowedIPs: each entry must be a valid IP or CIDR
		for j, ip := range cfg.Sync.Webhook.AllowedIPs {
			if err := validateCIDROrIP(ip); err != nil {
				fe := err.(*FieldError)
				errs = append(errs, FieldError{
					Field:   fmt.Sprintf("sync.webhook.allowed_ips[%d]", j),
					Value:   ip,
					Message: fe.Message,
				})
			}
		}
	}

	// Server.HSTSMaxAge: 0–63072000 (2 years); only emitted when TLSTerminated
	if cfg.Server.HSTSMaxAge < 0 || cfg.Server.HSTSMaxAge > 63072000 {
		errs = append(errs, FieldError{
			Field:   "server.hsts_max_age",
			Value:   cfg.Server.HSTSMaxAge,
			Message: "must be between 0 and 63072000 (2 years)",
		})
	}

	// Server.TrustedProxyCIDRs: each entry must be a valid IP or CIDR
	for i, cidr := range cfg.Server.TrustedProxyCIDRs {
		if err := validateCIDROrIP(cidr); err != nil {
			fe := err.(*FieldError)
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("server.trusted_proxy_cidrs[%d]", i),
				Value:   cidr,
				Message: fe.Message,
			})
		}
	}

	// Feed validation only when feed is enabled
	if cfg.Feed.Enabled {
		// Feed.Type: must be atom or rss
		if !validFeedTypes[cfg.Feed.Type] {
			errs = append(errs, FieldError{
				Field:   "feed.type",
				Value:   cfg.Feed.Type,
				Message: "must be one of: atom, rss",
			})
		}

		// Feed.Items: 1-100
		if cfg.Feed.Items < 1 || cfg.Feed.Items > 100 {
			errs = append(errs, FieldError{
				Field:   "feed.items",
				Value:   cfg.Feed.Items,
				Message: "must be between 1 and 100",
			})
		}
	}

	if len(errs) > 0 {
		return &ConfigError{Errors: errs}
	}
	return nil
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
