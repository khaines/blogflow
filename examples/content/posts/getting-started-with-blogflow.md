---
title: "Getting Started with BlogFlow"
slug: "getting-started-with-blogflow"
date: 2025-01-20
tags: ["tutorial", "blogflow", "go"]
description: "A step-by-step guide to setting up your blog with BlogFlow, from first post to custom configuration."
author: "BlogFlow Team"
draft: false
---

This tutorial walks through setting up a blog with BlogFlow — from zero to a customized, running site.

## Prerequisites

- [Go 1.22+](https://go.dev/dl/) (to build from source) or a pre-built binary
- A text editor
- A terminal

## Step 1: Install BlogFlow

Build from source:

```bash
git clone https://github.com/khaines/blogflow.git
cd blogflow
make build
```

The binary lands in `bin/blogflow`.

## Step 2: Create Your Content Directory

BlogFlow expects a simple directory structure:

```
my-blog/
├── content/
│   ├── posts/        # Blog posts (date-ordered)
│   │   └── hello.md
│   └── pages/        # Static pages (about, contact)
│       └── about.md
└── site.yaml         # Optional configuration
```

Create it:

```bash
mkdir -p my-blog/content/posts my-blog/content/pages
```

## Step 3: Write Your First Post

Create `my-blog/content/posts/first-post.md`:

```markdown
---
title: "My First Post"
slug: "my-first-post"
date: 2025-01-20
tags: ["hello"]
description: "My very first blog post."
---

This is my first post! BlogFlow makes blogging simple.
```

### Front Matter Fields

Every post starts with YAML front matter between `---` delimiters:

| Field | Required | Description |
|---|---|---|
| `title` | Yes | Post title displayed in headings and feeds |
| `slug` | No | URL path segment (defaults to filename) |
| `date` | Yes | Publication date (`YYYY-MM-DD`) |
| `tags` | No | List of tags for categorization |
| `description` | No | Summary for feeds and SEO meta tags |
| `draft` | No | Set `true` to hide from listings |
| `author` | No | Post author name |
| `image` | No | Featured image URL |
| `categories` | No | List of categories |
| `template` | No | Override the default post template |

## Step 4: Run BlogFlow

```bash
./bin/blogflow --content ./my-blog/content --dev
```

Flags explained:

- `--content` — path to your content directory
- `--dev` — enables verbose logging and disables caching

Open [http://localhost:8080](http://localhost:8080) to see your blog.

## Step 5: Add Configuration

Create `my-blog/site.yaml` to personalize your site:

```yaml
site:
  title: "My Awesome Blog"
  description: "Writing about code and life"
  base_url: "https://myblog.example.com"
  author:
    name: "Your Name"
    email: "you@example.com"

content:
  posts_per_page: 5

feed:
  enabled: true       # (coming soon) feed generation is a planned feature
  type: "atom"
```

> **Tip:** You don't need to specify every field. BlogFlow merges your config
> with sensible defaults — only override what you want to change.

## Step 6: Add a Static Page

Create `my-blog/content/pages/about.md`:

```markdown
---
title: "About"
slug: "about"
description: "Learn more about this blog."
---

Welcome! This blog is powered by BlogFlow.
```

Pages are served at `/<slug>` — this one appears at `/about`.

## Step 7: Deploy with Docker

Use the pre-built BlogFlow image (or build from the
[engine repo](https://github.com/khaines/blogflow)):

```bash
docker run -p 8080:8080 \
  -v ./my-blog/content:/data/content:ro \
  -v ./my-blog/site.yaml:/data/config/site.yaml:ro \
  -e BLOGFLOW_SITE_BASE_URL="https://myblog.example.com" \
  blogflow --content /data/content --config /data/config
```

The resulting image is under 15 MB and runs rootless on a distroless base.

## What's Next?

- Explore [Markdown Features](/posts/markdown-features) to see everything BlogFlow supports
- Set up webhook sync for automatic deploys on `git push` (coming soon)
- Create a custom theme directory with your own templates and CSS
