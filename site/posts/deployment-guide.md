---
title: "Deployment Guide"
slug: deployment-guide
date: 2025-07-18
tags: ["guide", "deployment", "docker", "kubernetes"]
description: "How to deploy BlogFlow — local development, Docker production, and Kubernetes with Helm."
---

# Deployment Guide

BlogFlow supports four deployment patterns. All use the same binary and container image — only the `sync.strategy` config changes.

## Deployment Patterns

| Pattern | Strategy | Trigger | Latency | Best For |
|---------|----------|---------|---------|----------|
| Local dev | `watch` | fsnotify | < 1 s | Development |
| K8s sidecar | `sidecar` | git-sync | ≤ 60 s | HA Kubernetes |
| K8s webhook | `webhook` | GitHub push | Instant | K8s single-node |
| Docker prod | `webhook` | GitHub push | Instant | Docker/VM |

## Pattern 1: Local Development

```bash
make dev
# or:
./bin/blogflow --dev --content ./my-content
```

The `watch` strategy uses fsnotify to detect file changes with sub-second latency. Files are debounced (500 ms) to avoid duplicate reloads during saves.

## Pattern 2: Docker Production

```yaml
# docker-compose.yml
services:
  blogflow:
    image: ghcr.io/khaines/blogflow:latest
    ports:
      - "8080:8080"
    environment:
      BLOGFLOW_SYNC_REPO: "https://github.com/you/content.git"
      BLOGFLOW_SITE_TITLE: "My Blog"
      BLOGFLOW_SITE_BASE_URL: "https://myblog.example.com"
```

For webhook-based updates, add the webhook secret and expose the webhook path:

```yaml
    environment:
      BLOGFLOW_SYNC_STRATEGY: "webhook"
      BLOGFLOW_WEBHOOK_SECRET_FILE: "/run/secrets/webhook_secret"
```

## Pattern 3: Kubernetes with Helm

```bash
helm install blogflow deploy/helm/blogflow/ \
  --set sync.strategy=sidecar \
  --set sync.repo=https://github.com/you/content.git
```

The Helm chart supports all sync strategies, horizontal pod autoscaling, PodDisruptionBudgets, and Ingress configuration.

### Key Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `sync.strategy` | `watch` | Content sync strategy |
| `sync.repo` | — | Git repository URL |
| `sync.branch` | `main` | Branch to track |
| `ingress.enabled` | `false` | Enable Ingress resource |
| `pdb.enabled` | `false` | Enable PodDisruptionBudget |

## Health Endpoints

| Endpoint | Purpose | When to Use |
|----------|---------|-------------|
| `GET /healthz` | Liveness probe | Always returns 200 |
| `GET /readyz` | Readiness probe | 200 when ready, 503 when not |
| `GET /readyz/content` | Content check | 200 when posts loaded |
| `GET /metrics` | Prometheus metrics | Scrape target |

## Environment Variable Reference

All configuration can be overridden via `BLOGFLOW_*` environment variables:

| Variable | Description |
|----------|-------------|
| `BLOGFLOW_SITE_TITLE` | Site title |
| `BLOGFLOW_SITE_BASE_URL` | Public URL |
| `BLOGFLOW_SITE_HOMEPAGE` | Homepage mode: `post_list` or `page:<slug>` |
| `BLOGFLOW_SERVER_PORT` | HTTP port (default: 8080) |
| `BLOGFLOW_SYNC_STRATEGY` | Sync strategy |
| `BLOGFLOW_SYNC_REPO` | Git repository URL |
| `BLOGFLOW_WEBHOOK_SECRET` | Webhook HMAC secret (≥ 32 bytes) |
