---
title: "Content Authoring Guide"
slug: content-authoring
date: 2025-07-19
tags: ["guide", "content", "markdown"]
description: "How to write posts and pages for BlogFlow — front matter, markdown features, and content organization."
---

# Content Authoring Guide

Everything you need to write, organize, and publish content with BlogFlow.

## Front Matter

Every post starts with a YAML front matter block:

```yaml
---
title: "Deploying to Kubernetes"
slug: "deploying-to-kubernetes"
date: 2025-06-20
updated: 2025-07-01
tags: ["kubernetes", "devops"]
description: "Step-by-step K8s deployment guide."
draft: false
---
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `title` | string | **Yes** | — | Displayed in listings and feeds |
| `date` | time | **Yes** | — | Publication date (YYYY-MM-DD) |
| `slug` | string | No | filename | URL path segment |
| `updated` | time | No | — | Last-modified date |
| `draft` | bool | No | false | Hidden from listings when true |
| `tags` | []string | No | [] | Tags for filtering |
| `description` | string | No | auto | Summary for feeds and meta tags |

## Posts vs Pages

**Posts** live in `posts/`, require a date, and appear in the feed and post list. They're sorted by date (newest first).

**Pages** live in `pages/`, don't require a date, and are standalone. Use them for "About", "Contact", or landing pages. Pages are sorted by `weight` (ascending), then title.

## Content Directory Structure

```
content/
├── posts/           # Blog posts (configurable: content.posts_dir)
│   ├── first-post.md
│   └── second-post.md
├── pages/           # Static pages (configurable: content.pages_dir)
│   └── about.md
├── media/           # Images and files (configurable: content.media_dir)
│   └── hero.png
└── config/
    └── site.yaml    # Site configuration
```

## Markdown Features

BlogFlow supports GitHub Flavored Markdown plus extras:

- **Tables** — GFM pipe tables
- **Task lists** — `- [x] done` / `- [ ] todo`
- **Strikethrough** — `~~deleted~~`
- **Footnotes** — `[^1]` references
- **Fenced code blocks** — with syntax highlighting via Chroma
- **Auto heading IDs** — for deep linking
- **Smart quotes** — straight quotes become curly

> **Note**: Raw HTML is stripped by default for XSS safety. Use markdown for all formatting.

## Customization Levels

| Level | What You Add | What Changes |
|-------|-------------|--------------|
| 0 | Just markdown | Default theme, default config |
| 1 | `config/site.yaml` | Your title, description, base URL |
| 2 | Theme directory | Custom templates and CSS |
| 3 | Separate repos | Content repo + theme repo via git-sync |

## Git Workflow

BlogFlow works best with trunk-based development:

1. Write content on a branch (`post/my-new-article`)
2. Open a pull request for review
3. Merge to `main` — BlogFlow picks up changes automatically
