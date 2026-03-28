# BlogFlow Vision

> Git is the CMS. Everything else is optional.

## What BlogFlow Is

BlogFlow is a compact Go blog engine where **git is the only content management interface**. You write markdown, push to a repo, and BlogFlow serves your blog. No database. No admin panel. No build step.

One binary. One container. Markdown in → blog out.

BlogFlow is for **developers and operators** — people who already live in git, terminals, and YAML. If your workflow is `vim → git commit → git push`, BlogFlow is your blog engine.

## What BlogFlow Is Not

- **Not a CMS.** There is no admin panel, no WYSIWYG editor, no user management. Git is the interface.
- **Not a static site generator.** BlogFlow is a live server with runtime features — config reload, content sync, health endpoints, Prometheus metrics. It doesn't produce static HTML files.
- **Not a framework.** There is no plugin system, no extension API, no hooks. If you need a plugin ecosystem, use WordPress or Ghost.
- **Not for non-technical users.** BlogFlow assumes you're comfortable with git, command-line tools, YAML configuration, and container deployments. We don't build GUIs.

## Design Principles

These principles are the filter for every feature request, PR, and design decision. When in doubt, apply them.

### 1. Git is the interface

All content, configuration, and theme changes flow through git. There is no other write path. This means:
- Content lives in a git repo (yours, not ours)
- Configuration is a YAML file committed alongside content
- Theme overrides are files in the repo
- Deployments are triggered by git push

We will never build an alternative content input mechanism.

### 2. Batteries included, not batteries required

BlogFlow ships with sensible defaults baked into the binary — a default theme, default templates, default CSS, a working configuration. You can run BlogFlow with zero configuration and get a working blog.

But every default is overridable. The overlay filesystem lets you replace any file — one template, one CSS rule, or the entire theme — without forking the engine.

### 3. One binary, one concern

BlogFlow serves blogs. It is not a wiki, not a forum, not a social network, not a portfolio builder. Feature requests that pull BlogFlow toward being a general-purpose web application are rejected.

The binary should stay small. The container should stay minimal. The attack surface should stay tiny.

### 4. Observable by default

Production software needs observability. BlogFlow ships with:
- Prometheus metrics (RED + overlay FS + Go runtime)
- OpenTelemetry tracing (opt-in, zero overhead when disabled)
- Structured logging with trace correlation
- Health and readiness endpoints with content awareness

Observability is not an afterthought or a plugin — it's built in.

### 5. Secure by default

The default posture is locked down:
- Distroless container, nonroot (UID 65532), read-only root filesystem
- Content Security Policy, HSTS, Permissions-Policy headers
- No shell, no package manager, no curl in the container
- Secrets read from files, not environment variables (operator's choice)
- HMAC-SHA256 webhook validation with rate limiting

Security features are always on. Loosening security is the operator's explicit choice.

### 6. Cloud-native, not cloud-dependent

BlogFlow runs anywhere containers run — Docker, Kubernetes, Podman, bare metal. It integrates with cloud services (Azure Monitor, Grafana Cloud, any OTLP endpoint) but never depends on them.

No vendor-specific SDKs. No cloud provider lock-in. Standard protocols only (OTLP, Prometheus, HTTP, git).

### 7. No client-side framework

The default theme uses vanilla HTML and CSS. No React, no Vue, no build tools for the frontend.

Small, purposeful JavaScript is acceptable for genuine UX improvements (search, code-copy buttons) — but the blog must be fully functional with JavaScript disabled.

## Feature Evaluation Rubric

When evaluating a feature request, ask these five questions:

| Question | If the answer is... | Then... |
|----------|---------------------|---------|
| Does it serve developers/operators? | No → non-technical users | Reject |
| Does it keep git as the single source of truth? | No → requires a database or API | Reject |
| Does it add operational complexity? | Yes, and it's not opt-in | Redesign as opt-in or reject |
| Can it work without a database? | No | Reject |
| Does it require an admin UI? | Yes | Reject |

A feature that passes all five questions is a candidate. A feature that fails any one is not BlogFlow's responsibility — it belongs in a different tool.

## What We Will Never Build

These are permanent decisions, not items deferred to a future roadmap:

- **Database-backed content storage** — git is the database
- **Admin panel or dashboard** — git is the admin panel
- **Plugin or extension system** — fork the engine or use the overlay FS
- **User authentication for readers** — blogs are public; auth is for operators/preview only
- **Built-in comment system** — use Giscus, utterances, or Disqus
- **WYSIWYG or rich-text editor** — write markdown in your editor of choice
- **Multi-tenancy** — one BlogFlow instance serves one blog
- **E-commerce or membership features** — BlogFlow serves content, not transactions

## What We Will Invest In

These are the themes that guide our roadmap:

- **Content workflow** — draft preview, multi-branch promotion, content validation CI
- **Developer experience** — content repo templates, local preview, quick-start tooling
- **Observability** — deeper tracing, custom dashboards, SLO tooling
- **Deployment flexibility** — sync strategies, easier K8s setup, edge deployment
- **Performance** — rendering speed, container startup, memory efficiency
- **Theme ecosystem** — more default themes, better override ergonomics

## Living Document

This vision is reviewed when the project reaches significant milestones. It evolves slowly and deliberately — changes require the same scrutiny as architectural decisions.

If a proposed change to this document would fundamentally alter BlogFlow's identity (e.g., "add a database"), the answer is to build a different project, not to change this one.
