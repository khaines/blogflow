package handlers_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// testDeps builds a Deps with a minimal theme engine and test content.
func testDeps(t *testing.T, posts, pages []*content.Post) *handlers.Deps {
	t.Helper()

	// Build index from provided posts/pages.
	idx := &content.Index{
		Posts:      posts,
		BySlug:     make(map[string]*content.Post),
		ByTag:      make(map[string][]*content.Post),
		ByYear:     make(map[int][]*content.Post),
		Pages:      pages,
		PageBySlug: make(map[string]*content.Post),
	}
	for _, p := range posts {
		idx.BySlug[p.Slug] = p
		for _, tag := range p.Tags {
			idx.ByTag[tag] = append(idx.ByTag[tag], p)
		}
	}
	for _, p := range pages {
		idx.PageBySlug[p.Slug] = p
	}

	// Minimal templates that render enough to assert on.
	tmplFS := fstest.MapFS{
		"templates/base.html": &fstest.MapFile{
			Data: []byte(`{{block "content" .}}{{end}}`),
		},
		"templates/list.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}{{.Title}}|posts={{len .Posts}}|page={{.Pagination.CurrentPage}}|total={{.Pagination.TotalPages}}{{end}}`),
		},
		"templates/post.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}post:{{.Post.Slug}}|{{.Title}}{{end}}`),
		},
		"templates/page.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}page:{{.Page.Slug}}|{{.Title}}{{end}}`),
		},
		"templates/404.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}404:{{.Title}}{{end}}`),
		},
	}
	eng, err := theme.NewEngine(tmplFS)
	if err != nil {
		t.Fatalf("creating theme engine: %v", err)
	}

	cfg := config.Default()
	cfg.Content.PostsPerPage = 2

	return &handlers.Deps{
		Config: cfg,
		Index:  idx,
		Theme:  eng,
	}
}

func makePost(slug, title string, tags []string) *content.Post {
	return &content.Post{
		FrontMatter: content.FrontMatter{
			Title: title,
			Tags:  tags,
			Date:  time.Now(),
		},
		Slug:    slug,
		Content: template.HTML("<p>" + title + "</p>"), //nolint:gosec // test data,
	}
}

func makePage(slug, title string) *content.Post {
	return &content.Post{
		FrontMatter: content.FrontMatter{
			Title: title,
		},
		Slug:    slug,
		Content: template.HTML("<p>" + title + "</p>"), //nolint:gosec // test data,
	}
}

func TestListHandler(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
		makePost("b", "Beta", nil),
		makePost("c", "Gamma", nil),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=2") {
		t.Errorf("expected 2 posts on page 1, got body: %s", body)
	}
	if !strings.Contains(body, "page=1") {
		t.Errorf("expected page=1, got body: %s", body)
	}
	if !strings.Contains(body, "total=2") {
		t.Errorf("expected total=2, got body: %s", body)
	}
}

func TestListHandler_Pagination(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
		makePost("b", "Beta", nil),
		makePost("c", "Gamma", nil),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/?page=2", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Page 2 with 2 per page should have 1 post.
	if !strings.Contains(body, "posts=1") {
		t.Errorf("expected 1 post on page 2, got body: %s", body)
	}
	if !strings.Contains(body, "page=2") {
		t.Errorf("expected page=2, got body: %s", body)
	}
}

func TestPostHandler(t *testing.T) {
	posts := []*content.Post{makePost("hello", "Hello World", nil)}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/posts/hello", nil)
	req.SetPathValue("slug", "hello")
	rec := httptest.NewRecorder()
	handlers.PostHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "post:hello") {
		t.Errorf("expected post:hello, got body: %s", body)
	}
	if !strings.Contains(body, "Hello World") {
		t.Errorf("expected Hello World, got body: %s", body)
	}
}

func TestPostHandler_NotFound(t *testing.T) {
	deps := testDeps(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/posts/nope", nil)
	req.SetPathValue("slug", "nope")
	rec := httptest.NewRecorder()
	handlers.PostHandler(deps)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "404:") {
		t.Errorf("expected 404 template, got body: %s", rec.Body.String())
	}
}

func TestPageHandler(t *testing.T) {
	pages := []*content.Post{makePage("about", "About Me")}
	deps := testDeps(t, nil, pages)

	req := httptest.NewRequest(http.MethodGet, "/pages/about", nil)
	req.SetPathValue("slug", "about")
	rec := httptest.NewRecorder()
	handlers.PageHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "page:about") {
		t.Errorf("expected page:about, got body: %s", body)
	}
	if !strings.Contains(body, "About Me") {
		t.Errorf("expected About Me, got body: %s", body)
	}
}

func TestPageHandler_NotFound(t *testing.T) {
	deps := testDeps(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/pages/nope", nil)
	req.SetPathValue("slug", "nope")
	rec := httptest.NewRecorder()
	handlers.PageHandler(deps)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "404:") {
		t.Errorf("expected 404 template, got body: %s", rec.Body.String())
	}
}

func TestTagHandler(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", []string{"go"}),
		makePost("b", "Beta", []string{"go", "web"}),
		makePost("c", "Gamma", []string{"web"}),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/tags/go", nil)
	req.SetPathValue("tag", "go")
	rec := httptest.NewRecorder()
	handlers.TagHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=2") {
		t.Errorf("expected 2 posts for tag 'go', got body: %s", body)
	}
	if !strings.Contains(body, `Posts tagged`) {
		t.Errorf("expected tag title, got body: %s", body)
	}
}

func TestTagHandler_NotFound(t *testing.T) {
	deps := testDeps(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/tags/nope", nil)
	req.SetPathValue("tag", "nope")
	rec := httptest.NewRecorder()
	handlers.TagHandler(deps)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "404:") {
		t.Errorf("expected 404 template, got body: %s", rec.Body.String())
	}
}

func TestListHandler_PageZero(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
		makePost("b", "Beta", nil),
		makePost("c", "Gamma", nil),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/?page=0", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "page=1") {
		t.Errorf("expected page=1 (clamped from 0), got body: %s", body)
	}
}

func TestListHandler_PageBeyondTotal(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
		makePost("b", "Beta", nil),
		makePost("c", "Gamma", nil),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/?page=999", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// 3 posts, 2 per page → totalPages=2; page should clamp to 2.
	if !strings.Contains(body, "page=2") {
		t.Errorf("expected page=2 (clamped from 999), got body: %s", body)
	}
}

func TestListHandler_PageNonInteger(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", nil),
	}
	deps := testDeps(t, posts, nil)

	req := httptest.NewRequest(http.MethodGet, "/?page=abc", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "page=1") {
		t.Errorf("expected page=1 (default for non-integer), got body: %s", body)
	}
}

func TestListHandler_EmptyIndex(t *testing.T) {
	deps := testDeps(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=0") {
		t.Errorf("expected posts=0, got body: %s", body)
	}
	if !strings.Contains(body, "page=1") {
		t.Errorf("expected page=1, got body: %s", body)
	}
	if !strings.Contains(body, "total=1") {
		t.Errorf("expected total=1, got body: %s", body)
	}
}

func TestNotFoundHandler(t *testing.T) {
	deps := testDeps(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	handlers.NotFoundHandler(deps)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "404:Page Not Found") {
		t.Errorf("expected 404:Page Not Found, got body: %s", body)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got: %s", ct)
	}
}

func TestRenderTemplate_500(t *testing.T) {
	// Engine with base.html but NO page templates — Render returns an error
	// for every page template, exercising the 500 path in renderTemplate.
	tmplFS := fstest.MapFS{
		"templates/base.html": &fstest.MapFile{
			Data: []byte(`{{block "content" .}}{{end}}`),
		},
	}
	eng, err := theme.NewEngine(tmplFS)
	if err != nil {
		t.Fatalf("creating theme engine: %v", err)
	}

	cfg := config.Default()
	cfg.Content.PostsPerPage = 2

	posts := []*content.Post{makePost("hello", "Hello", []string{"go"})}
	pages := []*content.Post{makePage("about", "About")}

	idx := &content.Index{
		Posts:      posts,
		BySlug:     map[string]*content.Post{"hello": posts[0]},
		ByTag:      map[string][]*content.Post{"go": posts},
		ByYear:     make(map[int][]*content.Post),
		Pages:      pages,
		PageBySlug: map[string]*content.Post{"about": pages[0]},
	}

	deps := &handlers.Deps{
		Config: cfg,
		Index:  idx,
		Theme:  eng,
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		setup   func(r *http.Request)
	}{
		{
			name:    "ListHandler",
			handler: handlers.ListHandler(deps),
			path:    "/",
		},
		{
			name:    "PostHandler",
			handler: handlers.PostHandler(deps),
			path:    "/posts/hello",
			setup:   func(r *http.Request) { r.SetPathValue("slug", "hello") },
		},
		{
			name:    "PageHandler",
			handler: handlers.PageHandler(deps),
			path:    "/pages/about",
			setup:   func(r *http.Request) { r.SetPathValue("slug", "about") },
		},
		{
			name:    "TagHandler",
			handler: handlers.TagHandler(deps),
			path:    "/tags/go",
			setup:   func(r *http.Request) { r.SetPathValue("tag", "go") },
		},
		{
			name:    "NotFoundHandler",
			handler: handlers.NotFoundHandler(deps),
			path:    "/nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.setup != nil {
				tc.setup(req)
			}
			rec := httptest.NewRecorder()
			tc.handler(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Internal Server Error") {
				t.Errorf("expected 'Internal Server Error' body, got: %s", body)
			}
			// Verify no partial template content leaked (buffered rendering).
			if strings.Contains(body, "posts=") || strings.Contains(body, "post:") ||
				strings.Contains(body, "page:") || strings.Contains(body, "404:") {
				t.Errorf("partial template content leaked into 500 response: %s", body)
			}
		})
	}
}

func TestTagHandler_Paginated(t *testing.T) {
	posts := []*content.Post{
		makePost("a", "Alpha", []string{"go"}),
		makePost("b", "Beta", []string{"go"}),
		makePost("c", "Gamma", []string{"go"}),
	}
	deps := testDeps(t, posts, nil)

	// Page 2: 3 posts, 2 per page → page 2 has 1 post.
	req := httptest.NewRequest(http.MethodGet, "/tags/go?page=2", nil)
	req.SetPathValue("tag", "go")
	rec := httptest.NewRecorder()
	handlers.TagHandler(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=1") {
		t.Errorf("expected 1 post on tag page 2, got body: %s", body)
	}
	if !strings.Contains(body, "page=2") {
		t.Errorf("expected page=2, got body: %s", body)
	}
	if !strings.Contains(body, "total=2") {
		t.Errorf("expected total=2, got body: %s", body)
	}
}
