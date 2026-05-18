# Changelog

## main / unreleased

## 0.4.1 / 2026-05-18

### BlogFlow

* [BUGFIX] cmd/blogflow: `defer` the sync-strategy context cancel function so the context is always released on early-exit paths (silences gosec G118). #207

### Dependencies

* [BUGFIX] Deps: Upgrade `go-git/v5` to v5.19.0 and `go-billy/v5` to v5.9.0 — addresses CVE-2026-44973 (go-billy path traversal), CVE-2026-45022 (go-git object parsing), and CVE-2026-44740 (go-billy symlink loops). #206
* [CHANGE] Deps: Bump `github.com/go-git/go-git/v5` from 5.18.0 to 5.19.0. #205
* [CHANGE] Deps: Bump the golang-x group with 2 updates. #204
* [CHANGE] Deps: Bump `github.com/fsnotify/fsnotify` from 1.9.0 to 1.10.1. #203
* [CHANGE] Deps: Bump `github.com/alecthomas/chroma/v2` from 2.23.1 to 2.24.1. #202
* [CHANGE] Deps: Bump `actions/setup-go` from 5 to 6. #201
* [CHANGE] Deps: Bump `azure/login` from 2 to 3. #200
* [CHANGE] Deps: Bump `docker/metadata-action` from 5 to 6. #199
* [CHANGE] Deps: Bump `aquasecurity/trivy-action` from 0.35.0 to 0.36.0. #198

## 0.4.0 / 2026-04-26

### BlogFlow

* [FEATURE] Handlers: Content analytics — `blogflow_content_views_total{type, slug}` Prometheus counter for content popularity tracking. #186
* [FEATURE] Handlers: OTel span attributes (`content.type`, `content.slug`, `content.title`, `content.tags`) on all content-serving requests. #186

### Documentation

* [ENHANCEMENT] Docs: Content analytics section in deployment guide with PromQL examples and span attribute reference. #186

### Deploy

* [BUGFIX] Deploy: Stop app metrics going to Log Analytics via App Insights — removes OTel Collector sidecar, uses ACA managed OTel agent for traces only. #184
* [ENHANCEMENT] Deploy: Provision Azure Monitor workspace for future Prometheus metrics (Phase 2). #184
* [ENHANCEMENT] Deploy: Parameterize Log Analytics retention (`logRetentionDays`, default 30). #184
* [ENHANCEMENT] Deploy: Add custom domain and TLS certificate parameters to deploy workflow. #184
* [ENHANCEMENT] Deploy: Complete SETUP.md rewrite with Phase 1/2 architecture, rollback procedure, and verification steps. #184

### Dependencies

* [CHANGE] Deps: Bump github.com/yuin/goldmark from 1.7.17 to 1.8.2. #196
* [CHANGE] Deps: Bump the golang-x group with 2 updates. #195
* [CHANGE] Deps: Bump azure/setup-helm from 4 to 5. #194
* [CHANGE] Deps: Bump docker/setup-buildx-action from 3 to 4. #193
* [CHANGE] Deps: Bump docker/login-action from 3 to 4. #192
* [CHANGE] Deps: Bump docker/build-push-action from 6 to 7. #191
* [CHANGE] Deps: Bump actions/checkout from 4 to 6. #190
* [CHANGE] Deps: Bump distroless/static-debian12 base image digest. #189
* [BUGFIX] Deps: Resolve 6 open code scanning alerts and enable Dependabot. #188
* [BUGFIX] Deps: Bump grpc to v1.79.3 and circl to v1.6.3 (security). #160

## 0.3.0 / 2026-03-27

### BlogFlow

* [FEATURE] Server: OpenTelemetry core SDK with opt-in HTTP tracing via `OTEL_TRACES_EXPORTER` env var. #151
* [FEATURE] Server: OTel metrics bridge — dual-export existing Prometheus metrics via OTLP. #152
* [FEATURE] Server: Trace ID and span ID automatically injected into slog log records. #151
* [FEATURE] Content: OTel spans on content scanner with posts/pages/errors attributes. #154
* [FEATURE] GitOps: OTel spans on git clone/pull operations with sanitized repo URL. #154
* [FEATURE] Theme: OTel span on template rendering with template name attribute. #154
* [FEATURE] Config: OTel span on config reload with success/failure tracking. #154
* [FEATURE] OverlayFS: OTel spans on all ContextOverlayFS operations with layer Resolution attributes. #153
* [BUGFIX] Server: Theme and config now reload alongside content on sync events (was content-only). #156

### Documentation

* [ENHANCEMENT] Docs: OpenTelemetry observability guide with docker-compose/K8s examples and provider reference. #155
* [ENHANCEMENT] Docs: Health & Readiness Endpoints section with operator decision table and K8s probe examples. #150
* [ENHANCEMENT] Docs: Mermaid diagram style guide with 7 classDef color classes. #147
* [ENHANCEMENT] Docs: Applied consistent Mermaid theme to user-facing diagrams. #148

## 0.2.1 / 2026-03-26

### BlogFlow

* [FEATURE] Server: Separate metrics listener port (`server.metrics_port`) for network isolation. #146
* [BUGFIX] CI: Update `codeql-action` from v3 to v4 and make SARIF upload non-blocking. #138

### Helm Chart

* [ENHANCEMENT] Helm: Added `service.metricsPort` for dedicated Prometheus scrape target. #146

### Documentation

* [ENHANCEMENT] Docs: Rewritten README with comprehensive v0.2.0 feature coverage. #139
* [ENHANCEMENT] Docs: Added CONTRIBUTING.md with development workflow, testing, AI agents, RFL process. #140
* [ENHANCEMENT] Docs: GitHub Pages landing page at blogflow.io. #142 #143 #144
* [BUGFIX] Docs: Corrected changelog format attribution to Prometheus/Cortex ecosystem. #141

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
