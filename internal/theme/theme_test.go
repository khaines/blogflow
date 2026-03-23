package theme

import (
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func testFS(files map[string]string) fstest.MapFS {
	m := make(fstest.MapFS)
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func TestNewEngine_LoadsTemplates(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/index.html": `<h1>Hello</h1>`,
	})

	e, err := NewEngine(fs)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	var b strings.Builder
	if err := e.Render(&b, "templates/index.html", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := b.String(); got != "<h1>Hello</h1>" {
		t.Errorf("got %q, want %q", got, "<h1>Hello</h1>")
	}
}

func TestRender_WithData(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/post.html": `<h1>{{.Title}}</h1><p>by {{.Author}}</p>`,
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
	if got := b.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
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
			template: `{{formatDate .Date "2006-01-02"}}`,
			data:     struct{ Date time.Time }{Date: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
			want:     "2025-03-15",
		},
		{
			name:     "lower",
			template: `{{lower .}}`,
			data:     "HELLO",
			want:     "hello",
		},
		{
			name:     "upper",
			template: `{{upper .}}`,
			data:     "hello",
			want:     "HELLO",
		},
		{
			name:     "truncate_short",
			template: `{{truncate . 10}}`,
			data:     "short",
			want:     "short",
		},
		{
			name:     "truncate_long_word_boundary",
			template: `{{truncate . 20}}`,
			data:     "This is a long sentence that should be truncated",
			want:     "This is a long…",
		},
		{
			name:     "truncate_long_no_space",
			template: `{{truncate . 5}}`,
			data:     "abcdefghij",
			want:     "abcde…",
		},
		{
			name:     "urlize",
			template: `{{urlize .}}`,
			data:     "Hello World! 123",
			want:     "hello-world-123",
		},
		{
			name:     "add",
			template: `{{add 2 3}}`,
			data:     nil,
			want:     "5",
		},
		{
			name:     "sub",
			template: `{{sub 10 3}}`,
			data:     nil,
			want:     "7",
		},
		{
			name:     "seq",
			template: `{{range seq 1 3}}{{.}} {{end}}`,
			data:     nil,
			want:     "1 2 3 ",
		},
		{
			name:     "readingTime_short",
			template: `{{readingTime .}}`,
			data:     "A few words",
			want:     "1",
		},
		{
			name:     "readingTime_long",
			template: `{{readingTime .}}`,
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
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_Partials(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/partials/header.html": `<header>{{.SiteName}}</header>`,
		"templates/page.html":            `{{template "templates/partials/header.html" .}}<main>content</main>`,
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
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_MissingTemplate(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/index.html": `<h1>Hello</h1>`,
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
		// When no {{define}} overrides the block, the default content is used.
		fs := testFS(map[string]string{
			"templates/base.html": `<html>{{block "content" .}}default{{end}}</html>`,
		})

		e, err := NewEngine(fs)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		got, err := e.RenderToString("templates/base.html", nil)
		if err != nil {
			t.Fatalf("RenderToString: %v", err)
		}
		if got != "<html>default</html>" {
			t.Errorf("got %q, want %q", got, "<html>default</html>")
		}
	})

	t.Run("overridden_block", func(t *testing.T) {
		// A {{define}} in another template overrides the block default.
		fs := testFS(map[string]string{
			"templates/base.html": `<html>{{block "content" .}}default{{end}}</html>`,
			"templates/home.html": `{{define "content"}}home page{{end}}`,
		})

		e, err := NewEngine(fs)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		got, err := e.RenderToString("templates/base.html", nil)
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
		"templates/index.html": &fstest.MapFile{Data: []byte(`v1`)},
	}

	e, err := NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	got, err := e.RenderToString("templates/index.html", nil)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	if got != "v1" {
		t.Errorf("before reload: got %q, want %q", got, "v1")
	}

	// Update the file in the MapFS.
	m["templates/index.html"] = &fstest.MapFile{Data: []byte(`v2`)}

	if err := e.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got, err = e.RenderToString("templates/index.html", nil)
	if err != nil {
		t.Fatalf("RenderToString after reload: %v", err)
	}
	if got != "v2" {
		t.Errorf("after reload: got %q, want %q", got, "v2")
	}
}

func TestReload_ConcurrentWithRender(t *testing.T) {
	m := fstest.MapFS{"templates/t.html": &fstest.MapFile{Data: []byte("v1")}}
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
	fs := testFS(map[string]string{
		"static/main.css": `body {}`,
	})

	_, err := NewEngine(fs)
	if err == nil {
		t.Fatal("expected error when templates/ dir is missing, got nil")
	}
}

func TestRender_HTMLEscaping(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/page.html": `<p>{{.Content}}</p>`,
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
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderToString(t *testing.T) {
	fs := testFS(map[string]string{
		"templates/greeting.html": `Hello, {{.Name}}!`,
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
	if got != "Hello, World!" {
		t.Errorf("got %q, want %q", got, "Hello, World!")
	}
}
