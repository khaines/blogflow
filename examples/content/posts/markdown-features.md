---
title: "Markdown Features"
slug: "markdown-features"
date: 2025-01-25
tags: ["markdown", "reference", "blogflow"]
description: "A showcase of all Markdown features supported by BlogFlow's goldmark renderer."
draft: false
---

BlogFlow uses [goldmark](https://github.com/yuin/goldmark) with GitHub Flavored Markdown extensions. This post demonstrates every supported feature.

## Headings

Headings from `##` through `######` are supported (h1 is reserved for the post title):

### Third-Level Heading

#### Fourth-Level Heading

##### Fifth-Level Heading

###### Sixth-Level Heading

## Emphasis

- **Bold text** using `**double asterisks**`
- *Italic text* using `*single asterisks*`
- ***Bold and italic*** using `***triple asterisks***`
- ~~Strikethrough~~ using `~~double tildes~~`

## Links

- [Inline link](https://example.com)
- [Link with title](https://example.com "Example Site")
- Autolinks: https://example.com

## Images

![Alt text for an image](/static/images/example.svg "Optional image title")

## Blockquotes

> This is a blockquote. It can span multiple lines
> and supports **formatting** inside.
>
> > Nested blockquotes work too.

## Lists

### Unordered List

- First item
- Second item
  - Nested item A
  - Nested item B
- Third item

### Ordered List

1. First step
2. Second step
   1. Sub-step A
   2. Sub-step B
3. Third step

### Task Lists

- [x] Set up BlogFlow
- [x] Write first post
- [ ] Customize theme
- [ ] Deploy to production

## Code

### Inline Code

Use `blogflow` to start the server. The config struct is `config.Config`.

### Fenced Code Blocks

Go:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello from BlogFlow!")
}
```

YAML (front matter example):

```yaml
---
title: "My Post"
slug: "my-post"
date: 2025-01-25
tags: ["go", "blog"]
draft: false
---
```

Bash:

```bash
# Build and run BlogFlow
make build
./bin/blogflow --dev --content ./content
```

HTML:

```html
<article class="post">
  <h1>{{ .Title }}</h1>
  <div class="content">{{ .Body }}</div>
</article>
```

## Tables

| Feature | Supported | Notes |
|---|:---:|---|
| GFM tables | ✅ | With alignment |
| Task lists | ✅ | Checkbox syntax |
| Strikethrough | ✅ | GFM extension |
| Footnotes | ✅ | Goldmark extension |
| Syntax highlighting | ✅ | Chroma via goldmark-highlighting (CSS classes) |
| Auto-heading IDs | ✅ | For anchor links |

### Right-Aligned Table

| Metric | Value |
|---|---:|
| Binary size | ~18 MB |
| Container image | < 25 MB |
| Startup time | < 100 ms |
| Memory (idle) | ~10 MB |

## Horizontal Rules

Content above the rule.

---

Content below the rule.

## Footnotes

BlogFlow supports footnotes[^1] for citations and references[^2].

[^1]: Footnotes appear at the bottom of the rendered page.
[^2]: They use the goldmark footnote extension.

## Definition-Style Content

While not standard Markdown, you can achieve definition-like formatting:

**Overlay FS**
: External files take priority over embedded defaults. First match wins.

**Goldmark**
: Go community-standard Markdown parser. Extensible, GFM-compatible.

**Distroless**
: Container base image with no OS packages, no shell, minimal attack surface.

## Escaping

Use backslashes to escape Markdown syntax: \*not italic\*, \[not a link\].

Literal backticks in inline code: `` `code` `` uses double backticks.

## HTML in Markdown

BlogFlow sanitizes raw HTML in Markdown for security. Stick to standard Markdown syntax for best results — the goldmark renderer handles everything you need.

## Putting It All Together

A typical BlogFlow post combines several of these features naturally:

> **Pro tip:** Start with front matter, write in plain Markdown, and let
> BlogFlow handle the rest. No build step, no templating language to learn
> in your content files.

The content pipeline is straightforward:

1. Write `.md` files with YAML front matter
2. BlogFlow parses front matter and renders Markdown via goldmark
3. Templates wrap the rendered HTML
4. The result is cached and served

That's it — simple by design.
