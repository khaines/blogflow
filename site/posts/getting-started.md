---
title: "Getting Started with BlogFlow"
slug: getting-started
date: 2025-07-20
tags: ["tutorial", "quickstart"]
description: "Set up your first BlogFlow site in under five minutes."
---

# Getting Started with BlogFlow

Get a blog running in under five minutes. No database, no CMS, no build step.

## Prerequisites

- Docker (or a Go toolchain if building from source)
- A terminal

## Option 1: Docker (Recommended)

```bash
docker run -p 8080:8080 ghcr.io/khaines/blogflow:latest
```

Open `http://localhost:8080`. You'll see the default blog with example content.

## Option 2: Build from Source

```bash
git clone https://github.com/khaines/blogflow.git
cd blogflow
make build
./bin/blogflow --dev
```

## Adding Your Content

Create a content directory with posts and pages:

```
my-blog/
├── posts/
│   └── hello-world.md
├── pages/
│   └── about.md
└── config/
    └── site.yaml
```

### Write Your First Post

Create `posts/hello-world.md`:

```markdown
---
title: "Hello World"
date: 2025-07-20
tags: ["intro"]
---

Welcome to my blog, powered by BlogFlow!
```

### Configure Your Site

Create `config/site.yaml`:

```yaml
site:
  title: "My Blog"
  description: "Thoughts on code and craft"
  base_url: "https://myblog.example.com"
```

### Run with Your Content

```bash
docker run -p 8080:8080 \
  -v $(pwd)/my-blog:/content \
  ghcr.io/khaines/blogflow:latest \
  --content /content
```

Or, point BlogFlow at a git repository:

```bash
docker run -p 8080:8080 \
  -e BLOGFLOW_SYNC_REPO=https://github.com/you/my-blog-content.git \
  ghcr.io/khaines/blogflow:latest
```

## What's Next?

- [Content Authoring Guide](/posts/content-authoring) — front matter schema, pages vs posts, media
- [Deployment Guide](/posts/deployment-guide) — production Docker, Kubernetes, Helm
