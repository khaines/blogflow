package content

import (
	"bytes"
	"io"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
)

// Option configures the renderer.
type Option func(*rendererOptions)

type rendererOptions struct {
	unsafeHTML bool
	hardWraps  bool
}

// WithUnsafeHTML allows raw HTML in markdown input.
// Use only for trusted content (e.g., embedded defaults).
func WithUnsafeHTML() Option {
	return func(o *rendererOptions) { o.unsafeHTML = true }
}

// WithHardWraps converts newlines in markdown to <br> tags.
func WithHardWraps() Option {
	return func(o *rendererOptions) { o.hardWraps = true }
}

// Renderer converts Markdown content to HTML using goldmark.
type Renderer struct {
	md goldmark.Markdown
}

// NewRenderer creates a new markdown renderer with GFM extensions.
// By default, raw HTML in markdown is stripped (secure mode).
// Use WithUnsafeHTML() to allow raw HTML for trusted content.
func NewRenderer(opts ...Option) *Renderer {
	o := &rendererOptions{}
	for _, fn := range opts {
		fn(o)
	}

	var rendererOpts []renderer.Option
	if o.unsafeHTML {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}
	if o.hardWraps {
		rendererOpts = append(rendererOpts, html.WithHardWraps())
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.Typographer,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	return &Renderer{md: md}
}

// Render converts markdown bytes to HTML bytes.
func (r *Renderer) Render(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := r.md.Convert(src, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderString converts a markdown string to an HTML string.
func (r *Renderer) RenderString(src string) (string, error) {
	data, err := r.Render([]byte(src))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RenderTo writes rendered HTML to the given writer.
func (r *Renderer) RenderTo(w io.Writer, src []byte) error {
	return r.md.Convert(src, w)
}
