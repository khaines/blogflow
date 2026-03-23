# Changelog

## main / unreleased

## 0.1.0 / 2026-03-23

### BlogFlow

* [FEATURE] OverlayFS: Overlay filesystem for layering user content over embedded defaults. #4
* [FEATURE] Config: YAML configuration system with environment variable overrides. #10
* [FEATURE] Config: Runtime configuration reloading with `Reload()` and `OnChange()`. #101
* [FEATURE] Content: Goldmark markdown renderer with front matter parser, GFM, and footnotes. #12
* [FEATURE] Content: Chroma syntax highlighting for fenced code blocks. #12 #67
* [FEATURE] Content: Content scanner with in-memory index. #15
* [FEATURE] Content: Rendered HTML cache with invalidation on content reload. #35 #94
* [FEATURE] Content: Atom/RSS feed and sitemap XML generators. #30
* [FEATURE] Theme: Theme engine with Go `html/template` rendering. #16
* [FEATURE] Theme: Embedded default assets (templates, static files). #11
* [FEATURE] Server: HTTP server with graceful shutdown. #17
* [FEATURE] Server: Wired component initialization in `cmd/blogflow/main.go`. #31
* [FEATURE] Server: HTTP caching headers for feeds and sitemap. #89
* [FEATURE] Server: Prometheus `/metrics` endpoint. #79
* [FEATURE] Server: Request-ID middleware with proxy-aware client IP detection. #86
* [FEATURE] Handlers: Content route handlers (posts, pages, index) with pagination. #29
* [FEATURE] Handlers: Tag-based browsing with paginated tag listings. #29
* [FEATURE] GitOps: Sync strategy selector (webhook, poll, sidecar). #39
* [FEATURE] GitOps: go-git clone and pull operations with authentication support. #50 #40
* [FEATURE] GitOps: fsnotify-based file watcher with recursive subdirectory support. #49
* [FEATURE] GitOps: HMAC-SHA256 webhook handler with configurable body-size limits. #58 #84
* [FEATURE] GitOps: Git-sync sidecar strategy for Kubernetes deployments. #87
* [FEATURE] Security: Content-Security-Policy headers.
* [FEATURE] Security: HSTS and Permissions-Policy headers. #77
* [FEATURE] Security: X-Content-Type-Options, X-Frame-Options, and Referrer-Policy headers.
* [FEATURE] CLI: Healthcheck subcommand for distroless containers. #76
* [FEATURE] CLI: Version injection via ldflags and `--version` flag.
* [ENHANCEMENT] Config: Structured logging in config loader and content scanner. #102
* [ENHANCEMENT] Content: Link/URL sanitization blocking dangerous URI schemes in Markdown.
* [ENHANCEMENT] Server: Panic recovery middleware with stack-trace logging.
* [ENHANCEMENT] Security: Rate limiting with LRU cache and TTL-based eviction. #100
* [ENHANCEMENT] Security: Proxy-aware client IP resolution. #86
* [ENHANCEMENT] Security: Webhook body-size limits via `MaxBytesReader`. #84
* [ENHANCEMENT] Docker: Distroless container image pinned by SHA256 digest. #74
* [ENHANCEMENT] Docker: Stripped binary debug symbols for smaller images. #98
* [ENHANCEMENT] Docker: Docker Compose for local development. #37
* [FEATURE] Docs: Comprehensive deployment guide. #93
* [FEATURE] Docs: Content authoring and theme development guides. #51
* [FEATURE] Docs: Container security guide. #38
* [FEATURE] Docs: Configuration system design document. #2
* [FEATURE] Docs: README with quick-start guide and sample content. #36

### Helm Chart

* [FEATURE] Helm: Production-ready chart with sidecar and webhook strategy support. #92
* [FEATURE] Helm: Kubernetes example manifests for sidecar and webhook modes. #91
* [ENHANCEMENT] Helm: K8s hardening — secret files, startupProbe, sizeLimit, PDB. #99

### CI/CD

* [FEATURE] CI: Lint, test, and build workflows.
* [FEATURE] CI: Docker image build with size-limit check.
* [FEATURE] CI: Container smoke test verifying all endpoints.
* [FEATURE] CI: Docker Compose end-to-end test suite.
* [FEATURE] CI: Trivy vulnerability scanning (medium+ severity gate). #80
* [FEATURE] CI: Publish workflow for container images on merge to main. #96
* [FEATURE] CI: Content deploy workflow (example). #59 #97
* [FEATURE] CI: Kubernetes manifest linting with kubeconform.
* [FEATURE] CI: Helm chart linting and test template.
