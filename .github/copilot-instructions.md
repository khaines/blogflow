# Copilot Instructions — BlogFlow

## Project Overview

BlogFlow is a compact, highly efficient blog engine written in Go. All configuration, content, themes, and templates are driven through a gitflow process. The engine binary ships with sensible defaults (theme, CSS, templates, sample pages) compiled in via `embed.FS` — users only need to provide markdown files to have a working blog. For teams, content and theme repos are managed separately with full gitflow branching (feature → develop → release → main).

## Architecture Summary

- **Single Go binary** (< 15 MB container) built on `gcr.io/distroless/static-debian12:nonroot`
- **Overlay filesystem** (`io/fs.FS`) — external disk files override embedded defaults
- **Content pipeline**: goldmark (Markdown) → Go `html/template` → cached HTML
- **Git operations**: go-git (pure Go, no external binary) with SSH/PAT/GitHub App auth
- **Content sync strategies**: webhook (push), git-sync sidecar (pull/K8s), fsnotify (local dev)
- **Security-first**: distroless, rootless (UID 65532), read-only root FS, HMAC-SHA256 webhooks, no secrets in images

## Repository Structure

### Engine source — `cmd/` and `internal/`

```
cmd/blogflow/          — CLI entry point
internal/server/       — HTTP server, routes, middleware, handlers
internal/content/      — Content scanning, parsing, rendering, indexing, caching
internal/theme/        — Theme loading, overlay FS, template engine
internal/config/       — Configuration system (YAML + embedded defaults)
internal/gitops/       — go-git operations, webhook handler, file watcher, sync strategies
```

### Embedded defaults — `defaults/`

```
defaults/templates/    — Default Go HTML templates (base, post, list, page, 404)
defaults/static/       — Default CSS, JS, and images
defaults/config/       — Default site configuration (defaults.yaml)
```

### Developer experience — `.github/` and `.claude/`

```
.github/agents/        — GitHub Copilot custom agent wrappers
.github/skills/        — AI-assisted workflow skills (implement, review, design-doc)
.github/workflows/     — CI/CD pipelines
.github/ISSUE_TEMPLATE/— Bug, feature, ADR, spike templates
.claude/agents/        — Claude Code agent wrappers (mirrors .github/agents/)
```

### Documentation — `docs/`

```
docs/persona/agents/   — Canonical agent specifications
docs/engineering/      — Design documents, ADRs
```

## Persona System

BlogFlow uses a research-backed agent system for AI-assisted development. Each agent has:

- A canonical spec in `docs/persona/agents/`
- Platform-specific wrappers in `.github/agents/` (Copilot) and `.claude/agents/` (Claude)

Wrappers are lightweight and point back to their canonical spec. If a wrapper and its canonical spec drift, the canonical spec wins.

### Agent Roster

| Agent | Domain |
|---|---|
| Cloud-Native Distributed Systems Architect | System architecture, overlay FS design, content pipeline |
| Cloud-Native Systems Engineer | Go services, go-git, goldmark, HTTP server |
| Cloud-Native Front-End Engineer | Default theme, templates, CSS, responsive design, accessibility |
| Cloud-Native Site Reliability Engineer | Container health, webhook reliability, cache SLOs |
| Cloud-Native Security SME | Distroless hardening, HMAC webhooks, deploy keys, secrets |
| Technical Writer | Content authoring docs, theme guide, API reference, gitflow workflow |
| Product Manager | Feature roadmap, content workflow UX, user experience |
| Program Manager | Phase tracking, cross-repo coordination, execution governance |
| Solutions Engineer | Quick-start experience, deployment guides, integration patterns |
| Privacy & Compliance Lead | GDPR for blog content, data retention, cookie compliance |

## Conventions

### Document Format

- All docs are Markdown with **Mermaid diagrams** for architecture and flow visualization
- Feature comparisons use tables with consistent structure
- Code examples include full file paths

### Content Format (Blog Posts)

Blog posts use YAML front matter + Markdown body:

```yaml
---
title: "Post Title"
slug: "post-title"
date: 2026-03-22
tags: ["go", "architecture"]
draft: false
---

Markdown content here...
```

### Theme Format

Themes are directories with `theme.yaml` metadata + `templates/` + `static/`:

```
theme-name/
├── theme.yaml
├── templates/
│   ├── base.html
│   ├── post.html
│   ├── list.html
│   └── partials/
└── static/
    ├── css/
    └── js/
```

### Key Architecture Patterns

- **Overlay FS**: Theme → Content → Config → Embedded defaults (first match wins)
- **Immutable core**: Binary works standalone with zero external files
- **Progressive customization**: Level 0 (just markdown) → Level 1 (add config) → Level 2 (custom theme dir) → Level 3 (full gitflow repos)
- **Webhook-driven rebuild**: Push → HMAC validate → go-git pull → re-index → serve
- **Environment promotion**: develop → staging → main (no branch-per-environment)

### Security Principles

- Distroless container, rootless (UID 65532), read-only root filesystem
- No secrets in images — runtime injection via env vars or K8s Secrets
- HMAC-SHA256 webhook signature validation with constant-time comparison
- SSH deploy keys (read-only, per-repo) preferred over PATs
- All git operations via SSH or HTTPS — never plaintext
- Drop ALL Linux capabilities, no privilege escalation
- Pin container images by SHA256 digest in production

## Key Technology Choices

| Component | Technology | Why |
|-----------|-----------|-----|
| Language | Go | stdlib covers HTTP, templates, embed; fast compilation; single static binary |
| Markdown | goldmark | Go community standard; GFM, syntax highlighting, extensible |
| Templates | html/template | stdlib, XSS-safe, embed.FS integration |
| Git ops | go-git | Pure Go, no external binary needed, works in distroless |
| File watching | fsnotify | Cross-platform, Go community standard |
| HTTP router | net/http (or chi) | Minimal dependencies |
| Container base | distroless/static-debian12:nonroot | ~2 MB, no shell, nonroot |
