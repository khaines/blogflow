package content

import (
	"bytes"
	"strings"
	"testing"
)

func TestRender_BasicMarkdown(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"heading", "# Hello", `<h1 id="hello">Hello</h1>`},
		{"paragraph", "Hello world", "<p>Hello world</p>"},
		{"bold", "**bold**", "<p><strong>bold</strong></p>"},
		{"italic", "*italic*", "<p><em>italic</em></p>"},
		{"bold+italic", "***both***", "<p><em><strong>both</strong></em></p>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RenderString(tt.in)
			if err != nil {
				t.Fatalf("RenderString() error: %v", err)
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_GFM(t *testing.T) {
	r := NewRenderer()

	t.Run("strikethrough", func(t *testing.T) {
		got, err := r.RenderString("~~deleted~~")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "<del>deleted</del>") {
			t.Errorf("expected strikethrough, got %q", got)
		}
	})

	t.Run("task_list", func(t *testing.T) {
		got, err := r.RenderString("- [x] done\n- [ ] todo")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, `checked=""`) && !strings.Contains(got, "checked") {
			t.Errorf("expected checked checkbox, got %q", got)
		}
	})

	t.Run("table", func(t *testing.T) {
		md := "| A | B |\n|---|---|\n| 1 | 2 |"
		got, err := r.RenderString(md)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "<table>") {
			t.Errorf("expected table, got %q", got)
		}
	})

	t.Run("autolink", func(t *testing.T) {
		got, err := r.RenderString("Visit https://example.com today")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, `href="https://example.com"`) {
			t.Errorf("expected autolink, got %q", got)
		}
	})
}

func TestRender_Footnotes(t *testing.T) {
	r := NewRenderer()
	md := "Text with a footnote[^1].\n\n[^1]: This is the footnote."
	got, err := r.RenderString(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "footnote") {
		t.Errorf("expected footnote markup, got %q", got)
	}
}

func TestRender_CodeBlocks(t *testing.T) {
	r := NewRenderer()
	md := "```go\nfmt.Println(\"hello\")\n```"
	got, err := r.RenderString(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<code") {
		t.Errorf("expected code block, got %q", got)
	}
	if !strings.Contains(got, "language-go") {
		t.Errorf("expected language-go class, got %q", got)
	}
}

func TestRender_SecureDefault(t *testing.T) {
	r := NewRenderer()
	md := "before\n\n<script>alert('xss')</script>\n\nafter"
	got, err := r.RenderString(md)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("raw HTML should be stripped by default, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("text content should be preserved, got %q", got)
	}
}

func TestRender_UnsafeOption(t *testing.T) {
	r := NewRenderer(WithUnsafeHTML())
	md := "<div class=\"custom\">content</div>"
	got, err := r.RenderString(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `<div class="custom">`) {
		t.Errorf("raw HTML should be allowed with WithUnsafeHTML(), got %q", got)
	}
}

func TestRender_HeadingIDs(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		md     string
		wantID string
	}{
		{"# Hello World", `id="hello-world"`},
		{"## Go Code", `id="go-code"`},
		{"### Multi Word Heading", `id="multi-word-heading"`},
	}

	for _, tt := range tests {
		got, err := r.RenderString(tt.md)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, tt.wantID) {
			t.Errorf("RenderString(%q): expected %q in output, got %q", tt.md, tt.wantID, got)
		}
	}
}

func TestRenderString(t *testing.T) {
	r := NewRenderer()
	got, err := r.RenderString("hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<p>hello</p>") {
		t.Errorf("expected paragraph, got %q", got)
	}
}

func TestRenderTo(t *testing.T) {
	r := NewRenderer()
	var buf bytes.Buffer
	err := r.RenderTo(&buf, []byte("# Title"))
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "<h1") {
		t.Errorf("expected h1, got %q", got)
	}
}

func TestRender_HardWraps(t *testing.T) {
	r := NewRenderer(WithHardWraps())
	got, err := r.RenderString("line1\nline2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<br") {
		t.Errorf("expected <br> with hard wraps, got %q", got)
	}
}

func TestRender_JavaScriptURISanitized(t *testing.T) {
	r := NewRenderer()
	html, err := r.RenderString(`[click](javascript:alert(1))`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "javascript:") {
		t.Errorf("javascript: URI not sanitized: %s", html)
	}
}

func TestRender_DataURISanitized(t *testing.T) {
	r := NewRenderer()
	html, err := r.RenderString(`[click](data:text/html,<script>alert(1)</script>)`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "data:") {
		t.Errorf("data: URI not sanitized: %s", html)
	}
}

func TestRender_SafeURLsAllowed(t *testing.T) {
	r := NewRenderer()
	html, err := r.RenderString(`[link](https://example.com) [mail](mailto:a@b.com) [rel](/page) [frag](#id)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://example.com", "mailto:a@b.com", "/page", "#id"} {
		if !strings.Contains(html, want) {
			t.Errorf("safe URL %q not preserved in: %s", want, html)
		}
	}
}

func TestRender_SizeLimit(t *testing.T) {
	r := NewRenderer()
	huge := make([]byte, 11*1024*1024)
	_, err := r.Render(huge)
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestRenderTo_NilWriter(t *testing.T) {
	r := NewRenderer()
	err := r.RenderTo(nil, []byte("# Hello"))
	if err == nil {
		t.Fatal("expected error for nil writer")
	}
}

func TestRender_ProtocolRelativeBlocked(t *testing.T) {
	r := NewRenderer()
	html, err := r.RenderString("[evil](//malicious.com/payload)")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "//malicious.com") {
		t.Errorf("protocol-relative URL not blocked: %s", html)
	}
}
