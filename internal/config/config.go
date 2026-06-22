// Package config provides BlogFlow's configuration loading, validation,
// and access. It reads YAML files through a 2-layer overlay (config + defaults),
// applies environment variable overrides, and exposes an immutable Config struct.
package config

import (
	"log/slog"
	"time"
)

// Config is the top-level BlogFlow configuration.
// Returned by Loader.Get() — treat as immutable after load.
type Config struct {
	Site    SiteConfig    `yaml:"site"`
	Content ContentConfig `yaml:"content"`
	Theme   ThemeConfig   `yaml:"theme"`
	Server  ServerConfig  `yaml:"server"`
	Cache   CacheConfig   `yaml:"cache"`
	Sync    SyncConfig    `yaml:"sync"`
	Feed    FeedConfig    `yaml:"feed"`
}

// SiteConfig holds site identity settings.
type SiteConfig struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	BaseURL     string       `yaml:"base_url"`
	Language    string       `yaml:"language"`
	Author      AuthorConfig `yaml:"author"`
	Homepage    string       `yaml:"homepage"` // "post_list" (default), "page:<slug>", or "static:<path>"
	Social      SocialConfig `yaml:"social"`
}

// SocialConfig holds social media account identifiers.
type SocialConfig struct {
	Twitter string `yaml:"twitter"` // Twitter/X username (without @)
}

// AuthorConfig holds the site author details.
type AuthorConfig struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// ContentConfig holds content directory and display settings.
type ContentConfig struct {
	PostsDir      string `yaml:"posts_dir"`
	PagesDir      string `yaml:"pages_dir"`
	MediaDir      string `yaml:"media_dir"`
	PostsPerPage  int    `yaml:"posts_per_page"`
	DateFormat    string `yaml:"date_format"`
	SummaryLength int    `yaml:"summary_length"`
}

// ThemeConfig holds theme selection settings.
type ThemeConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port              int           `yaml:"port"`
	MetricsPort       int           `yaml:"metrics_port"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	TLSTerminated     bool          `yaml:"tls_terminated"`
	HSTSMaxAge        int           `yaml:"hsts_max_age"`
	TrustedProxyCIDRs []string      `yaml:"trusted_proxy_cidrs"`
}

// CacheConfig holds rendered content cache settings.
type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	TTL        time.Duration `yaml:"ttl"`
	MaxEntries int           `yaml:"max_entries"`
}

// SyncConfig holds content sync strategy settings.
type SyncConfig struct {
	Strategy     string        `yaml:"strategy"`
	Repo         string        `yaml:"repo"`
	Branch       string        `yaml:"branch"`
	CloneDepth   int           `yaml:"clone_depth"`   // git clone/pull depth; default 1, recommended 10
	PollInterval string        `yaml:"poll_interval"` // Go duration (e.g. "5m"); empty = disabled
	SparseDirs   []string      `yaml:"sparse_dirs"`   // limit checkout to these dirs; empty = full checkout
	Webhook      WebhookConfig `yaml:"webhook"`
}

// WebhookConfig holds webhook receiver settings.
// IP allowlisting is handled at the application layer via AllowedIPs ([]string); when
// non-empty, only IPs in the list are permitted (others receive HTTP 403). Empty or
// nil means no filtering — this mirrors the old note about infrastructure-layer allowlists
// still being recommended for defense-in-depth (reverse proxy, K8s NetworkPolicy).
type WebhookConfig struct {
	Path          string   `yaml:"path"`
	Secret        string   `yaml:"-"` // never from YAML — env var only
	AllowedEvents []string `yaml:"allowed_events"`
	AllowedIPs    []string `yaml:"allowed_ips"`
	BranchFilter  string   `yaml:"branch_filter"`
	RateLimit     int      `yaml:"rate_limit"`
	MaxBodySize   int64    `yaml:"max_body_size"` // max POST body in bytes; 0 = default (1 MB)
}

// FeedConfig holds RSS/Atom feed settings.
type FeedConfig struct {
	Enabled bool   `yaml:"enabled"`
	Type    string `yaml:"type"`
	Items   int    `yaml:"items"`
}

// Default returns a Config with sensible defaults.
// This is what the binary uses when no external config is provided.
func Default() *Config {
	return &Config{
		Site: SiteConfig{
			Title:       "My Blog",
			Description: "A blog powered by BlogFlow",
			BaseURL:     "http://localhost:8080",
			Language:    "en",
			Author:      AuthorConfig{}, // intentionally empty per design
			Homepage:    "post_list",
		},
		Content: ContentConfig{
			PostsDir:      "posts",
			PagesDir:      "pages",
			MediaDir:      "media",
			PostsPerPage:  10,
			DateFormat:    "January 2, 2006",
			SummaryLength: 200,
		},
		Theme: ThemeConfig{
			Name: "default",
		},
		Server: ServerConfig{
			Port:          8080,
			ReadTimeout:   5 * time.Second,
			WriteTimeout:  10 * time.Second,
			IdleTimeout:   120 * time.Second,
			TLSTerminated: false,
			HSTSMaxAge:    63072000, // 2 years
		},
		Cache: CacheConfig{
			Enabled:    true,
			TTL:        1 * time.Hour,
			MaxEntries: 1000,
		},
		Sync: SyncConfig{
			Strategy:   "watch",
			Branch:     "main",
			CloneDepth: 1,
			Webhook: WebhookConfig{
				Path:          "/api/webhook",
				AllowedEvents: []string{"push"},
				BranchFilter:  "main",
				RateLimit:     10,
			},
		},
		Feed: FeedConfig{
			Enabled: true,
			Type:    "atom",
			Items:   20,
		},
	}
}

// LogValue implements slog.LogValuer, redacting sensitive fields.
func (c Config) LogValue() slog.Value {
	type noMethods Config // break recursion
	r := noMethods(c)
	if r.Sync.Webhook.Secret != "" {
		r.Sync.Webhook.Secret = "[REDACTED]"
	}
	return slog.AnyValue(r)
}
