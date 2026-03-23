package theme

import (
	"html/template"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/content"
)

func testFS(files map[string]string) fstest.MapFS {
	m := make(fstest.MapFS)
	// Always include base template for the engine's clone+parse pattern
	if _, ok := files["templates/base.html"]; !ok {
		m["templates/base.html"] = &fstest.MapFile{Data: []byte(`<!DOCTYPE html>{{block "content" .}}{{end}}`)}
	}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func TestNewEngine_LoadsTemplates(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/index.html": `{{define "content"}}<h1>Hello</h1>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	var b strings.Builder
	if err := e.Render(&b, "templates/index.html", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := b.String(); !strings.Contains(got, "<h1>Hello</h1>") {
		t.Errorf("output %q does not contain %q", got, "<h1>Hello</h1>")
	}
}

func TestRender_WithData(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/post.html": `{{define "content"}}<h1>{{.Title}}</h1><p>by {{.Author}}</p>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	data := struct {
		Title  string
		Author string
	}{Title: "My Post", Author: "Alice"}

	var b strings.Builder
	if err := e.Render(&b, "templates/post.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `<h1>My Post</h1><p>by Alice</p>`
	if got := b.String(); !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
}

func TestRender_FuncMap(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     any
		want     string
	}{
		{
			name:     "formatDate",
			template: `{{define "content"}}{{formatDate .Date "2006-01-02"}}{{end}}`,
			data:     struct{ Date time.Time }{Date: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
			want:     "2025-03-15",
		},
		{
			name:     "lower",
			template: `{{define "content"}}{{lower .}}{{end}}`,
			data:     "HELLO",
			want:     "hello",
		},
		{
			name:     "upper",
			template: `{{define "content"}}{{upper .}}{{end}}`,
			data:     "hello",
			want:     "HELLO",
		},
		{
			name:     "truncate_short",
			template: `{{define "content"}}{{truncate . 10}}{{end}}`,
			data:     "short",
			want:     "short",
		},
		{
			name:     "truncate_long_word_boundary",
			template: `{{define "content"}}{{truncate . 20}}{{end}}`,
			data:     "This is a long sentence that should be truncated",
			want:     "This is a long…",
		},
		{
			name:     "truncate_long_no_space",
			template: `{{define "content"}}{{truncate . 5}}{{end}}`,
			data:     "abcdefghij",
			want:     "abcde…",
		},
		{
			name:     "urlize",
			template: `{{define "content"}}{{urlize .}}{{end}}`,
			data:     "Hello World! 123",
			want:     "hello-world-123",
		},
		{
			name:     "add",
			template: `{{define "content"}}{{add 2 3}}{{end}}`,
			data:     nil,
			want:     "5",
		},
		{
			name:     "sub",
			template: `{{define "content"}}{{sub 10 3}}{{end}}`,
			data:     nil,
			want:     "7",
		},
		{
			name:     "seq",
			template: `{{define "content"}}{{range seq 1 3}}{{.}} {{end}}{{end}}`,
			data:     nil,
			want:     "1 2 3 ",
		},
		{
			name:     "readingTime_short",
			template: `{{define "content"}}{{readingTime .}}{{end}}`,
			data:     "A few words",
			want:     "1",
		},
		{
			name:     "readingTime_long",
			template: `{{define "content"}}{{readingTime .}}{{end}}`,
			data:     strings.Repeat("word ", 600),
			want:     "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := testFS(map[string]string{
				"templates/t.html": tt.template,
			})
			e, err := NewEngine(fs)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			got, err := e.RenderToString("templates/t.html", tt.data)
			if err != nil {
				t.Fatalf("RenderToString: %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("output %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestRender_Partials(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/partials/header.html": `<header>{{.SiteName}}</header>`,
		"templates/page.html":            `{{define "content"}}{{template "templates/partials/header.html" .}}<main>content</main>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	data := struct{ SiteName string }{SiteName: "BlogFlow"}
	got, err := e.RenderToString("templates/page.html", data)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	want := `<header>BlogFlow</header><main>content</main>`
	if !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
}

func TestRender_MissingTemplate(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/index.html": `{{define "content"}}<h1>Hello</h1>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	var b strings.Builder
	err = e.Render(&b, "templates/nonexistent.html", nil)
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}

func TestRender_Blocks(t *testing.T) {
	t.Run("default_block", func(t *testing.T) {
		// When the page template does not override the block, the default is used.
		fs := testFS(map[string]string{
			"templates/base.html":  `<html>{{block "content" .}}default{{end}}</html>`,
			"templates/empty.html": ``,
		})

		e, err := NewEngine(fs)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		got, err := e.RenderToString("templates/empty.html", nil)
		if err != nil {
			t.Fatalf("RenderToString: %v", err)
		}
		if got != "<html>default</html>" {
			t.Errorf("got %q, want %q", got, "<html>default</html>")
		}
	})

	t.Run("overridden_block", func(t *testing.T) {
		// A {{define}} in a page template overrides the block default.
		fs := testFS(map[string]string{
			"templates/base.html": `<html>{{block "content" .}}default{{end}}</html>`,
			"templates/home.html": `{{define "content"}}home page{{end}}`,
		})

		e, err := NewEngine(fs)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		got, err := e.RenderToString("templates/home.html", nil)
		if err != nil {
			t.Fatalf("RenderToString: %v", err)
		}
		if got != "<html>home page</html>" {
			t.Errorf("got %q, want %q", got, "<html>home page</html>")
		}
	})
}

func TestReload(t *testing.T) {
	m := fstest.MapFS{
		"templates/base.html":  &fstest.MapFile{Data: []byte(`<!DOCTYPE html>{{block "content" .}}{{end}}`)},
		"templates/index.html": &fstest.MapFile{Data: []byte(`{{define "content"}}v1{{end}}`)},
	}

	e, err := NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	got, err := e.RenderToString("templates/index.html", nil)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	if !strings.Contains(got, "v1") {
		t.Errorf("before reload: output %q does not contain %q", got, "v1")
	}

	// Update the page template in the MapFS.
	m["templates/index.html"] = &fstest.MapFile{Data: []byte(`{{define "content"}}v2{{end}}`)}

	if err := e.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got, err = e.RenderToString("templates/index.html", nil)
	if err != nil {
		t.Fatalf("RenderToString after reload: %v", err)
	}
	if !strings.Contains(got, "v2") {
		t.Errorf("after reload: output %q does not contain %q", got, "v2")
	}
}

func TestReload_ConcurrentWithRender(t *testing.T) {
	m := fstest.MapFS{
		"templates/base.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html>{{block "content" .}}{{end}}`)},
		"templates/t.html":    &fstest.MapFile{Data: []byte(`{{define "content"}}v1{{end}}`)},
	}
	e, err := NewEngine(m)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = e.RenderToString("templates/t.html", nil)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = e.Reload()
			}
		}()
	}
	wg.Wait()
}

func TestNewEngine_NoTemplatesDir(t *testing.T) {
	// Use raw MapFS so testFS doesn't auto-add templates/base.html.
	fs := fstest.MapFS{
		"static/main.css": &fstest.MapFile{Data: []byte(`body {}`)},
	}

	_, err := NewEngine(fs)
	if err == nil {
		t.Fatal("expected error when templates/ dir is missing, got nil")
	}
}

func TestRender_HTMLEscaping(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/page.html": `{{define "content"}}<p>{{.Content}}</p>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	data := struct{ Content string }{Content: `<script>alert("xss")</script>`}
	got, err := e.RenderToString("templates/page.html", data)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}

	if strings.Contains(got, "<script>") {
		t.Errorf("expected HTML escaping, got raw script tag: %s", got)
	}
	want := `<p>&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;</p>`
	if !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
}

// TestRender_HTMLEscaping_TemplateHTML verifies that the content pipeline's
// sanitizer — not html/template auto-escaping — is what prevents XSS.
// In production, Post.Content is template.HTML (bypasses auto-escaping).
// This test proves that goldmark's renderer strips dangerous tags before
// the content reaches the template engine.
func TestRender_HTMLEscaping_TemplateHTML(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/page.html": `{{define "content"}}<div>{{.Content}}</div>{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	r := content.NewRenderer()

	// Simulate the production pipeline: render markdown containing dangerous
	// HTML through the content renderer, then pass as template.HTML.
	xssVectors := []struct {
		name      string
		input     string
		forbidden string
	}{
		{"script_tag", "<script>alert('xss')</script>", "<script>"},
		{"event_handler_onerror", `<img src=x onerror="alert('xss')">`, "onerror"},
		{"event_handler_onload", `<svg onload="alert(1)">`, "onload"},
		{"iframe", `<iframe src="javascript:alert(1)"></iframe>`, "<iframe"},
	}

	for _, tt := range xssVectors {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, err := r.RenderString(tt.input)
			if err != nil {
				t.Fatalf("RenderString: %v", err)
			}

			data := struct{ Content template.HTML }{
				Content: template.HTML(sanitized), //nolint:gosec // testing that sanitizer already stripped the tag
			}
			got, err := e.RenderToString("templates/page.html", data)
			if err != nil {
				t.Fatalf("RenderToString: %v", err)
			}

			if strings.Contains(got, tt.forbidden) {
				t.Errorf("template.HTML content contains %q; sanitizer did not strip it: %s", tt.forbidden, got)
			}
		})
	}

	// Prove that template.HTML actually bypasses auto-escaping: safe HTML
	// tags rendered by goldmark must appear unescaped in the output.
	// If they were escaped we'd see &lt;p&gt; instead of <p>.
	t.Run("safe_html_unescaped", func(t *testing.T) {
		safeMD, err := r.RenderString("hello **world**")
		if err != nil {
			t.Fatalf("RenderString: %v", err)
		}
		safeData := struct{ Content template.HTML }{
			Content: template.HTML(safeMD), //nolint:gosec // safe content
		}
		safeGot, err := e.RenderToString("templates/page.html", safeData)
		if err != nil {
			t.Fatalf("RenderToString: %v", err)
		}
		if !strings.Contains(safeGot, "<strong>world</strong>") {
			t.Errorf("template.HTML content was re-escaped; expected raw <strong> tag in: %s", safeGot)
		}
	})
}

// TestReadingTime_TemplateHTML verifies that readingTime works when called
// with template.HTML (the actual type of .Content in production templates),
// not just plain string.
func TestReadingTime_TemplateHTML(t *testing.T) {
	tests := []struct {
		name    string
		content template.HTML
		want    string
	}{
		{
			name:    "short_html_content",
			content: template.HTML("<p>A <strong>few</strong> words here</p>"),
			want:    "1",
		},
		{
			name:    "long_html_content",
			content: template.HTML("<p>" + strings.Repeat("word ", 600) + "</p>"), //nolint:gosec // static test data
			want:    "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := testFS(map[string]string{
				"templates/post.html": `{{define "content"}}{{readingTime .Content}}{{end}}`,
			})
			e, err := NewEngine(fs)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}

			data := struct{ Content template.HTML }{Content: tt.content}
			got, err := e.RenderToString("templates/post.html", data)
			if err != nil {
				t.Fatalf("RenderToString: %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("output %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestRenderToString(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/greeting.html": `{{define "content"}}Hello, {{.Name}}!{{end}}`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	data := struct{ Name string }{Name: "World"}
	got, err := e.RenderToString("templates/greeting.html", data)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	if !strings.Contains(got, "Hello, World!") {
		t.Errorf("output %q does not contain %q", got, "Hello, World!")
	}
}
