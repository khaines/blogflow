---
title: "Hello, World!"
slug: "hello-world"
date: 2025-01-15
tags: ["intro", "blogflow"]
description: "Welcome to BlogFlow — a compact blog engine powered by Go and Markdown."
draft: false
---

Welcome to **BlogFlow**! If you're reading this, your blog engine is up and running.

BlogFlow is a compact, efficient blog engine written in Go. It takes your Markdown files, applies a clean default theme, and serves a fast, static-feeling blog from a single binary.

## What Just Happened?

You dropped a Markdown file into a `posts/` directory and BlogFlow did the rest:

1. Scanned the content directory for `.md` files
2. Parsed the YAML front matter (the `---` block above)
3. Rendered the Markdown body to HTML using goldmark
4. Applied the default template
5. Served the result at [localhost:8080](http://localhost:8080)

No build step. No configuration file. No database.

## What's Next?

- **Add more posts** — create new `.md` files in `posts/`
- **Customize your site** — add a `site.yaml` config with your title and URL
- **Swap the theme** — point `--theme` at your own templates and CSS
- **Deploy** — build the Docker image and push to your favorite platform

Happy writing!
