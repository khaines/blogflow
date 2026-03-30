---
title: "Getting Started with BlogFlow"
slug: "getting-started"
date: 2026-03-30
tags: ["guide", "blogflow"]
description: "How to add posts, customize your site, and deploy with BlogFlow."
---

This guide covers the basics of running and customizing your BlogFlow blog.

## Adding posts

Creating a new post is simple:

1. **Create a file** — add a `.md` file in the `posts/` directory
2. **Add front matter** — include the required YAML metadata at the top:

   ```yaml
   ---
   title: "Your Post Title"
   slug: "your-post-title"
   date: 2026-04-15
   tags: ["topic"]
   description: "A brief summary."
   ---
   ```

3. **Write content** — use standard Markdown below the front matter
4. **Publish** — push to your `main` branch (webhook mode) or save the file (watch mode)

BlogFlow detects changes automatically and updates your site without a restart.

## Adding pages

Static pages like "About" or "Contact" go in the `pages/` directory. They work
just like posts but appear at the root URL path (`/about` instead of `/posts/about`).

## Customizing your site

### Site configuration

Edit `config/site.yaml` to change your blog's identity:

```yaml
site:
  title: "My Awesome Blog"
  description: "Thoughts on code and coffee"
  base_url: "https://myblog.example.com"
  author:
    name: "Your Name"
    email: "you@example.com"
```

Every setting has a sensible default — you only need to override what you want to
change. See the comments in `site.yaml` for all available options.

### Theme overrides

BlogFlow ships with a clean default theme. To customize it:

1. Create a theme directory (e.g. `themes/custom/`)
2. Add template overrides — only the files you include will replace the defaults
3. Point to it in `site.yaml`:

   ```yaml
   theme:
     name: "custom"
     path: "themes/custom"
   ```

### Environment variables

Any setting can be overridden with an environment variable prefixed with
`BLOGFLOW_`. For example:

```bash
export BLOGFLOW_SITE_BASE_URL="https://myblog.example.com"
export BLOGFLOW_SERVER_PORT=3000
```

Sensitive values like webhook secrets should **always** use environment variables
rather than `site.yaml`.

## Deployment

The included CI workflow (`.github/workflows/content-deploy.yml`) triggers a
BlogFlow webhook on every push to `main`. To set it up:

1. Deploy BlogFlow to your server or container platform
2. Add these repository secrets:
   - `BLOGFLOW_WEBHOOK_URL` — your BlogFlow instance's webhook endpoint
   - `BLOGFLOW_WEBHOOK_SECRET` — a shared secret (minimum 32 characters)
3. Push content to `main` — BlogFlow pulls the latest changes automatically

## Learn more

- [BlogFlow README](https://github.com/khaines/blogflow#readme)
- [Configuration reference](https://github.com/khaines/blogflow/blob/main/examples/config/site.yaml)
- [Deployment guide](https://github.com/khaines/blogflow/blob/main/docs/deployment-guide.md)
- [Kubernetes examples](https://github.com/khaines/blogflow/tree/main/examples/k8s)
