---
title: "BlogFlow — Git-Driven Blog Engine"
slug: landing
---

# BlogFlow

**The anti-WordPress blog engine.** Compact Go binary, git-driven content, distroless container. Just add markdown.

## Why BlogFlow?

- **~18 MB binary** — single static Go executable, no runtime dependencies
- **< 25 MB container** — distroless image, minimal attack surface
- **< 100 ms startup** — cold-start ready for serverless and edge deployments
- **Zero database** — content lives in git; deploy with `docker run`

## Quick Start

```bash
docker run -p 8080:8080 ghcr.io/khaines/blogflow:latest
```

Open [http://localhost:8080](http://localhost:8080) and you have a blog.

### With Your Own Content

```bash
docker run -p 8080:8080 \
  -e BLOGFLOW_SYNC_REPO=https://github.com/you/your-content.git \
  ghcr.io/khaines/blogflow:latest
```

BlogFlow clones your repo at startup and serves markdown as HTML.

## Features

### Git-Driven Content
Write posts and pages in markdown. Push to git. BlogFlow picks up changes automatically via four sync strategies: file watch, git-sync sidecar, webhook, or polling.

### Overlay Filesystem
Four-layer filesystem: theme → content → config → defaults. Override any file at any layer without forking the theme.

### Distroless Container
Production image runs as non-root on a read-only filesystem in a distroless container. No shell, no package manager, minimal CVE surface.

### Four Sync Strategies
- **watch** — fsnotify for local development (< 1 s latency)
- **sidecar** — Kubernetes git-sync sidecar (≤ 60 s poll)
- **webhook** — GitHub push webhooks (instant)
- **poll** — periodic git pull (configurable interval)

### Observability
Prometheus metrics at `/metrics`, structured JSON logging, OpenTelemetry traces, health endpoints at `/healthz` and `/readyz`.

### Runtime Config Reload
Change `site.yaml` and BlogFlow picks up the new config without restart. No downtime for title, description, or theme changes.

## Documentation

- [Getting Started](/posts/getting-started) — first blog in 5 minutes
- [Content Authoring](/posts/content-authoring) — front matter, pages, media
- [Deployment Guide](/posts/deployment-guide) — Docker, Kubernetes, Helm
