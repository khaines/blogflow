# Changelog

All notable changes to BlogFlow will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-23

### Added

#### Core Engine
- Overlay filesystem for layering user content over embedded defaults
- YAML/TOML configuration system with environment variable overrides
- Markdown renderer with front matter parser
- Content scanner with in-memory index
- Theme engine with Go `html/template` rendering
- HTTP server with graceful shutdown
- Wired component initialization in `cmd/blogflow/main.go`
- Embedded default assets (templates, static files)

#### Content Features
- Atom/RSS feed and sitemap XML generators
- Syntax highlighting via Chroma
- Rendered HTML cache with invalidation on content reload
- HTTP caching headers for feeds and sitemap
- Content route handlers (posts, pages, index)

#### Git Integration
- fsnotify-based file watcher with recursive subdirectory support
- HMAC-SHA256 webhook handler with configurable body-size limits
- Git-sync sidecar strategy for Kubernetes deployments
- go-git clone and pull operations with authentication support
- Sync strategy selector (webhook, poll, sidecar)

#### Observability
- Prometheus `/metrics` endpoint
- Structured logging in config loader and content scanner
- Request-ID middleware with proxy-aware client IP detection

#### Security
- Content-Security-Policy headers
- HSTS and Permissions-Policy headers
- Proxy-aware client IP resolution
- Webhook body-size limits via `MaxBytesReader`
- Rate limiting with LRU cache and TTL-based eviction

#### Operations
- Healthcheck CLI subcommand for distroless containers
- Runtime configuration reloading
- Docker Compose for local development
- Helm chart for Kubernetes deployment
- Kubernetes example manifests for sidecar and webhook modes
- Distroless container image pinned by SHA256 digest
- Stripped binary debug symbols for smaller images

#### CI/CD
- Lint, test, and build workflows
- Docker image build with size-limit check
- Container smoke tests
- End-to-end test suite
- Trivy vulnerability scanning (medium+ severity gate)
- Publish workflow for container images on merge to main
- Content deploy workflow (example)

#### Documentation
- Comprehensive deployment guide
- Content authoring and theme development guides
- Container security guide
- Configuration system design document
- README with quick-start guide and sample content

[0.1.0]: https://github.com/khaines/blogflow/releases/tag/v0.1.0
