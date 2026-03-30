---
title: "Theme Development Guide"
slug: "theme-development"
date: 2026-03-23
tags: ["theme", "templates", "css"]
description: "How to create and customize BlogFlow themes."
---
# Theme Development Guide

> **Audience**: Theme developers and front-end engineers  
> **Prerequisite**: Familiarity with HTML, CSS, and Go `html/template` syntax  
> **Last Updated**: 2026-03-30

This guide covers how to build, customize, and override BlogFlow themes. Start
with the [directory structure](#1-theme-directory-structure) to understand the
layout, then work through templates, functions, and styling.

---

## 1. Theme Directory Structure

A BlogFlow theme is a directory containing templates, static assets, and an
optional manifest:

```
my-theme/
├── theme.yaml                # Theme metadata (optional)
├── templates/
│   ├── base.html             # Master layout — all pages extend this
│   ├── post.html             # Single blog post
│   ├── list.html             # Post listing / homepage
│   ├── page.html             # Static page (about, contact)
│   ├── 404.html              # Not found page
│   └── partials/
│       ├── header.html       # Site header / navigation
│       ├── footer.html       # Site footer
│       ├── post-meta.html    # Post date, tags, reading time
│       └── pagination.html   # Previous / next page controls
└── static/
    ├── css/
    │   └── main.css          # Primary stylesheet
    ├── images/
    │   └── favicon.svg       # Site icon
    └── js/                   # Client-side scripts (if any)
```

**Activate a custom theme**:

```bash
blogflow --content ./content --theme ./my-theme
```

Or in `site.yaml`:

```yaml
theme:
  name: "my-theme"
  path: "./my-theme"
```

---

## 2. Go html/template Syntax Basics

BlogFlow templates use Go's standard
[`html/template`](https://pkg.go.dev/html/template) package, which provides
automatic HTML escaping for XSS safety.

### Actions

```html
<!-- Output a value (auto-escaped) -->
{{ .Site.Title }}

<!-- Conditional -->
{{ if .Post }}
  <h1>{{ .Post.Title }}</h1>
{{ else }}
  <h1>{{ .Title }}</h1>
{{ end }}

<!-- Range over a slice -->
{{ range .Posts }}
  <article>
    <h2>{{ .Title }}</h2>
    <p>{{ .Summary }}</p>
  </article>
{{ end }}

<!-- Range with empty fallback -->
{{ range .Posts }}
  <article>{{ .Title }}</article>
{{ else }}
  <p>No posts yet.</p>
{{ end }}

<!-- Variable assignment -->
{{ $name := .Site.Author.Name }}
<span>{{ $name }}</span>

<!-- With (rebind dot) -->
{{ with .Post }}
  <h1>{{ .Title }}</h1>
{{ end }}
```

### Template Blocks

The base template defines named blocks that child templates override:

```html
<!-- base.html -->
<html>
<head>
  <title>{{ block "title" . }}{{ .Site.Title }}{{ end }}</title>
</head>
<body>
  {{ template "partials/header.html" . }}
  <main>{{ block "content" . }}{{ end }}</main>
  {{ template "partials/footer.html" . }}
</body>
</html>
```

```html
<!-- post.html — overrides blocks from base.html -->
{{ define "title" }}{{ .Post.Title }} — {{ .Site.Title }}{{ end }}

{{ define "content" }}
<article>
  <h1>{{ .Post.Title }}</h1>
  {{ template "post-meta" .Post }}
  <div class="post-content">{{ .Post.Content }}</div>
</article>
{{ end }}
```

### Including Partials

```html
{{ template "partials/header.html" . }}
{{ template "partials/footer.html" . }}
{{ template "post-meta" .Post }}
{{ template "pagination" . }}
```

The dot (`.`) passes the current data context to the partial.

---

## 3. Available Template Functions

BlogFlow registers these functions in addition to Go's built-in template
functions:

| Function      | Signature                            | Description                                |
|---------------|--------------------------------------|--------------------------------------------|
| `formatDate`  | `(t time.Time, layout string) string` | Format a time value using a Go time layout string. |
| `now`         | `() time.Time`                       | Returns the current time.                  |
| `lower`       | `(s string) string`                  | Convert string to lowercase.               |
| `upper`       | `(s string) string`                  | Convert string to uppercase.               |
| `truncate`    | `(s string, n int) string`           | Truncate to `n` runes at word boundary, appending `…`. |
| `readingTime` | `(content string) int`               | Estimated reading time in minutes (~200 wpm). |
| `urlize`      | `(s string) string`                  | Convert to URL-safe slug: lowercase, spaces→hyphens, strip special chars. |
| `add`         | `(a, b int) int`                     | Integer addition.                          |
| `sub`         | `(a, b int) int`                     | Integer subtraction.                       |
| `seq`         | `(start, end int) ([]int, error)`    | Generate an integer sequence. Max range: 10,000. |

### Usage Examples

```html
<!-- Format a post date -->
<time datetime="{{ formatDate .Date "2006-01-02" }}">
  {{ formatDate .Date "January 2, 2006" }}
</time>

<!-- Show reading time (readingTime always returns at least 1) -->
{{ $min := readingTime .Content }}
<span>{{ $min }} min read</span>

<!-- Generate a tag URL -->
<a href="/tags/{{ urlize .Tag }}">{{ .Tag }}</a>

<!-- Truncate a summary -->
<p>{{ truncate .Summary 120 }}</p>

<!-- Pagination arithmetic -->
<span>Page {{ .Pagination.CurrentPage }} of {{ .Pagination.TotalPages }}</span>

<!-- Previous / Next links using PrevPage and NextPage (int fields) -->
{{ if .Pagination.HasPrev }}<a href="?page={{ .Pagination.PrevPage }}">← Prev</a>{{ end }}
{{ if .Pagination.HasNext }}<a href="?page={{ .Pagination.NextPage }}">Next →</a>{{ end }}

<!-- Generate page number sequence -->
{{ range seq 1 .Pagination.TotalPages }}
  <a href="?page={{ . }}">{{ . }}</a>
{{ end }}

<!-- Copyright year -->
<footer>&copy; {{ formatDate now "2006" }} {{ .Site.Author.Name }}</footer>
```

---

## 4. Template Data Context

Every template receives a `PageData` struct as its root context (`.`):

### PageData

| Field        | Type              | Available In         | Description                          |
|--------------|-------------------|----------------------|--------------------------------------|
| `.Site`      | `SiteConfig`      | All templates        | Site metadata from configuration.     |
| `.Feed`      | `FeedConfig`      | All templates        | Feed settings (enabled, type, items). |
| `.Post`      | `*Post`           | `post.html`          | The current blog post.               |
| `.Page`      | `*Post`           | `page.html`          | The current static page.             |
| `.Posts`     | `[]*Post`         | `list.html`          | List of posts for current page.      |
| `.Tag`       | `string`          | `list.html` (tag)    | Current tag filter (empty on homepage). |
| `.Title`     | `string`          | All templates        | Page title override.                 |
| `.Pagination`| `*Pagination`     | `list.html`          | Pagination metadata.                 |

### Post (and Page)

Posts and pages share the same structure:

| Field          | Type            | Description                                |
|----------------|-----------------|--------------------------------------------|
| `.Title`       | `string`        | Title from front matter.                   |
| `.Slug`        | `string`        | URL segment.                               |
| `.Date`        | `time.Time`     | Publication date.                          |
| `.Updated`     | `time.Time`     | Last modified date (zero if unset).        |
| `.Draft`       | `bool`          | Draft status.                              |
| `.Tags`        | `[]string`      | Tag list.                                  |
| `.Categories`  | `[]string`      | Category list.                             |
| `.Author`      | `string`        | Author name (post-level or site default).  |
| `.Description` | `string`        | Summary for SEO / feeds.                   |
| `.Template`    | `string`        | Custom template name (if specified).       |
| `.Image`       | `string`        | Featured image URL.                        |
| `.Content`     | `template.HTML` | Rendered HTML from Markdown (safe to emit).|
| `.Summary`     | `string`        | Auto-generated plain-text summary (~200 chars). |
| `.ReadingTime` | `int`           | Estimated reading time in minutes.         |
| `.Path`        | `string`        | Original `.md` file path relative to content root. |

### SiteConfig

| Field              | Type     | Description                           |
|--------------------|----------|---------------------------------------|
| `.Site.Title`      | `string` | Site display name.                    |
| `.Site.Description`| `string` | Meta description.                     |
| `.Site.BaseURL`    | `string` | Canonical base URL.                   |
| `.Site.Language`   | `string` | BCP 47 language tag (e.g., `"en"`).   |
| `.Site.Author.Name`| `string` | Default author name.                  |
| `.Site.Author.Email`| `string`| Default author email.                 |

### FeedConfig

| Field          | Type   | Description                              |
|----------------|--------|------------------------------------------|
| `.Feed.Enabled`| `bool` | Whether feed generation is on.            |
| `.Feed.Type`   | `string`| `"atom"` or `"rss"`.                    |
| `.Feed.Items`  | `int`  | Maximum items in the feed.               |

### Pagination

| Field                       | Type   | Description                         |
|-----------------------------|--------|-------------------------------------|
| `.Pagination.CurrentPage`   | `int`  | Current page number (1-indexed).    |
| `.Pagination.TotalPages`    | `int`  | Total number of pages.              |
| `.Pagination.HasPrev`       | `bool` | Whether a previous page exists.     |
| `.Pagination.HasNext`       | `bool` | Whether a next page exists.         |
| `.Pagination.PrevPage`      | `int`  | Previous page number.               |
| `.Pagination.NextPage`      | `int`  | Next page number.                   |

---

## 5. Partial Templates

Partials live in `templates/partials/` and are included with the `template`
action. The template name is the `{{define "name"}}` identifier inside the
file — **not** the file path:

```html
{{ template "partials/header.html" . }}
```

### Default Partials

BlogFlow ships these partials in the embedded defaults:

| `{{define}}` Name          | File                         | Purpose                                              |
|----------------------------|------------------------------|------------------------------------------------------|
| `partials/header.html`     | `partials/header.html`       | Site header with navigation and title link.           |
| `partials/footer.html`     | `partials/footer.html`       | Site footer with copyright and attribution.           |
| `post-meta`                | `partials/post-meta.html`    | Post metadata: date, reading time, tag links.         |
| `pagination`               | `partials/pagination.html`   | Previous / next page navigation with aria labels.     |

> **Naming convention**: The `{{define}}` name must match exactly what
> `{{template}}` uses. The default header and footer use full-path names
> (`partials/header.html`, `partials/footer.html`), while post-meta and
> pagination use short names. When overriding a partial, keep the same
> `{{define}}` name so existing `{{template}}` calls continue to resolve.

### Creating Custom Partials

Add new partials to your theme's `templates/partials/` directory:

```html
<!-- templates/partials/social-links.html -->
<nav class="social-links" aria-label="Social media">
  {{ with .Site.Author.Name }}
    <a href="https://github.com/{{ urlize . }}">GitHub</a>
  {{ end }}
</nav>
```

Then include it in any template:

```html
{{ template "social-links" . }}
```

---

## 6. Overriding Default Templates via Overlay FS

BlogFlow's overlay filesystem lets you selectively replace default templates
without forking the entire theme. Place files at the same relative path in your
theme directory to shadow the defaults.

### Resolution Order

The overlay FS checks layers in this order (highest priority first):

1. **Theme layer** — your custom `--theme` directory
2. **Content layer** — the `--content` directory
3. **Config layer** — configuration files
4. **Defaults layer** — embedded defaults (`embed.FS`)

### Override Examples

**Override just the header**:

```
my-theme/
└── templates/
    └── partials/
        └── header.html    # ← shadows defaults/templates/partials/header.html
```

All other templates (`base.html`, `post.html`, `footer.html`, etc.) continue
to use the embedded defaults.

**Override the base layout**:

```
my-theme/
└── templates/
    └── base.html          # ← shadows defaults/templates/base.html
```

**Override CSS only** (no template changes):

```
my-theme/
└── static/
    └── css/
        └── main.css       # ← shadows defaults/static/css/main.css
```

### How It Works

When BlogFlow resolves a template path like `templates/post.html`:

1. Check theme layer → if found, use it
2. Check content layer → if found, use it
3. Check config layer → if found, use it
4. Check defaults layer → use the embedded version

The overlay FS implements Go's `fs.FS`, `fs.ReadFileFS`, `fs.ReadDirFS`, and
`fs.StatFS` interfaces. `ReadDir` returns the **union** of directory entries
across all layers, with higher layers shadowing lower ones.

### Safety Features

- **Max file size**: 64 MB per file
- **Symlink escape detection**: disk-backed layers reject symlinks that escape
  the layer root
- **Negative cache**: absent paths are cached (up to 100,000 entries) to avoid
  repeated layer traversal
- **Hot reload**: layers can be invalidated and replaced at runtime (used by
  sync strategies)

---

## 7. CSS Architecture

### Custom Properties

The default theme uses CSS custom properties for consistent theming:

```css
:root {
  --color-bg: #ffffff;
  --color-text: #1a1a1a;
  --color-link: #0066cc;
  --color-link-hover: #004499;
  --color-border: #e0e0e0;
  --color-code-bg: #f5f5f5;
  --font-body: system-ui, -apple-system, sans-serif;
  --font-mono: "SFMono-Regular", Consolas, monospace;
  --max-width: 42rem;
}
```

Override these in your custom `main.css` to change the entire color scheme
without rewriting styles.

### Dark Mode

Implement dark mode with a `prefers-color-scheme` media query:

```css
@media (prefers-color-scheme: dark) {
  :root {
    --color-bg: #1a1a1a;
    --color-text: #e0e0e0;
    --color-link: #66b3ff;
    --color-link-hover: #99ccff;
    --color-border: #333333;
    --color-code-bg: #2d2d2d;
  }
}
```

### Responsive Design

The default theme is mobile-first with a fluid layout:

```css
body {
  max-width: var(--max-width);
  margin: 0 auto;
  padding: 1rem;
}

/* Wider viewports */
@media (min-width: 768px) {
  body {
    padding: 2rem;
  }
}
```

### Syntax Highlighting

Code blocks use CSS classes compatible with
[Chroma](https://github.com/alecthomas/chroma) syntax highlighter. The default
theme includes base Chroma styles. Customize by overriding the `.chroma` and
related classes:

```css
.chroma .kw { color: #007020; font-weight: bold; }  /* Keyword */
.chroma .s  { color: #4070a0; }                      /* String */
.chroma .c  { color: #60a0b0; font-style: italic; }  /* Comment */
```

---

## 8. Static Assets

### Serving Static Files

All files under `static/` in your theme (or the defaults) are served at the
`/static/` URL prefix:

| File Path                       | Served At                     |
|---------------------------------|-------------------------------|
| `static/css/main.css`           | `/static/css/main.css`        |
| `static/images/favicon.svg`     | `/static/images/favicon.svg`  |
| `static/js/app.js`              | `/static/js/app.js`           |

### Referencing in Templates

```html
<!-- CSS -->
<link rel="stylesheet" href="/static/css/main.css">

<!-- Favicon -->
<link rel="icon" href="/static/images/favicon.svg" type="image/svg+xml">

<!-- JavaScript -->
<script src="/static/js/app.js" defer></script>
```

### Adding Custom Assets

Place any static files in your theme's `static/` directory. They are served
through the overlay FS, so theme assets shadow default assets at the same path.

```
my-theme/
└── static/
    ├── css/
    │   ├── main.css          # Overrides default CSS
    │   └── syntax.css        # Additional stylesheet
    ├── images/
    │   └── logo.svg          # Custom logo
    └── js/
        └── theme-toggle.js   # Dark mode toggle
```

### Content-Security-Policy

BlogFlow enforces a strict Content-Security-Policy via an **HTTP header** set
in the server middleware (`internal/server/server.go`,
`securityHeadersMiddleware`). The default policy is:

```
default-src 'none'; script-src 'none'; object-src 'none';
connect-src 'none'; style-src 'self'; img-src 'self' https: data:;
font-src 'self' https:; base-uri 'self'; form-action 'self';
frame-ancestors 'self'
```

The `base.html` meta tag contains a similar CSP as **supplementary
defense-in-depth**. However, when the HTTP header is present (which it always
is when served by BlogFlow), browsers ignore the meta tag CSP entirely. The
meta tag only matters if someone serves the HTML files through a different
server that omits the header.

**To allow external resources** (CDN fonts, analytics scripts, etc.), you must
modify the `securityHeadersMiddleware` in `internal/server/server.go` — editing
the `base.html` meta tag alone has no effect.

> ⚠️ `frame-ancestors` **cannot** be set via a `<meta>` tag — the browser spec
> forbids it. This directive is only effective in the HTTP header, which
> BlogFlow already sets.
