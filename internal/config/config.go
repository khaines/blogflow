package config

import "time"

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

type SiteConfig struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	BaseURL     string       `yaml:"base_url"`
	Language    string       `yaml:"language"`
	Author      AuthorConfig `yaml:"author"`
}

type AuthorConfig struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

type ContentConfig struct {
	PostsDir      string `yaml:"posts_dir"`
	PagesDir      string `yaml:"pages_dir"`
	MediaDir      string `yaml:"media_dir"`
	PostsPerPage  int    `yaml:"posts_per_page"`
	DateFormat    string `yaml:"date_format"`
	SummaryLength int    `yaml:"summary_length"`
}

type ThemeConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	TTL        time.Duration `yaml:"ttl"`
	MaxEntries int           `yaml:"max_entries"`
}

type SyncConfig struct {
	Strategy string        `yaml:"strategy"`
	Webhook  WebhookConfig `yaml:"webhook"`
}

type WebhookConfig struct {
	Path          string   `yaml:"path"`
	Secret        string   `yaml:"-"` // never from YAML — env var only
	AllowedEvents []string `yaml:"allowed_events"`
	BranchFilter  string   `yaml:"branch_filter"`
	IPAllowlist   bool     `yaml:"ip_allowlist"`
	RateLimit     int      `yaml:"rate_limit"`
}

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
			Port:         8080,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Cache: CacheConfig{
			Enabled:    true,
			TTL:        1 * time.Hour,
			MaxEntries: 1000,
		},
		Sync: SyncConfig{
			Strategy: "watch",
			Webhook: WebhookConfig{
				Path:          "/api/webhook",
				AllowedEvents: []string{"push"},
				BranchFilter:  "main",
				IPAllowlist:   true,
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
