# BlogFlow

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A compact, efficient blog engine written in Go. Ship a single binary with sensible
defaults — just add markdown.

BlogFlow embeds its entire default theme, templates, CSS, and configuration via
`embed.FS`. Content, config, and themes are git-driven: manage everything through
branches and pull requests, or just drop markdown files in a directory.

## Quick Start

```bash
# 1. Create a content directory with a post
mkdir -p content/posts

# 2. Write a markdown post
cat > content/posts/hello.md << 'EOF'
---
title: "Hello, World!"
date: 2025-01-15
tags: ["intro"]
---

Welcome to my blog, powered by BlogFlow.
EOF

# 3. Run BlogFlow
blogflow --content ./content
```

Open [http://localhost:8080](http://localhost:8080) — that's it.

## Features

- **Zero-config start** — embedded defaults mean the binary works standalone
- **Overlay filesystem** — external files override embedded defaults (`io/fs.FS`)
- **Goldmark** — GitHub Flavored Markdown with syntax highlighting (CSS classes for Chroma — requires theme CSS), tables, task lists, footnotes
- **Secure by default** — distroless container, rootless (UID 65532), read-only root FS
- **< 25 MB container** — single static binary on `gcr.io/distroless/static-debian12:nonroot`
- **Git-driven content** — git-sync sidecar or fsnotify for live reload
- **HMAC-SHA256 webhooks** — constant-time signature validation, branch filtering, rate limiting
- **Atom/RSS feeds** — auto-generated with configurable item count
- **Rendered content cache** — in-memory LRU with configurable TTL

## Configuration

BlogFlow loads configuration in three layers (highest priority first):

1. **Environment variables** (`BLOGFLOW_*`)
2. **`site.yaml`** in your config directory
3. **Embedded defaults**

### Example `site.yaml`

```yaml
site:
  title: "My Blog"
  description: "Thoughts on code and craft"
  base_url: "https://blog.example.com"
  language: "en"
  author:
    name: "Jane Doe"
    email: "jane@example.com"

content:
  posts_dir: "posts"
  pages_dir: "pages"
  posts_per_page: 10
  date_format: "January 2, 2006"
  summary_length: 200

server:
  port: 8080
  read_timeout: "5s"
  write_timeout: "10s"
  idle_timeout: "120s"

cache:
  enabled: true
  ttl: "1h"
  max_entries: 1000

feed:
  enabled: true
  type: "atom"
  items: 20
```

See [`examples/config/site.yaml`](examples/config/site.yaml) for a fully documented example.

### Environment Variable Overrides

| Variable | Description |
|---|---|
| `BLOGFLOW_SITE_TITLE` | Site title |
| `BLOGFLOW_SITE_DESCRIPTION` | Site description |
| `BLOGFLOW_SITE_BASE_URL` | Base URL (set to production HTTPS URL) |
| `BLOGFLOW_SERVER_PORT` | HTTP server port (1–65535) |
| `BLOGFLOW_SERVER_READ_TIMEOUT` | Read timeout (Go duration, e.g. `5s`) |
| `BLOGFLOW_SERVER_WRITE_TIMEOUT` | Write timeout |
| `BLOGFLOW_SERVER_IDLE_TIMEOUT` | Idle timeout |
| `BLOGFLOW_CACHE_ENABLED` | Enable/disable cache (`true`/`false`) |
| `BLOGFLOW_SYNC_STRATEGY` | Sync strategy: `watch`, `webhook`, `sidecar` |
| `BLOGFLOW_WEBHOOK_SECRET` | Webhook HMAC secret (≥ 32 bytes, **never in YAML**) |
| `BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT` | Webhook rate limit (1–100 req/min) |
| `BLOGFLOW_FEED_TYPE` | Feed format: `atom` or `rss` |

## Progressive Customization

BlogFlow is designed for progressive disclosure — start simple, customize as needed.

| Level | What You Do | What Changes |
|---|---|---|
| **0 — Just Markdown** | Drop `.md` files in `content/posts/` | Embedded theme, default config |
| **1 — Add Config** | Create `site.yaml` with your site title, URL, author | Personalized metadata, same theme |
| **2 — Custom Theme** | Add a `theme/` directory with templates and CSS | Your look and feel, git-managed |
| **3 — Full GitFlow** | Separate content and theme repos, webhook sync, CI/CD | Team workflow with PRs to `main` |

## CLI Flags

```
blogflow [flags]

Flags:
  --content <path>    Path to content directory
  --theme <path>      Path to custom theme directory
  --config <path>     Path to site.yaml config file
  --dev               Enable development mode (verbose logging, no cache)
  --port <number>     HTTP server port (overrides config)
```

## Docker

### Build

```bash
docker build -t blogflow .
```

### Run

```bash
# Minimal — use embedded defaults
docker run -p 8080:8080 blogflow

# With external content
docker run -p 8080:8080 \
  -v ./content:/data/content:ro \
  blogflow --content /data/content

# With config overrides
docker run -p 8080:8080 \
  -e BLOGFLOW_SITE_TITLE="My Blog" \
  -e BLOGFLOW_SITE_BASE_URL="https://blog.example.com" \
  blogflow
```

The image is built on `gcr.io/distroless/static-debian12:nonroot` — no shell,
no package manager, no attack surface. Runs as `nonroot:nonroot` (UID 65532).

## Development

```bash
make build    # Compile binary to bin/blogflow
make test     # Run tests with race detector
make lint     # Run golangci-lint
make fmt      # Format with gofumpt
make docker   # Build Docker image
make run      # Build and run locally (dev mode)
make dev      # Build and run with live reload
make clean    # Remove build artifacts
make help     # Show all targets
```

## Architecture

```
cmd/blogflow/          CLI entry point
internal/server/       HTTP server, routes, middleware, handlers
internal/content/      Content scanning, front matter parsing, markdown rendering
internal/theme/        Theme loading, overlay FS, template engine
internal/config/       Configuration system (YAML + env vars + embedded defaults)
internal/overlayfs/    Overlay filesystem (io/fs.FS) implementation
defaults/              Embedded defaults (templates, CSS, images, config)
```

**Overlay FS resolution order** (first match wins):

```
External theme → External content → External config → Embedded defaults
```

**Content pipeline**:

```
Markdown files → YAML front matter + goldmark → Go html/template → Cached HTML
```

Design documents and ADRs are in [`docs/engineering/design/`](docs/engineering/design/).

## Health Checks

BlogFlow exposes two health endpoints:

| Endpoint | Purpose |
|---|---|
| `/healthz` | Liveness probe — returns `200 OK` if the process is running |
| `/readyz` | Readiness probe — returns `200 OK` once the server has finished initialization (atomic gate) |

Use these with Kubernetes liveness/readiness probes or any load-balancer health check.

## Logging

BlogFlow uses Go's structured `slog` logger. Each request logs:

| Field | Description |
|---|---|
| `method` | HTTP method |
| `path` | Request path |
| `status` | Response status code |
| `duration` | Request handling time |
| `remote` | Client address |

Use `--dev` to set the log level to debug. In production, logs are emitted as
structured JSON at info level.

## Content Format

Blog posts use YAML front matter:

```yaml
---
title: "Post Title"
slug: "post-title"
date: 2025-01-15
tags: ["go", "architecture"]
description: "A brief summary for feeds and SEO"
draft: false
---

Markdown content here...
```

Supported front matter fields: `title`, `slug`, `date`, `updated`, `draft`,
`tags`, `categories`, `author`, `description`, `template`, `image`.

## Deployment

BlogFlow supports multiple deployment patterns — from local development to
production Kubernetes clusters.

- **[Deployment Guide](docs/deployment-guide.md)** — full walkthrough of all patterns (watch, sidecar, webhook, Docker)
- **[K8s Sidecar Manifests](examples/k8s/sidecar/)** — production-ready Kubernetes manifests for the git-sync sidecar pattern
- **[K8s Webhook Manifests](examples/k8s/webhook/)** — production-ready Kubernetes manifests for the webhook pattern
- **[Helm Chart](deploy/helm/blogflow/)** — deploy with `helm install` using any sync strategy

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make changes with tests (`make test`)
4. Run linters (`make lint`)
5. Submit a pull request to `main`

Trunk-based development: `main` is always deployable. All changes go through
feature branches and pull requests.

## License

[Apache License 2.0](LICENSE)
