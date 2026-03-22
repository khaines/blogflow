# Configuration System — Design Document

> **Status**: Approved  
> **Author**: Cloud-Native Distributed Systems Architect  
> **Reviewers**: Cloud-Native Security SME, Cloud-Native Systems Engineer, Cloud-Native SRE  
> **Last Updated**: 2026-03-22  

---

## 1 · Overview

### 1.1 What This Component Is

The Configuration System (`internal/config`) is BlogFlow's centralized settings loader and validator. It reads site configuration from YAML files through the overlay filesystem's config and defaults layers, merges in environment variable overrides, validates the result, and exposes a typed, immutable `Config` struct to every other subsystem. It is the second component in the dependency chain — built on top of the overlay FS and consumed by the theme engine, content pipeline, HTTP server, and git-sync strategies.

### 1.2 Functionality It Provides

- **YAML-based configuration** — loads `site.yaml` from the config layer with fallback to the embedded `defaults.yaml`
- **Environment variable overrides** — `BLOGFLOW_` prefixed env vars override any YAML value at runtime
- **Embedded zero-config defaults** — a compiled-in `defaults.yaml` ensures the binary is fully functional with no external files
- **Startup validation** — fails fast on invalid configuration with actionable error messages
- **Secret isolation** — enforces that sensitive values (webhook secrets, git tokens) are injected only via env vars, never in YAML files
- **Config reloading** — supports file-change-triggered reload for the `watch` sync strategy without process restart
- **Typed access** — all settings are accessed through a strongly-typed Go struct with compile-time safety
- **Layer isolation** — reads exclusively from config + defaults layers, preventing theme or content repos from shadowing operator configuration

### 1.3 Why It Is Important

Every BlogFlow subsystem needs configuration — the HTTP server needs a port, the content pipeline needs directory paths, the theme engine needs a theme name, and the webhook handler needs rate limits and secrets. Without a centralized, validated configuration system, each subsystem would independently parse files and env vars, leading to inconsistent behavior, duplicated validation, and security blind spots. The configuration system provides a single source of truth that is validated once at startup and shared safely across goroutines.

### 1.4 Requirements Traceability

| Requirement | Version | Priority | Summary |
|-------------|---------|----------|---------|
| REQ-CFG-001 | v1 | P0 | Load configuration from YAML files through overlay FS (config + defaults layers only) |
| REQ-CFG-002 | v1 | P0 | Environment variable overrides with `BLOGFLOW_` prefix |
| REQ-CFG-003 | v1 | P0 | Embedded `defaults.yaml` provides zero-config operation |
| REQ-CFG-004 | v1 | P0 | Startup validation with fail-fast on invalid config |
| REQ-CFG-005 | v1 | P0 | Secret values only via env vars — reject secret patterns in YAML |
| REQ-CFG-006 | v1 | P1 | Config file hot-reload on file change |
| REQ-CFG-007 | v1 | P1 | Structured logging and metrics for config operations |
| REQ-CFG-008 | v1 | P0 | Strict YAML unmarshalling — reject unknown fields |
| REQ-CFG-009 | v1 | P1 | Config file size limit (1 MB) to prevent resource exhaustion |

---

## 2 · Logical Architecture

### 2.1 High-Level Architecture

```mermaid
graph TB
    subgraph "BlogFlow Engine"
        direction TB
        Server["HTTP Server<br/>(internal/server)"]
        Theme["Theme Engine<br/>(internal/theme)"]
        Content["Content Pipeline<br/>(internal/content)"]
        GitOps["Git Ops<br/>(internal/gitops)"]
        Config["<strong>Config System</strong><br/>(internal/config)"]
        OFS["Overlay FS<br/>(internal/overlayfs)"]
    end

    subgraph "Config Sources (Precedence Order)"
        ENV["Environment Variables<br/>BLOGFLOW_*<br/><em>Highest priority</em>"]
        YAML["site.yaml<br/>/data/config/site.yaml<br/><em>User config</em>"]
        DEFAULTS["defaults.yaml<br/>embed.FS<br/><em>Lowest priority</em>"]
    end

    Server --> Config
    Theme --> Config
    Content --> Config
    GitOps --> Config

    Config --> OFS
    OFS --> L3["/data/config/<br/>Config Layer"]
    OFS --> L4["embed.FS<br/>Defaults Layer"]
    ENV -.->|override| Config
    YAML -.->|via overlay FS| L3
    DEFAULTS -.->|compiled in| L4

    FSNotify["fsnotify watcher"] -.->|reload signal| Config
```

**Key constraint**: The config system constructs a **2-layer overlay** (config + defaults only). It does NOT use the full 4-layer overlay (theme → content → config → defaults) to prevent namespace shadowing. This is a deliberate namespace partition documented in the [Overlay Filesystem design](overlay-filesystem.md) §2.7 and §6.3.

### 2.2 Component Boundaries & Responsibilities

| Responsibility | Owned by This Component | Owned by |
|----------------|:-----------------------:|----------|
| YAML file loading and parsing | ✅ | — |
| Environment variable reading and mapping | ✅ | — |
| Config validation and error reporting | ✅ | — |
| Secret pattern detection in YAML files | ✅ | — |
| Config struct definition and typing | ✅ | — |
| Reload orchestration on file change | ✅ | — |
| Config file size enforcement | ✅ | — |
| Overlay FS layer construction | ❌ | `internal/overlayfs` |
| File system watching (fsnotify) | ❌ | `internal/gitops` |
| Secret storage and injection | ❌ | Runtime environment (K8s Secrets, env vars) |
| HTTP server startup with config values | ❌ | `internal/server` |
| Theme selection and loading | ❌ | `internal/theme` |
| Content directory scanning | ❌ | `internal/content` |

### 2.3 Data Flow

#### Happy Path — Startup Config Load

```mermaid
sequenceDiagram
    participant Main as cmd/blogflow
    participant Cfg as Config System
    participant OFS as OverlayFS (2-layer)
    participant CL as Config Layer (disk)
    participant DL as Defaults Layer (embed)
    participant Env as Environment

    Main->>Cfg: Load(ctx, configPath, defaultsFS)
    Cfg->>OFS: NewOverlayFS(configLayer, defaultsLayer)
    Cfg->>OFS: ReadFile(ctx, "site.yaml")
    OFS->>CL: Open("site.yaml")
    alt site.yaml exists on disk
        CL-->>OFS: fs.File (found)
        OFS-->>Cfg: []byte (YAML content)
    else site.yaml not on disk
        CL-->>OFS: fs.ErrNotExist
        OFS->>DL: Open("site.yaml")
        DL-->>OFS: fs.File (embedded defaults)
        OFS-->>Cfg: []byte (default YAML)
    end
    Cfg->>Cfg: CheckFileSize(≤ 1 MB)
    Cfg->>Cfg: StrictUnmarshal(yaml → Config struct)
    Cfg->>Cfg: RejectSecretPatterns(yaml content)
    Cfg->>Env: Read BLOGFLOW_* env vars
    Cfg->>Cfg: ApplyEnvOverrides(Config struct)
    Cfg->>Cfg: Validate(Config struct)
    Cfg-->>Main: *Config, nil
    Main->>Main: Pass Config to server, theme, content, gitops
```

#### Merge Strategy

The config system uses a **two-phase unmarshal** to merge embedded defaults with user-supplied configuration:

1. **Phase 1 — Load defaults**: The embedded `defaults.yaml` is unmarshalled into a `Config` struct. This populates every field with sensible default values.
2. **Phase 2 — Overlay user config**: If a disk `site.yaml` exists, it is unmarshalled **over the same `Config` struct**. Go's `yaml.v3` Decoder only overwrites fields present in the YAML document — fields absent from `site.yaml` retain their default values from Phase 1.

This means a partial `site.yaml` (e.g., containing only `site.title`) is valid: the title comes from disk, and all other fields use embedded defaults. The overlay FS determines *which file* to read (disk wins if present); the two-phase unmarshal determines *how fields are merged*.

> **Slice field semantics**: For slice fields (e.g., `allowed_events`), if the key is present in `site.yaml` it completely replaces the default — an explicit empty list `[]` clears the default, not inherits it. Operators who want defaults for slice fields must omit the key entirely. Validation: `AllowedEvents` must be non-empty when `Strategy` is `webhook`.

> **Note**: This is NOT the overlay FS merge — the overlay FS returns a single file. The field-level merge happens within the config loader by unmarshalling twice into the same struct.

#### Error Path — Validation Failure

```mermaid
sequenceDiagram
    participant Main as cmd/blogflow
    participant Cfg as Config System
    participant OFS as OverlayFS (2-layer)

    Main->>Cfg: Load(ctx, configPath, defaultsFS)
    Cfg->>OFS: ReadFile(ctx, "site.yaml")
    OFS-->>Cfg: []byte (YAML content)
    Cfg->>Cfg: StrictUnmarshal(yaml → Config struct)
    Cfg->>Cfg: Validate(Config struct)
    Note over Cfg: server.port = -1 (invalid)
    Cfg-->>Main: nil, ConfigValidationError{Field: "server.port", ...}
    Main->>Main: Log ERROR, exit(1)
```

#### Reload Path — File Change

```mermaid
sequenceDiagram
    participant Watcher as fsnotify (internal/gitops)
    participant Cfg as Config System
    participant OFS as OverlayFS (2-layer)
    participant Server as HTTP Server

    Watcher->>Cfg: OnConfigChange()
    Cfg->>OFS: ReadFile(ctx, "site.yaml")
    OFS-->>Cfg: []byte (updated YAML)
    Cfg->>Cfg: StrictUnmarshal + Validate
    alt validation passes
        Cfg->>Cfg: atomicSwap(oldConfig, newConfig)
        Cfg-->>Server: notify subscribers
        Note over Cfg: Metric: config_reload_total{status=success}
    else validation fails
        Note over Cfg: Keep old config active
        Note over Cfg: Log ERROR with validation details
        Note over Cfg: Metric: config_reload_total{status=failure}
    end
```

### 2.4 Data Model / Schema

#### Config Struct Definitions

```go
package config

import "time"

// Config is the root configuration struct. It is immutable after construction —
// consumers receive a pointer and must not modify fields.
type Config struct {
    Site    SiteConfig    `yaml:"site"    validate:"required"`
    Content ContentConfig `yaml:"content" validate:"required"`
    Theme   ThemeConfig   `yaml:"theme"   validate:"required"`
    Server  ServerConfig  `yaml:"server"  validate:"required"`
    Cache   CacheConfig   `yaml:"cache"   validate:"required"`
    Sync    SyncConfig    `yaml:"sync"    validate:"required"`
    Feed    FeedConfig    `yaml:"feed"    validate:"required"`
}

type SiteConfig struct {
    Title       string       `yaml:"title"       validate:"required,min=1,max=200"`
    Description string       `yaml:"description" validate:"max=500"`
    BaseURL     string       `yaml:"base_url"    validate:"required,url"`
    Language    string       `yaml:"language"    validate:"required,bcp47_language_tag"`
    Author      AuthorConfig `yaml:"author"      validate:"required"`
}

type AuthorConfig struct {
    Name  string `yaml:"name"  validate:"required,min=1,max=100"`
    Email string `yaml:"email" validate:"required,email"`
}

type ContentConfig struct {
    PostsDir      string `yaml:"posts_dir"       validate:"required,relpath"`
    PagesDir      string `yaml:"pages_dir"       validate:"required,relpath"`
    MediaDir      string `yaml:"media_dir"       validate:"required,relpath"`
    PostsPerPage  int    `yaml:"posts_per_page"  validate:"required,min=1,max=100"`
    DateFormat    string `yaml:"date_format"     validate:"required"`
    SummaryLength int    `yaml:"summary_length"  validate:"required,min=50,max=1000"`
}

type ThemeConfig struct {
    Name string `yaml:"name" validate:"required,min=1,max=100"`
    Path string `yaml:"path" validate:"omitempty,relpath"` // empty = use embedded defaults
}

type ServerConfig struct {
    Port         int           `yaml:"port"          validate:"required,min=1,max=65535"`
    ReadTimeout  time.Duration `yaml:"read_timeout"  validate:"required,min=1s,max=60s"`
    WriteTimeout time.Duration `yaml:"write_timeout" validate:"required,min=1s,max=300s"`
    IdleTimeout  time.Duration `yaml:"idle_timeout"  validate:"required,min=1s,max=600s"`
}

type CacheConfig struct {
    Enabled    bool          `yaml:"enabled"`
    TTL        time.Duration `yaml:"ttl"          validate:"min=0s,max=24h"`
    MaxEntries int           `yaml:"max_entries"  validate:"min=0,max=100000"`
}

type SyncConfig struct {
    Strategy string        `yaml:"strategy" validate:"required,oneof=watch webhook sidecar"`
    Webhook  WebhookConfig `yaml:"webhook"`
}

type WebhookConfig struct {
    Path          string   `yaml:"path"           validate:"required_if=Strategy webhook,startswith=/"`
    Secret        string   `yaml:"-"` // NEVER from YAML — env var only
    AllowedEvents []string `yaml:"allowed_events" validate:"required_if=Strategy webhook,dive,oneof=push ping"`
    BranchFilter  string   `yaml:"branch_filter"  validate:"required_if=Strategy webhook,max=250"`
    IPAllowlist   bool     `yaml:"ip_allowlist"`
    RateLimit     int      `yaml:"rate_limit"     validate:"required_if=Strategy webhook,min=1,max=100"` // requests per minute
}

// **Implementation note**: `required_if=Strategy webhook` cannot be resolved via struct tags alone because `Strategy` lives in the parent `SyncConfig`, not `WebhookConfig`. Implementation must use a custom cross-struct validator registered with `validate.RegisterStructValidation` (or equivalent), applying webhook-field requirements only when `SyncConfig.Strategy == "webhook"`. The struct tags shown above document the *intent*; the enforcement mechanism is a cross-struct validator.

type FeedConfig struct {
    Enabled bool   `yaml:"enabled"`
    Type    string `yaml:"type"  validate:"oneof=atom rss"`
    Items   int    `yaml:"items" validate:"min=1,max=100"`
}
```

> **IdleTimeout**: `IdleTimeout` controls how long keepalive connections remain open. Set independently from `ReadTimeout`/`WriteTimeout` to prevent slow-loris-style resource exhaustion. Default 120s is appropriate for most deployments.

> **Immutability convention**: The `Config` struct returned by `Get()` is treated as immutable by convention. Fields are exported for read access but consumers MUST NOT modify the returned struct. The `atomic.Pointer` swap ensures each `Get()` returns a consistent snapshot.

> **Logging redaction**: The `Config` struct implements `slog.LogValuer` to redact sensitive fields (webhook secret, any field matching secret patterns). When logged via structured logging, secret values appear as `[REDACTED]`.

> **Path validation**: Directory path fields (`PostsDir`, `PagesDir`, `MediaDir`) and `ThemeConfig.Path` use a custom `relpath` validator that rejects absolute paths (no leading `/`) and paths containing `..` components. The overlay FS provides ultimate containment, but schema-level validation provides defense in depth.

#### YAML Schema — `site.yaml`

```yaml
site:
  title: "My Blog"
  description: "A blog about things"
  base_url: "https://blog.example.com"
  language: "en"
  author:
    name: "Author Name"
    email: "author@example.com"

content:
  posts_dir: "posts"
  pages_dir: "pages"
  media_dir: "media"
  posts_per_page: 10
  date_format: "January 2, 2006"
  summary_length: 200

theme:
  name: "default"
  path: ""  # empty = use embedded defaults

server:
  port: 8080
  read_timeout: "5s"
  write_timeout: "10s"
  idle_timeout: "120s"

cache:
  enabled: true
  ttl: "1h"
  max_entries: 1000

sync:
  strategy: "watch"  # "webhook" | "sidecar" | "watch"
  webhook:
    path: "/api/webhook"
    secret: "${BLOGFLOW_WEBHOOK_SECRET}"  # rejected — must use env var
    allowed_events: ["push"]
    branch_filter: "main"
    ip_allowlist: true
    rate_limit: 10

feed:
  enabled: true
  type: "atom"
  items: 20
```

> **Webhook conditional validation**: Webhook-specific fields (`RateLimit`, `AllowedEvents`, `BranchFilter`) are only validated when `sync.strategy` is `webhook`. Non-webhook deployments can set these to zero/empty without validation errors.

#### Embedded Defaults — `defaults/config/defaults.yaml`

The embedded defaults provide a fully functional configuration. Every field has a sensible value. This file is compiled into the binary via `embed.FS` and serves as the lowest-priority config source.

```yaml
site:
  title: "BlogFlow"
  description: "A blog powered by BlogFlow"
  base_url: "http://localhost:8080"
  language: "en"
  author:
    name: "BlogFlow Author"
    email: "author@example.com"

content:
  posts_dir: "posts"
  pages_dir: "pages"
  media_dir: "media"
  posts_per_page: 10
  date_format: "January 2, 2006"
  summary_length: 200

theme:
  name: "default"
  path: ""

server:
  port: 8080
  read_timeout: "5s"
  write_timeout: "10s"
  idle_timeout: "120s"

cache:
  enabled: true
  ttl: "1h"
  max_entries: 1000

sync:
  strategy: "watch"
  webhook:
    path: "/api/webhook"
    allowed_events: ["push"]
    branch_filter: "main"
    ip_allowlist: true
    rate_limit: 10

feed:
  enabled: true
  type: "atom"
  items: 20
```

### 2.5 API Surface

#### Public Go API

```go
package config

import (
    "context"
    "embed"
    "io/fs"
)

// Loader loads, validates, and manages configuration.
type Loader struct {
    overlay  *overlayfs.ContextOverlayFS // 2-layer: config + defaults
    current  atomic.Pointer[Config]
    subs     []func(*Config)             // reload subscribers
    mu       sync.RWMutex                // protects subs
    logger   *slog.Logger
    metrics  *configMetrics
}

// NewLoader creates a config loader with a 2-layer overlay FS
// (config directory + embedded defaults). If configPath is empty,
// only the embedded defaults layer is used. NewLoader eagerly loads
// embedded defaults so that Get() always returns a non-nil Config,
// even before Load() is called.
func NewLoader(configPath string, defaults embed.FS, logger *slog.Logger) (*Loader, error)

// Load reads site.yaml through the overlay FS, applies env var
// overrides, validates the result, and stores it atomically. Returns
// the validated Config or an error. This is called once at startup.
func (l *Loader) Load(ctx context.Context) (*Config, error)

// Reload re-reads site.yaml, validates, and atomically swaps the
// config if valid. If validation fails, the previous config remains
// active and an error is returned. Notifies all subscribers on success.
func (l *Loader) Reload(ctx context.Context) (*Config, error)

// Get returns the current immutable Config. Safe for concurrent use.
// Get() never returns nil. Before external config is loaded via Load(),
// it returns the embedded defaults populated by NewLoader.
func (l *Loader) Get() *Config

// OnChange registers a callback invoked after a successful reload.
// Callbacks receive the new Config. Used by the HTTP server and
// content pipeline to react to config changes.
// OnChange callbacks are invoked with panic recovery — a panicking
// callback is logged at ERROR and automatically deregistered.
// Callbacks have a 5-second timeout; slow callbacks are logged at WARN.
// Returns a cancel function to deregister the callback.
func (l *Loader) OnChange(fn func(*Config)) (cancel func())

// Validate checks a Config for structural and semantic correctness.
// Returns a ConfigError with all validation failures.
// Exported for use in tests and CLI validation commands.
func Validate(cfg *Config) error
```

#### Error Types

```go
// ConfigError aggregates one or more validation failures.
type ConfigError struct {
    Errors []FieldError
}

func (e *ConfigError) Error() string // "config validation failed: 3 errors"

// FieldError describes a single validation failure.
// When Field matches a secret-pattern path (e.g., sync.webhook.secret),
// Value is automatically set to [REDACTED] by the validation logic.
// The raw secret value is never stored in FieldError.
type FieldError struct {
    Field   string // e.g., "server.port"
    Value   any    // the invalid value (redacted for secret fields)
    Message string // e.g., "must be between 1 and 65535"
}

// SecretInYAMLError is returned when a YAML file contains a value
// matching known secret patterns.
type SecretInYAMLError struct {
    Field   string
    Pattern string // e.g., "looks like a GitHub token (ghp_...)"
}
```

#### Environment Variable Mapping

Environment variables use a `BLOGFLOW_` prefix with underscore-separated paths that map directly to the YAML hierarchy:

| YAML Path | Environment Variable | Example |
|-----------|---------------------|---------|
| `site.title` | `BLOGFLOW_SITE_TITLE` | `BLOGFLOW_SITE_TITLE="My Blog"` |
| `site.base_url` | `BLOGFLOW_SITE_BASE_URL` | `BLOGFLOW_SITE_BASE_URL="https://blog.example.com"` |
| `server.port` | `BLOGFLOW_SERVER_PORT` | `BLOGFLOW_SERVER_PORT=9090` |
| `server.read_timeout` | `BLOGFLOW_SERVER_READ_TIMEOUT` | `BLOGFLOW_SERVER_READ_TIMEOUT=10s` |
| `server.idle_timeout` | `BLOGFLOW_SERVER_IDLE_TIMEOUT` | `BLOGFLOW_SERVER_IDLE_TIMEOUT=120s` |
| `cache.enabled` | `BLOGFLOW_CACHE_ENABLED` | `BLOGFLOW_CACHE_ENABLED=false` |
| `sync.strategy` | `BLOGFLOW_SYNC_STRATEGY` | `BLOGFLOW_SYNC_STRATEGY=webhook` |
| `sync.webhook.secret` | `BLOGFLOW_WEBHOOK_SECRET` | `BLOGFLOW_WEBHOOK_SECRET=whsec_abc123` |
| `sync.webhook.rate_limit` | `BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT` | `BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT=20` |
| `feed.type` | `BLOGFLOW_FEED_TYPE` | `BLOGFLOW_FEED_TYPE=rss` |

**Secret-only variables** (no YAML equivalent):

| Environment Variable | Maps to | Notes |
|---------------------|---------|-------|
| `BLOGFLOW_WEBHOOK_SECRET` | `sync.webhook.secret` | HMAC-SHA256 webhook signing secret |
| `BLOGFLOW_GIT_TOKEN` | Git auth token | Used by `internal/gitops`, not in Config struct |
| `BLOGFLOW_GIT_SSH_KEY` | SSH key path | Used by `internal/gitops`, not in Config struct |

### 2.6 Dependencies

| Dependency | Type | Communication | Failure Behaviour |
|------------|------|---------------|-------------------|
| `internal/overlayfs` | Internal | In-process (2-layer overlay for config + defaults) | If overlay fails, config load fails; startup aborts |
| `gopkg.in/yaml.v3` | Library | In-process | Return parse error with line/column |
| `embed` (stdlib) | Library | In-process (compiled-in `defaults.yaml`) | N/A (always available) |
| `os` (stdlib) | Library | Env var reads | N/A (env vars always readable) |
| `slog` (stdlib) | Library | In-process | N/A (logging) |
| `internal/gitops` | Internal | Callback (reload trigger) | If watcher unavailable, reload disabled; log WARN |

> **Go version**: Go 1.22+ required (inherited from `internal/overlayfs` — see REQ-OFS-008).

> **Reload debounce contract**: `internal/gitops` must debounce file-change events to ≤ 1/second. Additionally, the config loader enforces a minimum reload interval of 100ms as self-protection. Reload requests within this window are coalesced — only the last one takes effect. This prevents reload flooding from rapid fsnotify events or misconfigured watchers.

### 2.7 Content Integrity & Isolation

**Config layer isolation**: The config system constructs its own 2-layer overlay FS using only the config directory and embedded defaults. It deliberately excludes the theme and content layers. This prevents a compromised content or theme repository from injecting or overriding a `site.yaml` file that alters engine behavior. The isolation boundary is enforced in `NewLoader` — it calls `overlayfs.NewOverlayFS(configLayer, defaultsLayer)` directly, never `overlayfs.NewFromPaths` which constructs the full 4-layer stack.

**Strict unmarshalling**: YAML parsing uses `yaml.v3` with `KnownFields(true)`, which rejects any key not defined in the Config struct. This prevents typosquatting attacks (e.g., `servr.port` silently ignored) and blocks injection of unexpected fields.

**Secret pattern detection**: Before unmarshalling, the raw YAML bytes are scanned for patterns known to be secrets:
- GitHub tokens: `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`
- GitHub fine-grained PATs: `github_pat_`
- GitLab tokens: `glpat-`
- Slack tokens: `xoxb-`, `xoxp-`
- OpenAI / generic API keys: `sk-`
- Generic secrets: strings matching `-----BEGIN.*PRIVATE KEY-----`
- AWS keys: strings matching `AKIA[0-9A-Z]{16}`
- Connection strings: `dsn://`, `postgres://`, `mysql://`, `redis://`
- Keyword patterns: `password`, `secret`, `token`, `credential`, `apikey`, `api_key` appearing as YAML values
- Webhook secret placeholders: `${BLOGFLOW_WEBHOOK_SECRET}` (operator mistake — meant to use env var, accidentally put in YAML)

> **Note**: This list is intentionally non-exhaustive. It is supplemented by a recommended git pre-commit hook (e.g., gitleaks) for broader coverage.

If any pattern is detected, config loading fails with a `SecretInYAMLError` directing the operator to use environment variables instead.

**File size limit**: Config files larger than 1 MB are rejected before parsing. This prevents resource exhaustion from a maliciously large config file placed in the config volume.

---

## 3 · Functional Test Scenarios

### 3.1 Happy-Path Scenarios

| # | Scenario | Precondition | Action | Expected Result |
|---|----------|--------------|--------|-----------------|
| 1 | Load config from disk | `site.yaml` exists in config layer with valid content | `Load(ctx)` | Returns Config with values from `site.yaml` |
| 2 | Load embedded defaults | No `site.yaml` on disk | `Load(ctx)` | Returns Config with embedded default values |
| 3 | Disk config overrides defaults | `site.yaml` on disk has `server.port: 9090`, defaults has `8080` | `Load(ctx)` | `Config.Server.Port == 9090` |
| 4 | Env var overrides YAML | `site.yaml` has `port: 8080`, env has `BLOGFLOW_SERVER_PORT=3000` | `Load(ctx)` | `Config.Server.Port == 3000` |
| 5 | Env var overrides defaults | No `site.yaml` on disk, env has `BLOGFLOW_SITE_TITLE="Custom"` | `Load(ctx)` | `Config.Site.Title == "Custom"` |
| 6 | Secret loaded from env | `BLOGFLOW_WEBHOOK_SECRET=whsec_abc` | `Load(ctx)` | `Config.Sync.Webhook.Secret == "whsec_abc"` |
| 7 | Duration parsing | YAML has `read_timeout: "5s"` | `Load(ctx)` | `Config.Server.ReadTimeout == 5 * time.Second` |
| 8 | Bool parsing from env | `BLOGFLOW_CACHE_ENABLED=false` | `Load(ctx)` | `Config.Cache.Enabled == false` |
| 9 | Config reload success | Valid `site.yaml` changes on disk | `Reload(ctx)` | Returns new Config; subscribers notified |
| 10 | Subscriber notification | Subscriber registered via `OnChange` | Successful `Reload(ctx)` | Subscriber callback invoked with new Config |
| 11 | Get returns current config | Config loaded successfully | `Get()` | Returns same Config pointer loaded earlier |
| 12 | Partial site.yaml | Disk `site.yaml` has only `site.title`, rest uses defaults | `Load(ctx)` | Title from disk, all other values from defaults |

### 3.2 Edge Cases & Error Scenarios

| # | Scenario | Input / Condition | Expected Behaviour |
|---|----------|-------------------|--------------------|
| 1 | Invalid YAML syntax | Malformed YAML (missing colon) | Return parse error with line number |
| 2 | Unknown YAML field | `site.yaml` contains `servr: ...` (typo) | Return error: "unknown field `servr`" (strict unmarshalling) |
| 3 | Invalid port number | `server.port: -1` | Return `FieldError{Field: "server.port", Message: "must be between 1 and 65535"}` |
| 4 | Port out of range | `server.port: 70000` | Return `FieldError{Field: "server.port"}` |
| 5 | Missing required field | `site.yaml` with no `site.title` and no default | Return `FieldError{Field: "site.title", Message: "required"}` |
| 6 | Invalid URL | `base_url: "not-a-url"` | Return `FieldError{Field: "site.base_url"}` |
| 7 | Invalid email | `author.email: "notanemail"` | Return `FieldError{Field: "site.author.email"}` |
| 8 | Secret in YAML (GitHub token) | `site.yaml` contains `ghp_abc123...` | Return `SecretInYAMLError`; config NOT loaded |
| 9 | Secret in YAML (private key) | `site.yaml` contains `-----BEGIN RSA PRIVATE KEY-----` | Return `SecretInYAMLError`; config NOT loaded |
| 10 | Secret placeholder in YAML | `webhook.secret: "${BLOGFLOW_WEBHOOK_SECRET}"` | Return `SecretInYAMLError` — meant for env var, not YAML |
| 11 | Config file too large | `site.yaml` is 2 MB | Return error: "config file exceeds 1 MB limit" |
| 12 | Config file is directory | Path resolves to a directory, not a file | Return error: "site.yaml is a directory" |
| 13 | Invalid duration string | `read_timeout: "five seconds"` | Return parse error for duration field |
| 14 | Negative duration | `ttl: "-1h"` | Return `FieldError{Field: "cache.ttl"}` |
| 15 | Reload with invalid config | `site.yaml` changes to invalid content | `Reload` returns error; previous config remains active |
| 16 | Reload with missing file | `site.yaml` deleted from disk | Falls back to embedded defaults; reload succeeds |
| 17 | Invalid sync strategy | `strategy: "poll"` | Return `FieldError{Field: "sync.strategy", Message: "must be one of: watch, webhook, sidecar"}` |
| 18 | Env var with invalid type | `BLOGFLOW_SERVER_PORT=abc` | Return error: "cannot parse env var BLOGFLOW_SERVER_PORT as int" |
| 19 | Concurrent Get during Reload | Multiple goroutines call `Get()` while `Reload()` runs | No race; `Get()` returns either old or new config atomically |
| 20 | Empty config file | `site.yaml` exists but is 0 bytes | Falls through to embedded defaults (empty file yields zero-value struct, defaults fill in) |
| 21 | Invalid feed type | `feed.type: "json"` | Return `FieldError{Field: "feed.type", Message: "must be one of: atom, rss"}` |
| 22 | Webhook secret too short | `BLOGFLOW_WEBHOOK_SECRET=abc` (< 32 bytes) | Return `FieldError{Field: "sync.webhook.secret", Message: "must be ≥ 32 bytes (256 bits)"}` — startup fails |
| 23 | Logging Config does not expose secrets | Config loaded with webhook secret set | `slog.AnyValue(cfg)` output does not contain the literal webhook secret value |
| 24 | Webhook secret too short — FieldError.Value redacted | `BLOGFLOW_WEBHOOK_SECRET` set to `"short"` | `FieldError` with `Value="[REDACTED]"`, not the actual secret |
| 25 | Quoted glob in YAML not rejected | `branch_filter: "feature/*"` in `site.yaml` | Config loads successfully — quoted `*` is not treated as a YAML alias |

### 3.3 Integration Test Boundaries

| Integration Point | Real in Tests | Mocked in Unit Tests | Notes |
|-------------------|:------------:|:-------------------:|-------|
| `internal/overlayfs` | ✅ in integration | ✅ via `fstest.MapFS` in unit | Integration tests use real 2-layer overlay with `t.TempDir()` |
| `embed.FS` (defaults) | ✅ | — | Use `fstest.MapFS` as stand-in for unit tests |
| Environment variables | ✅ via `t.Setenv()` | ✅ via `t.Setenv()` | `t.Setenv` provides test isolation |
| `internal/gitops` (reload trigger) | ❌ | ✅ | Test `Reload()` directly; mock the watcher |
| `internal/server` (config consumer) | ❌ | ✅ | Server tested separately with provided Config |
| Filesystem (config layer) | ✅ in integration | ✅ via `fstest.MapFS` | Integration tests write real YAML files to `t.TempDir()` |

### 3.4 Acceptance Criteria Mapping

| Acceptance Criterion | Test Scenario(s) | Coverage |
|----------------------|-------------------|----------|
| Config loads from YAML through overlay FS | §3.1 #1, #2, #3 | ✅ Covered |
| Env vars override YAML values | §3.1 #4, #5, #8 | ✅ Covered |
| Embedded defaults provide zero-config operation | §3.1 #2 | ✅ Covered |
| Startup validation fails fast | §3.2 #1–#7, #13, #14, #17 | ✅ Covered |
| Secrets rejected in YAML files | §3.2 #8, #9, #10 | ✅ Covered |
| Config reloads on file change | §3.1 #9, #10, §3.2 #15, #16 | ✅ Covered |
| Strict YAML unmarshalling | §3.2 #2 | ✅ Covered |
| Config file size limit enforced | §3.2 #11 | ✅ Covered |
| Thread-safe concurrent access | §3.2 #19 | ✅ Covered |

---

## 4 · Performance

### 4.1 Expected Load Profile

Configuration loading is an infrequent, cold-path operation. It happens once at startup and occasionally on file-change reloads. It is **not** on the hot path for HTTP request serving.

| Operation | Frequency | Duration Budget |
|-----------|-----------|----------------|
| Startup load | Once per process start | ≤ 50 ms |
| Reload (file change) | Rare (~1/hour for active blogs, less for most) | ≤ 50 ms |
| `Get()` (hot path) | Every HTTP request (via subsystem) | ≤ 10 ns (atomic pointer load) |

### 4.2 Latency Targets

| Percentile | Target | Measurement Point |
|------------|--------|-------------------|
| p50 | ≤ 5 ms | `Load()` / `Reload()` — YAML parse + validate |
| p95 | ≤ 20 ms | `Load()` / `Reload()` — includes disk I/O |
| p99 | ≤ 50 ms | `Load()` / `Reload()` — worst case cold disk |

`Get()` is an atomic pointer load — effectively 0 latency, not benchmarked.

### 4.3 Throughput Targets

Config loading is not throughput-sensitive. The system loads config once at startup and reloads at most once per file-change event. Even under rapid git pushes, the reload rate is bounded by fsnotify debouncing (typically 100ms–1s).

- **Sustained**: Not applicable (not a throughput-driven component)
- **Burst**: At most 1 reload/second (debounced by the file watcher)
- **`Get()` throughput**: Millions/sec — atomic pointer load, no contention

### 4.4 Scaling Strategy

The config system is a **singleton** within the BlogFlow process. It does not scale independently. Memory usage is constant (a single `Config` struct, ~2 KB). There is no scaling bottleneck because:

- `Get()` is a lock-free atomic pointer load — scales to any number of goroutines
- `Load()`/`Reload()` is single-threaded and infrequent
- YAML parsing is CPU-bound but completes in single-digit milliseconds for a <1 MB file

### 4.5 Resource Budgets

| Resource | Budget per Replica | Notes |
|----------|-------------------|-------|
| CPU | Negligible (< 0.1% of startup CPU) | Single YAML parse at startup |
| Memory | ~4 KB | Config struct + overlay FS references |
| Storage | 0 (reads from existing volumes) | No writes |

### 4.6 Performance Test Plan

**Benchmark suite** (`config_bench_test.go`):

| Benchmark | Description | Pass Threshold |
|-----------|-------------|---------------|
| `BenchmarkLoad_Defaults` | Load config from embedded defaults only | ≤ 5 ms/op |
| `BenchmarkLoad_DiskOverride` | Load config from disk YAML + defaults merge | ≤ 10 ms/op |
| `BenchmarkLoad_WithEnvOverrides` | Load with 10 env var overrides applied | ≤ 10 ms/op |
| `BenchmarkGet_Concurrent` | 8 goroutines calling `Get()` in parallel | ≤ 50 ns/op, no race |
| `BenchmarkReload` | Full reload cycle (read + parse + validate + swap) | ≤ 20 ms/op |
| `BenchmarkValidate` | Validate a fully populated Config struct | ≤ 1 ms/op |

Benchmarks run on every PR via `go test -bench=. -benchmem -count=5 ./internal/config/...`.

---

## 5 · Security

### 5.1 Authentication & Authorization

The config system does not handle authentication or authorization. It is an internal component that reads local files and environment variables. Access control is enforced at two levels:

- **Filesystem permissions**: The config volume (`/data/config/`) is mounted with read-only permissions for UID 65532 (nonroot). The BlogFlow process cannot modify its own config files.
- **Environment variable injection**: Secrets are injected by the container runtime (K8s Secrets, Docker secrets, systemd EnvironmentFile). The config system only reads them — it cannot modify or exfiltrate them.

### 5.2 Data Classification & Encryption

| Data Element | Classification | Encrypted at Rest | Encrypted in Transit |
|-------------|----------------|:-----------------:|:--------------------:|
| `site.yaml` (user config) | Internal | N/A (filesystem) | N/A (local read) |
| `defaults.yaml` (embedded) | Public | N/A (compiled in) | N/A (local read) |
| `BLOGFLOW_WEBHOOK_SECRET` | Confidential | ✅ (K8s Secret / KMS) | N/A (env var, in-memory) |
| `BLOGFLOW_GIT_TOKEN` | Confidential | ✅ (K8s Secret / KMS) | N/A (env var, in-memory) |
| Config struct (in-memory) | Internal | N/A (runtime memory) | N/A (not transmitted) |

### 5.3 Input Validation & Sanitization

**YAML input validation** (defense-in-depth layers):

| Layer | Check | Implementation | Threat Mitigated |
|-------|-------|---------------|-----------------|
| 1 | File size limit | `len(bytes) ≤ 1 MB` before parsing | Resource exhaustion via large config |
| 2 | Secret pattern scan | Regex scan of raw bytes before parsing | Accidental secret commit to config repo |
| 3 | Strict unmarshalling | `yaml.Decoder.KnownFields(true)` | Typosquatting, unexpected field injection |
| 4 | Struct tag validation | `validate:"required,min=1,max=65535"` etc. | Out-of-range values, missing required fields |
| 5 | Semantic validation | Custom rules (e.g., `write_timeout ≥ read_timeout`) | Logically inconsistent configurations |

**Environment variable validation:**

| Check | Implementation | Threat Mitigated |
|-------|---------------|-----------------|
| Type conversion | `strconv.Atoi`, `time.ParseDuration`, `strconv.ParseBool` | Type confusion from string env vars |
| Same struct tag validation | Env-overridden fields pass through the same `Validate()` | Env vars bypassing YAML validation |
| Prefix enforcement | Only `BLOGFLOW_*` vars are read | Namespace collision with other process env vars |
| Webhook secret entropy | `BLOGFLOW_WEBHOOK_SECRET` must be ≥ 32 bytes (256 bits) | Trivially brute-forceable HMAC secret; startup fails with clear error if too short |

### 5.4 Content Integrity

**Config isolation from content/theme repos**: The most critical security property of the config system is that it reads ONLY from the config and defaults layers. A compromised content or theme repository cannot inject a `site.yaml` that changes the server port, disables webhook signature verification, or alters sync behavior. This is enforced architecturally — `NewLoader` constructs a 2-layer overlay, not a 4-layer overlay.

**YAML anchor/alias rejection**: Config files containing YAML anchors (`&`) or aliases (`*`) are rejected during the pre-validation scan. The scan tokenizes raw YAML bytes and flags `&` and `*` only when they appear as the first non-whitespace character of a bare (unquoted) YAML scalar value. Characters inside single- or double-quoted strings (e.g., `branch_filter: "feature/*"`) are not rejected. Legitimate BlogFlow config files have no reason to use YAML anchors. This eliminates billion-laughs/alias expansion attacks entirely. The 1 MB file size limit provides an additional safeguard against oversized payloads.

**No template execution in config**: Config values are never passed through `text/template` or `html/template`. The `${BLOGFLOW_WEBHOOK_SECRET}` syntax in YAML is **not** variable interpolation — it is detected as a secret pattern and rejected. Env var values are applied programmatically, not via string substitution.

---

## 6 · Threat Model

### 6.1 Trust Boundaries

```mermaid
graph LR
    subgraph "Fully Trusted"
        Embed["embed.FS<br/>(compiled defaults.yaml)"]
        EnvVars["Environment Variables<br/>(K8s Secrets / runtime)"]
    end
    subgraph "Semi-Trusted — Operator-Controlled"
        ConfigDir["/data/config/site.yaml<br/>(config volume)"]
    end
    subgraph "Untrusted — Excluded"
        ThemeDir["/data/theme/<br/>(NOT read by config)"]
        ContentDir["/data/content/<br/>(NOT read by config)"]
    end
    subgraph "Config Boundary"
        CfgSys["Config System<br/>Parse · Validate · Reject secrets"]
    end
    subgraph "Consumers — Internal"
        Server["HTTP Server"]
        Theme["Theme Engine"]
        Content["Content Pipeline"]
        GitOps["Git Ops"]
    end

    Embed -->|immutable defaults| CfgSys
    EnvVars -->|secret injection + overrides| CfgSys
    ConfigDir -->|user YAML| CfgSys
    ThemeDir -.->|BLOCKED| CfgSys
    ContentDir -.->|BLOCKED| CfgSys
    CfgSys -->|validated *Config| Server
    CfgSys -->|validated *Config| Theme
    CfgSys -->|validated *Config| Content
    CfgSys -->|validated *Config| GitOps
```

### 6.2 Threat Actors & Attack Surfaces

| Threat Actor | Attack Surface | Motivation |
|-------------|----------------|------------|
| Compromised content repo | Config layer namespace (blocked by isolation) | Override server settings, disable security features |
| Compromised theme repo | Config layer namespace (blocked by isolation) | Alter cache TTL, change sync strategy |
| Malicious config author | `site.yaml` file content | Inject secrets into version-controlled file, misconfigure server |
| Environment variable injection | Process environment | Override config with malicious values (requires container access) |
| YAML bomb (DoS) | `site.yaml` file size and structure | Exhaust memory during YAML parsing |
| Config volume tampering | `/data/config/` mount | Replace config file with malicious content |

### 6.3 STRIDE Analysis

| Threat Category | Applicable? | Threat Description | Mitigation |
|----------------|:-----------:|--------------------|------------|
| **S**poofing | ❌ | Config system has no identity/auth — not applicable | N/A |
| **T**ampering | ✅ | Attacker modifies `site.yaml` to change server behavior (e.g., disable webhook signature verification, change port to expose an unmonitored port) | Config volume mounted read-only in production; git-backed config with PR review; validation rejects dangerous combinations; env var overrides for critical settings |
| **R**epudiation | ✅ | Config change goes unnoticed — operator denies responsibility | Config loaded from git-tracked file (audit trail); reload events logged with source and diff summary; metrics track reload count |
| **I**nformation Disclosure | ✅ | Secrets accidentally committed to `site.yaml` and pushed to git | Secret pattern detection rejects config at load time; `WebhookConfig.Secret` uses `yaml:"-"` tag (never read from YAML); secret fields excluded from config dump/log output |
| **D**enial of Service | ✅ | YAML bomb (alias expansion) or oversized config file exhausts memory/CPU | 1 MB file size limit enforced before parsing; strict unmarshalling bounds field count; YAML anchors (`&`) and aliases (`*`) rejected during pre-validation scan — eliminates alias expansion attacks entirely |
| **E**levation of Privilege | ✅ | Content repo places `site.yaml` in content directory to override operator config | Config reads ONLY from config + defaults layers (2-layer overlay); content and theme layers excluded; architectural enforcement in `NewLoader` |

### 6.4 Mitigations & Residual Risks

| Threat | Mitigation | Residual Risk |
|--------|-----------|---------------|
| Config tampering via volume | Read-only volume mount; git-backed config; PR review | **Low** — if K8s RBAC compromised, attacker can remount volume writable. Mitigated by cluster-level controls. |
| Secret in YAML | Pattern detection; `yaml:"-"` struct tag; load-time rejection | **Low** — patterns may miss novel secret formats. Mitigated by operator education and git pre-commit hooks. |
| YAML bomb | 1 MB size limit; YAML anchors (`&`) and aliases (`*`) rejected during pre-validation scan | **Negligible** — anchor/alias rejection eliminates the alias expansion attack vector entirely. Size limit prevents oversized payloads. |
| Layer shadowing (EoP) | 2-layer overlay (config + defaults only) | **Negligible** — content and theme layers architecturally excluded from config loading path. |
| Env var injection | Requires container-level access (K8s RBAC) | **Accepted** — if attacker has container access, they have broader capabilities. Config validation still applies to env-overridden values. |
| Config reload with malicious file | Validation on reload; old config preserved on failure | **Low** — validated config only; invalid config never activates. |

---

## 7 · Observability

### 7.1 Logging Strategy

All log entries include `component="config"` for filtering. Secret values are **never** logged — the `WebhookConfig.Secret` field is redacted in all log output.

> **Secret redaction in env override logs**: WARN logs for env var overrides redact values for secret-pattern fields. The log shows the field path and `value=REDACTED`, never the actual value. Secret-only env vars (`BLOGFLOW_WEBHOOK_SECRET`, `BLOGFLOW_GIT_TOKEN`, `BLOGFLOW_GIT_SSH_KEY`) are logged as: `"env var applied" var="BLOGFLOW_WEBHOOK_SECRET" field="sync.webhook.secret" value=REDACTED`.

> **Changed-fields diff algorithm**: The changed-fields diff compares field paths (e.g., `server.port`, `feed.enabled`) without logging values. Only the field path and old≠new boolean are recorded, never the actual values. This prevents accidental secret leakage in structured logs.

| Log Level | When Used | Example |
|-----------|-----------|---------|
| ERROR | Validation failure at startup or reload | `"config validation failed" field="server.port" value=-1 error="must be between 1 and 65535"` |
| ERROR | Secret detected in YAML file | `"secret pattern detected in config file" field="sync.webhook.secret" pattern="ghp_*" action="load rejected"` |
| ERROR | Config file exceeds size limit | `"config file exceeds size limit" path="site.yaml" size_bytes=2097152 limit_bytes=1048576` |
| WARN | Environment variable override applied | `"env var override applied" var="BLOGFLOW_SERVER_PORT" field="server.port" old_value=8080 new_value=9090` |
| WARN | Unknown YAML field rejected | `"unknown field in config file" field="servr" line=12` |
| INFO | Config loaded successfully at startup | `"config loaded" source="/data/config/site.yaml" duration_ms=4 fields_overridden=2` |
| INFO | Config loaded from embedded defaults | `"config loaded from embedded defaults" source="embed:defaults/config/defaults.yaml"` |
| INFO | Config reloaded successfully | `"config reloaded" source="/data/config/site.yaml" changed_fields=["site.title","cache.ttl"]` |
| INFO | Reload failed, keeping previous config | `"config reload failed, keeping previous config" error="validation failed: 1 error"` |
| DEBUG | Env var scan results | `"env var scan complete" vars_found=3 vars_applied=3` |
| DEBUG | YAML parse timing | `"yaml parsed" duration_us=1200 file_size_bytes=4096` |

### 7.2 Metrics & Dashboards

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `blogflow_config_load_duration_seconds` | Histogram | `source` (`disk`, `embedded`) | Time to load, parse, and validate config |
| `blogflow_config_reload_total` | Counter | `status` (`success`, `failure`) | Config reload attempts and outcomes |
| `blogflow_config_validation_errors_total` | Counter | `field` | Validation errors by field name |
| `blogflow_config_env_overrides_total` | Counter | `field` | Number of env var overrides applied at load time |
| `blogflow_config_secret_rejected_total` | Counter | `pattern` | Secret patterns detected and rejected in YAML |
| `blogflow_config_file_size_bytes` | Gauge | `source` (`disk`, `embedded`) | Size of the loaded config file |

**Dashboard: Config System Health**
- Reload success/failure rate (time series)
- Config load duration histogram
- Env var override count (bar — shows operational drift from YAML baseline)
- Secret rejection events (security signal — should always be 0)
- Validation error breakdown by field

### 7.3 Distributed Tracing

The config system creates spans for load and reload operations. These are rarely on the hot path (startup and reload only) but provide useful diagnostic data.

| Span Name | Attributes | Notes |
|-----------|------------|-------|
| `config.load` | `config.source`, `config.file_size`, `config.env_overrides_count` | Created during `Load()` |
| `config.reload` | `config.source`, `config.status`, `config.changed_fields` | Created during `Reload()` |
| `config.validate` | `config.errors_count`, `config.duration_us` | Child span of load/reload |

Config spans are children of the startup trace (for `Load`) or the fsnotify event trace (for `Reload`). `Get()` does not create spans — it is an atomic pointer load.

### 7.4 Alerting Rules & Escalation

| Alert Name | Condition | Severity | Response |
|------------|-----------|----------|----------|
| `ConfigReloadFailure` | `blogflow_config_reload_total{status="failure"}` > 0 in 5 min | 🟠 High | Check config file validity; review recent git commits to config repo; validate YAML syntax |
| `ConfigSecretInYAML` | `blogflow_config_secret_rejected_total` > 0 | 🔴 Critical | Secret committed to config file — rotate the secret immediately; remove from git history; review config repo access |
| `ConfigLoadSlowStartup` | `blogflow_config_load_duration_seconds` p99 > 200ms | 🟡 Warning | Check disk I/O; verify config volume mount; check for NFS latency |
| `ConfigReloadRateAnomaly` | `rate(blogflow_config_reload_total[5m])` > 5/sec sustained for 5 min | 🟡 Warning | Investigate fsnotify watcher; possible config file write loop |

---

## 8 · Rollout & Risk

### 8.1 Rollout Strategy

The config system is a library component that ships as part of the BlogFlow binary. It is the second component in the dependency chain (after the overlay FS).

1. **Phase 1 — Embedded defaults**: Create `defaults/config/defaults.yaml` with sensible defaults. Implement the `Config` struct and `Validate()` function.
2. **Phase 2 — Loader**: Implement `NewLoader`, `Load()`, `Get()` with 2-layer overlay FS integration. Binary works with zero external config.
3. **Phase 3 — Env overrides**: Implement `BLOGFLOW_*` env var scanning and override logic. Secret-only variables supported.
4. **Phase 4 — Reload**: Implement `Reload()`, `OnChange()`, and integration with fsnotify watcher.
5. **Phase 5 — Integration**: Wire config into server, theme engine, content pipeline, and gitops. Each integration is a separate PR.

### 8.2 Rollback Plan

- **Phase 1–3 (library only)**: Revert the merge commit. No downstream impact.
- **Phase 4–5 (integrated)**: If the config system introduces a regression, consumers can be reverted to hardcoded defaults. Each consumer integration is a separate PR, enabling surgical rollback.
- **Rollback time**: 30 min (revert PR + CI build). A build tag `blogflow_static_config` compiles consumers with hardcoded defaults as a fallback.

### 8.3 Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|:----------:|:------:|------------|
| Secret pattern regex has false positives (rejects valid config) | Low | Medium | Patterns target well-known formats (ghp_, AKIA, PEM headers); false positive = operator adds exception; startup log shows which pattern matched |
| YAML field mapping drift (struct tags don't match YAML keys) | Low | High | Integration tests with full YAML → struct → YAML round-trip; CI enforces test coverage |
| Env var naming collision with other tools | Low | Low | `BLOGFLOW_` prefix provides namespace isolation |
| Reload race with HTTP request processing | Medium | Medium | Atomic pointer swap via `atomic.Pointer[Config]`; `Get()` is lock-free; consumers see consistent config snapshot |
| Config file not found at expected path | Medium | Low | Falls back to embedded defaults; logged at INFO level; zero-config is a feature, not an error |
| `yaml.v3` parsing vulnerability | Low | High | Pin dependency version; dependabot alerts enabled; size limit prevents most payload-based exploits |

### 8.4 Dependencies & Sequencing

```mermaid
graph LR
    OFS["Overlay FS<br/>(must exist first)"] --> CFG["<strong>Config System</strong><br/>(this component)"]
    CFG --> Theme["Theme Engine"]
    CFG --> Content["Content Pipeline"]
    CFG --> Server["HTTP Server"]
    CFG --> GitOps["Git Ops"]

    style CFG fill:#f9f,stroke:#333,stroke-width:2px
```

**Deployment order:**
1. **Overlay FS** — must be implemented first (see [overlay-filesystem.md](overlay-filesystem.md))
2. **Config System** ← this component
3. **Theme engine** — depends on config for theme name/path
4. **Content pipeline** — depends on config for directory paths, pagination, date format
5. **Git ops** — depends on config for sync strategy, webhook settings
6. **HTTP server** — depends on config for port, timeouts + all of the above

### 8.5 Launch Checklist

- [ ] All acceptance criteria from §3.4 are met
- [ ] Unit tests pass with `go test -race ./internal/config/...`
- [ ] Benchmarks pass thresholds defined in §4.6
- [ ] `go vet` and `staticcheck` report no issues
- [ ] Integration test: binary starts with only embedded defaults (zero external config)
- [ ] Integration test: binary starts with disk `site.yaml` overriding defaults
- [ ] Integration test: env vars override both YAML and defaults
- [ ] Secret pattern detection tested for all known patterns (§2.7)
- [ ] Strict unmarshalling rejects unknown fields
- [ ] 1 MB file size limit enforced
- [ ] Reload preserves old config on validation failure
- [ ] Thread-safe: `Get()` under concurrent `Reload()` with race detector
- [ ] Security review completed (§5 and §6 reviewed by Security SME)
- [ ] Observability: metrics registered, log format verified
- [ ] Documentation: GoDoc comments on all exported types and functions
- [ ] No TODO or FIXME comments left in production code

> **Startup deadline**: `Load(ctx)` should be called with a 10-second context deadline. This prevents indefinite hangs on slow volumes (e.g., NFS). See overlay FS §4.2 for NFS latency considerations.

**Operational Readiness:**
- [ ] Runbook created for config failure scenarios (invalid YAML, secret in file, reload failure)

> **Note**: Runbook to be created during operational readiness at `docs/runbooks/config-system.md` with sections for: invalid YAML at startup, secret-in-YAML detection, reload loop, and volume unavailable.
- [ ] Alert rules deployed and smoke-tested in staging
- [ ] Dashboard deployed and functional in staging
- [ ] Graceful degradation tested: config volume missing, only embedded defaults serve
- [ ] All open questions §9 resolved

---

## 9 · Open Questions & Decisions

| # | Question | Status | Resolution |
|---|----------|--------|------------|
| 1 | Should config support multiple YAML files (e.g., `site.yaml` + `theme.yaml`) or a single file? | ✅ Resolved | Single file (`site.yaml`). Keeps the schema simple and avoids merge-order ambiguity. Theme-specific settings live under the `theme:` key. |
| 2 | Should env var overrides use flat naming (`BLOGFLOW_SERVER_PORT`) or nested (`BLOGFLOW_SERVER__PORT`)? | ✅ Resolved | Flat underscore naming: `BLOGFLOW_SERVER_PORT`. Double-underscore is error-prone and unusual. Flat works because the YAML schema has no ambiguous paths. |
| 3 | Should the config system support `.env` file loading? | ✅ Resolved | No. `.env` files are a development convenience handled by the shell or Docker Compose. The config system reads only `os.Getenv()`. Operators who want `.env` support use `direnv`, `dotenv`, or K8s `envFrom`. |
| 4 | Should `Reload()` diff the old and new config and only notify subscribers of changed sections? | ✅ Resolved | Yes — `Reload()` computes a changed-fields diff (field paths only, no values) and includes it in the INFO log and reload trace span. All subscribers still receive the full new Config (granular per-section notification deferred). See §7.1 for the diff algorithm. |
| 5 | Should the config system validate that `content.posts_dir` and related paths actually exist at startup? | ✅ Resolved | No. The config system validates the config schema. Directory existence is validated by the content pipeline when it initializes. Separation of concerns — config validates structure, consumers validate runtime preconditions. |
| 6 | Should `defaults.yaml` be a separate file in `embed.FS` or should defaults be defined as Go struct literal zero-values? | ✅ Resolved | Separate YAML file in `defaults/config/defaults.yaml`. This keeps defaults human-readable, inspectable, and consistent with the overlay FS architecture. Go struct zero-values are not meaningful defaults (e.g., `int` zero is not a valid port). |

---

## 10 · References

- **Requirements**: REQ-CFG-001 through REQ-CFG-009 (defined in §1.4)
- **ADRs**: See `docs/engineering/adr/` for BlogFlow ADRs
- **Depends on**: [Overlay Filesystem Design](overlay-filesystem.md) — config reads through the 2-layer overlay (config + defaults)
- **Architecture context**: [Copilot Instructions](../../../.github/copilot-instructions.md) — project overview and conventions
- **Go stdlib references**:
  - [`embed` package](https://pkg.go.dev/embed) — compiled-in `defaults.yaml`
  - [`os.Getenv`](https://pkg.go.dev/os#Getenv) — environment variable reads
  - [`sync/atomic`](https://pkg.go.dev/sync/atomic) — lock-free `Get()` via `atomic.Pointer`
  - [`log/slog`](https://pkg.go.dev/log/slog) — structured logging
- **External dependencies**:
  - [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML parsing with strict mode
- **Related design docs**: [Overlay Filesystem](overlay-filesystem.md) (approved), Theme Engine (planned), Content Pipeline (planned)
- **Project instructions**: [Copilot Instructions](../../../.github/copilot-instructions.md)
