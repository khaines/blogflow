# Content Authoring Guide

> **Audience**: Blog authors and content editors  
> **Prerequisite**: BlogFlow binary installed or running via Docker  
> **Last Updated**: 2025-07-16

This guide covers everything you need to write, organize, and publish content
with BlogFlow. Start at [Writing Posts](#1-writing-posts) if you just want to
publish your first article; read through to [Configuration Reference](#7-configuration-reference)
when you need full control.

---

## 1. Writing Posts

### Front Matter Schema

Every post starts with a YAML front matter block delimited by `---`. BlogFlow
parses front matter up to **64 KB**.

```yaml
---
title: "Deploying to Kubernetes"
slug: "deploying-to-kubernetes"
date: 2025-06-20
updated: 2025-07-01
tags: ["kubernetes", "devops"]
categories: ["infrastructure"]
author: "Jane Doe"
description: "Step-by-step guide for deploying BlogFlow on K8s."
draft: false
template: "tutorial.html"
image: "https://example.com/hero.png"
reading_time: 8
---
```

| Field          | Type       | Required | Default              | Description                                       |
|----------------|------------|----------|----------------------|---------------------------------------------------|
| `title`        | `string`   | **Yes**  | —                    | Post title displayed in listings and feeds.        |
| `date`         | `time`     | **Yes**  | —                    | Publication date (`YYYY-MM-DD` or RFC 3339).       |
| `slug`         | `string`   | No       | filename stem        | URL path segment. Must be a plain filename—no `/`, `\`, or `..`. |
| `updated`      | `time`     | No       | —                    | Last-modified date for feeds and SEO.              |
| `draft`        | `bool`     | No       | `false`              | When `true`, post is hidden from listings.         |
| `tags`         | `[]string` | No       | `[]`                 | Tags for grouping and filtering.                   |
| `categories`   | `[]string` | No       | `[]`                 | Broader groupings than tags.                       |
| `author`       | `string`   | No       | `site.author.name`   | Post-level author override.                        |
| `description`  | `string`   | No       | auto-generated       | Summary for feeds and `<meta>` tags.               |
| `template`     | `string`   | No       | `post.html`          | Custom template filename (plain name, no path).    |
| `image`        | `string`   | No       | —                    | Featured image URL (`http` or `https` only).       |
| `reading_time` | `int`      | No       | auto-computed        | Estimated minutes; auto-calculated at ~200 wpm if omitted. |

**Validation rules**:

- `slug` and `template` must be plain filenames—no `/`, `\`, `..`, or null bytes.
- `image` must use `http` or `https` scheme.
- Duplicate slugs across posts are rejected at startup (fail-fast).

### Date Format

Dates in front matter accept:

- **Date only**: `2025-06-20` (interpreted as midnight UTC)
- **Full RFC 3339**: `2025-06-20T14:30:00Z`
- **With timezone**: `2025-06-20T14:30:00-07:00`

Display format is controlled by `content.date_format` in `site.yaml` (default:
`"January 2, 2006"`, following Go time layout conventions).

### Tags and Categories

```yaml
tags: ["go", "docker", "tutorial"]
categories: ["backend"]
```

- Tags appear in post metadata and generate `/tags/{tag}` listing pages.
- Categories are a broader grouping mechanism.
- Both are case-sensitive—use lowercase consistently.

### Draft Posts

Set `draft: true` to exclude a post from all listings, feeds, and tag pages.
Draft posts are skipped during content scanning and never served.

```yaml
---
title: "Work in Progress"
date: 2025-07-10
draft: true
---
```

### Custom Templates

Override the default `post.html` template for a specific post:

```yaml
---
title: "Photo Gallery"
date: 2025-07-01
template: "gallery.html"
---
```

The template must exist in your theme's `templates/` directory or in the
default templates.

### Markdown Features

BlogFlow uses [Goldmark](https://github.com/yuin/goldmark) with the following
extensions enabled:

| Feature             | Syntax                          | Notes                        |
|---------------------|---------------------------------|------------------------------|
| GFM tables          | `\| col \| col \|`             | With column alignment        |
| Task lists          | `- [x] Done`                   | Checkbox syntax              |
| Strikethrough       | `~~deleted~~`                   | GFM extension                |
| Autolinks           | `https://example.com`           | Bare URLs become links       |
| Footnotes           | `[^1]` / `[^1]: Note text`     | Rendered at page bottom      |
| Smart quotes        | `"text"` → "text"              | Typographer extension        |
| Em/en dashes        | `---` / `--`                    | Typographer extension        |
| Auto heading IDs    | `## My Section` → `id="my-section"` | For anchor links        |
| Fenced code blocks  | ` ```go `                       | CSS classes for Chroma highlighting |

**HTML handling**: Raw HTML in Markdown is **stripped by default** for XSS
safety. Content is pre-sanitized through the Goldmark pipeline.

### File Naming Conventions

- Use **kebab-case** for filenames: `deploying-to-kubernetes.md`
- Extension must be `.md` (only Markdown files are processed)
- The filename stem becomes the default `slug` if none is specified
- Files without valid front matter are silently skipped

---

## 2. Writing Pages

### Posts vs Pages

| Aspect     | Posts                              | Pages                             |
|------------|------------------------------------|------------------------------------|
| Location   | `posts/` directory                 | `pages/` directory                 |
| `date`     | **Required**                       | Optional                           |
| Ordering   | Sorted by date (newest first)      | No automatic ordering              |
| Listings   | Appear on homepage and tag pages   | Standalone; not in listings         |
| Template   | `post.html`                        | `page.html`                        |
| Feed       | Included in RSS/Atom               | Excluded from feeds                |

### Pages Directory

Pages live in the `pages/` directory (configurable via `content.pages_dir`):

```
content/
└── pages/
    ├── about.md
    └── contact.md
```

### Example: About Page

```markdown
---
title: "About"
slug: "about"
description: "Learn more about this blog and its author."
---

## About This Blog

Welcome! This blog covers topics in distributed systems, Go programming,
and cloud-native architecture.

## Contact

Reach me at author@example.com or on [GitHub](https://github.com/example).
```

### Example: Contact Page

```markdown
---
title: "Contact"
slug: "contact"
description: "Get in touch."
---

## Get in Touch

- **Email**: hello@example.com
- **GitHub**: [github.com/example](https://github.com/example)
- **Twitter**: [@example](https://twitter.com/example)
```

---

## 3. Content Directory Structure

```
content/
├── posts/                    # Blog posts (configurable: content.posts_dir)
│   ├── hello-world.md
│   ├── getting-started.md
│   └── deploying-to-k8s.md
├── pages/                    # Static pages (configurable: content.pages_dir)
│   ├── about.md
│   └── contact.md
└── media/                    # Images and assets (configurable: content.media_dir)
    └── images/
        ├── hero.png
        └── diagram.svg
```

**Directory names** (`posts`, `pages`, `media`) are configurable in `site.yaml`
but the defaults above are conventional.

**Content scanning**:

> ⚠️ **Fail-fast scanning**: A single malformed post (missing date, duplicate slug, invalid slug) aborts the entire content scan. Validate content locally with `blogflow --content ./content --dev` before pushing.

- Only `.md` files are processed.
- Posts require both `title` and `date` in front matter.
- Pages require only `title`.
- Draft posts (`draft: true`) are excluded from all outputs.
- Files without valid front matter are skipped.

**Content indexing**: Posts are indexed into several structures for fast lookups:

- **By date**: all posts sorted newest-first (drives homepage)
- **By slug**: O(1) lookup for individual post pages
- **By tag**: posts grouped per tag (drives `/tags/{tag}` pages)
- **By year**: posts grouped by publication year
- **Pages by slug**: O(1) lookup for static pages

---

## 4. Progressive Customization

BlogFlow follows a four-level progressive customization model. Start simple and
add complexity only when you need it.

### Level 0 — Just Markdown + Binary

The simplest setup. No configuration file needed.

```
my-blog/
└── content/
    └── posts/
        └── hello-world.md
```

```bash
blogflow --content ./content
```

BlogFlow uses embedded defaults for everything: templates, CSS, configuration.
The site title is "My Blog" and it runs on port 8080.

### Level 1 — Add site.yaml

Add a configuration file to customize site identity, server settings, and
content options.

```
my-blog/
├── site.yaml
└── content/
    ├── posts/
    │   └── hello-world.md
    └── pages/
        └── about.md
```

```yaml
# site.yaml
site:
  title: "My Tech Blog"
  description: "Thoughts on Go and cloud-native systems"
  base_url: "https://blog.example.com"
  author:
    name: "Jane Doe"
    email: "jane@example.com"
```

### Level 2 — Custom Theme Directory

Override default templates and CSS with your own theme.

```
my-blog/
├── site.yaml
├── content/
│   └── posts/
│       └── hello-world.md
└── theme/
    ├── theme.yaml
    ├── templates/
    │   ├── base.html
    │   └── partials/
    │       └── header.html
    └── static/
        └── css/
            └── main.css
```

```bash
blogflow --content ./content --theme ./theme
```

The overlay FS merges your theme with embedded defaults—override only the files
you want to change. Everything else falls through to the defaults.

### Level 3 — Separate Content and Theme Repos

For teams, keep content and theme in separate Git repositories. Use git-sync
sidecars or webhooks to deploy.

```
# Content repo (authors own this)
blog-content/
├── posts/
│   └── hello-world.md
├── pages/
│   └── about.md
└── media/
    └── images/

# Theme repo (designers own this)
blog-theme/
├── theme.yaml
├── templates/
│   └── ...
└── static/
    └── ...

# Deployment config
docker-compose.yml    # BlogFlow + git-sync sidecars
site.yaml             # Environment-specific config
```

This separation lets content authors and theme developers work independently
with different review cycles.

---

## 5. Git Workflow for Content

### Trunk-Based Development

BlogFlow content follows a trunk-based workflow:

1. **`main` is production** — merged content is live.
2. **Feature branches** for new posts or edits.
3. **Pull requests** for review before publishing.

```
main ─────●────────●────────●──── (live)
           \      /          \
            post/my-new-article   fix/typo-in-about
```

### Branch Naming Conventions

| Purpose         | Pattern                          | Example                          |
|-----------------|----------------------------------|----------------------------------|
| New post        | `post/<slug>`                    | `post/deploying-to-kubernetes`   |
| Edit post       | `edit/<slug>`                    | `edit/hello-world`               |
| New page        | `page/<slug>`                    | `page/contact`                   |
| Fix/typo        | `fix/<description>`              | `fix/typo-in-about`              |
| Theme change    | `theme/<description>`            | `theme/dark-mode`                |

### PR Review for Content

- Use PRs for all content changes, even single-post additions.
- Reviewers check for: front matter validity, broken image paths, tag
  consistency, and prose quality.
- Merge to `main` triggers content sync.

### How Content Syncs to the Blog

BlogFlow supports three sync strategies (configured via `sync.strategy`):

| Strategy    | How it works                                           | Best for              |
|-------------|-------------------------------------------------------|-----------------------|
| `watch`     | Filesystem watcher (fsnotify) detects local changes.   | Local development     |
| `webhook`   | GitHub webhook on push to `main` triggers a git pull.  | See warning below     |
| `sidecar`   | Kubernetes git-sync sidecar pulls content on a loop.   | Production (Kubernetes) |

**Webhook sync**:

> ⚠️ **Not yet implemented.** The webhook sync strategy is currently a stub. HMAC verification, git pull, and content reload are not functional. Use the `watch` strategy for local development. Webhook support is tracked in the project backlog.

When complete, the webhook strategy will accept a GitHub `push` event at a
configurable endpoint (default `/api/webhook`), verify the payload signature,
and trigger a content reload. Configuration is accepted in `site.yaml` under
`sync.webhook` but has no effect until the implementation is finished.

---

## 6. Media and Images

### Where to Put Images

Place images in the `media/` directory (configurable via `content.media_dir`):

```
content/
└── media/
    └── images/
        ├── hero.png
        ├── architecture-diagram.svg
        └── screenshot.jpg
```

### Referencing Images in Markdown

All static assets — including media — are served under the `/static/` URL
prefix by the HTTP server. Use that prefix when referencing images:

```markdown
![Architecture diagram](/static/media/images/architecture-diagram.svg)

![Screenshot of the dashboard](/static/media/images/screenshot.jpg "Dashboard")
```

### Image Path Resolution

Images are resolved through the overlay filesystem. The resolution order
(highest priority first):

1. **Theme layer** — theme-provided images
2. **Content layer** — your `media/` directory (most common)
3. **Config layer** — configuration-level assets
4. **Defaults layer** — embedded default assets (e.g., `favicon.svg`)

Files placed in `content/media/` are served at `/static/media/…` by the HTTP
server via the overlay FS.

---

## 7. Configuration Reference

### Configuration Priority

BlogFlow resolves configuration from three sources (highest priority first):

1. **Environment variables** (`BLOGFLOW_*` prefix)
2. **`site.yaml`** file
3. **Embedded defaults**

### Complete site.yaml Schema

```yaml
# ─── Site Identity ────────────────────────────────────────────
site:
  title: "My Blog"                   # string — site display name
  description: "A blog powered by BlogFlow"  # string — meta description
  base_url: "http://localhost:8080"  # string — canonical URL (http/https, non-empty host)
  language: "en"                     # string — BCP 47 language tag
  author:
    name: ""                         # string — default author name
    email: ""                        # string — default author email

# ─── Content Pipeline ────────────────────────────────────────
content:
  posts_dir: "posts"                 # string — relative path to posts directory
  pages_dir: "pages"                 # string — relative path to pages directory
  media_dir: "media"                 # string — relative path to media directory
  posts_per_page: 10                 # int    — posts per listing page (1–100)
  date_format: "January 2, 2006"    # string — Go time layout for display
  summary_length: 200               # int    — auto-summary character count (50–1000)

# ─── Theme ────────────────────────────────────────────────────
theme:
  name: "default"                    # string — theme name
  path: ""                           # string — path to custom theme (empty = embedded)

# ─── Server ───────────────────────────────────────────────────
server:
  port: 8080                         # int      — listen port (1–65535)
  read_timeout: "5s"                 # duration — HTTP read timeout
  write_timeout: "10s"               # duration — HTTP write timeout
  idle_timeout: "120s"               # duration — keep-alive idle timeout

# ─── Cache ────────────────────────────────────────────────────
cache:
  enabled: true                      # bool     — enable render cache
  ttl: "1h"                          # duration — cache entry lifetime (max 24h)
  max_entries: 1000                  # int      — max cached entries (0–100000)

# ─── Sync ─────────────────────────────────────────────────────
sync:
  strategy: "watch"                  # string — "watch", "webhook", or "sidecar"
  webhook:
    path: "/api/webhook"             # string — webhook endpoint path
    # secret: — NEVER set in YAML; use BLOGFLOW_WEBHOOK_SECRET env var
    allowed_events:                  # []string — accepted GitHub event types
      - "push"
    branch_filter: "main"            # string — only sync pushes to this branch
    rate_limit: 10                   # int    — max requests per minute (1–100)

# ─── Feed ─────────────────────────────────────────────────────
feed:
  enabled: true                      # bool   — generate feed
  type: "atom"                       # string — "atom" or "rss"
  items: 20                          # int    — max feed entries (1–100)
```

### Validation Rules

| Field                | Constraint                                    |
|----------------------|-----------------------------------------------|
| `server.port`        | 1–65535                                       |
| `server.*_timeout`   | Must be > 0                                   |
| `site.base_url`      | Valid HTTP/HTTPS URL with non-empty host       |
| `content.*_dir`      | Relative paths only; no `..` or absolute paths |
| `content.posts_per_page` | 1–100                                     |
| `content.summary_length` | 50–1000                                   |
| `cache.max_entries`  | 0–100,000                                     |
| `sync.strategy`      | `watch`, `webhook`, or `sidecar`              |
| `sync.webhook.secret`| ≥32 bytes (required when strategy is `webhook`) |
| `feed.type`          | `atom` or `rss` (when `feed.enabled` is `true`) |
| `feed.items`         | 1–100 (when `feed.enabled` is `true`)          |

### Environment Variable Overrides

All environment variables use the `BLOGFLOW_` prefix:

| Environment Variable                | Config Path                  | Type     |
|-------------------------------------|------------------------------|----------|
| `BLOGFLOW_SITE_TITLE`              | `site.title`                 | string   |
| `BLOGFLOW_SITE_DESCRIPTION`        | `site.description`           | string   |
| `BLOGFLOW_SITE_BASE_URL`           | `site.base_url`              | string   |
| `BLOGFLOW_SERVER_PORT`             | `server.port`                | int      |
| `BLOGFLOW_SERVER_READ_TIMEOUT`     | `server.read_timeout`        | duration |
| `BLOGFLOW_SERVER_WRITE_TIMEOUT`    | `server.write_timeout`       | duration |
| `BLOGFLOW_SERVER_IDLE_TIMEOUT`     | `server.idle_timeout`        | duration |
| `BLOGFLOW_CACHE_ENABLED`           | `cache.enabled`              | bool     |
| `BLOGFLOW_SYNC_STRATEGY`           | `sync.strategy`              | string   |
| `BLOGFLOW_WEBHOOK_SECRET`          | `sync.webhook.secret`        | string   |
| `BLOGFLOW_SYNC_WEBHOOK_RATE_LIMIT` | `sync.webhook.rate_limit`    | int      |
| `BLOGFLOW_FEED_TYPE`               | `feed.type`                  | string   |

### Secret Handling

BlogFlow enforces strict secret hygiene:

- **Webhook secret** (`sync.webhook.secret`): Must be set via
  `BLOGFLOW_WEBHOOK_SECRET` environment variable. Never place secrets in
  `site.yaml`.
- **Git tokens**: Use environment variables or mounted secrets (Kubernetes).
- **YAML scanning**: BlogFlow scans YAML files for common secret patterns
  (tokens, keys, passwords) and rejects them before parsing.
- **YAML safety**: Anchor/alias constructs are rejected to prevent
  billion-laughs denial-of-service attacks.
- **Config file size**: Limited to 1 MB.
