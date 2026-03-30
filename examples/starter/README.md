# BlogFlow Starter Content

A ready-to-use content directory for [BlogFlow](https://github.com/khaines/blogflow).
Copy or fork this directory to start your blog in minutes.

## Quick start

### Option 1 — Local development

```bash
# Copy the starter directory
cp -r examples/starter ~/my-blog

# Run BlogFlow pointing at your content and config
blogflow --content ~/my-blog --config ~/my-blog/config
```

Open <http://localhost:8080> to see your blog.

### Option 2 — Docker Compose

```yaml
# docker-compose.yml
services:
  blogflow:
    image: ghcr.io/khaines/blogflow:latest
    command: ["--content", "/data/content", "--config", "/data/config"]
    ports:
      - "8080:8080"
    volumes:
      - ./my-blog/posts:/data/content/posts:ro
      - ./my-blog/pages:/data/content/pages:ro
      - ./my-blog/static:/data/content/static:ro
      - ./my-blog/config:/data/config:ro
    environment:
      BLOGFLOW_SYNC_STRATEGY: watch
```

```bash
docker compose up
```

## Directory structure

```
starter/
├── config/
│   └── site.yaml          # Site configuration (title, author, theme, etc.)
├── posts/                  # Blog posts (Markdown with YAML front matter)
│   ├── welcome.md
│   └── getting-started.md
├── pages/                  # Static pages (about, contact, etc.)
│   └── about.md
├── static/                 # Static assets (images, CSS, JS)
│   └── .gitkeep
└── .github/
    └── workflows/
        └── content-deploy.yml  # CI workflow to trigger BlogFlow webhook
```

## Front matter reference

Every Markdown file starts with YAML front matter between `---` fences:

```yaml
---
title: "My Post Title"          # Required — displayed as the heading
date: 2026-03-30                # Required — publish date (YYYY-MM-DD)
slug: "my-post-title"           # Optional — URL path segment (derived from filename if omitted)
tags: ["go", "blogging"]        # Optional — list of tags for filtering
description: "A short summary"  # Optional — used in feeds and meta tags
draft: false                    # Optional — set to true to hide from listings
---

Your content here in Markdown…
```

### Posts vs. Pages

| Feature         | Posts (`posts/`)                | Pages (`pages/`)              |
| --------------- | ------------------------------- | ----------------------------- |
| URL pattern     | `/posts/<slug>`                 | `/<slug>`                     |
| Listed on index | ✅ Yes                          | ❌ No                         |
| Date required   | ✅ Yes                          | ❌ Optional                   |
| Appears in feed | ✅ Yes                          | ❌ No                         |

## Customization

- **Site settings** — edit `config/site.yaml` (title, author, base URL, etc.)
- **Theme overrides** — set `theme.path` in site.yaml to a custom theme directory
- **Static files** — drop images, CSS, or JS into `static/`

## CI / Deployment

The included `.github/workflows/content-deploy.yml` triggers a BlogFlow webhook
whenever you push to `main`. To set it up:

1. Add `BLOGFLOW_WEBHOOK_URL` and `BLOGFLOW_WEBHOOK_SECRET` as repository secrets
2. Push to `main` — the workflow sends a signed webhook to your BlogFlow instance

See the [BlogFlow deployment guide](https://github.com/khaines/blogflow/blob/main/docs/deployment-guide.md)
for full details.
