package handlers_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
	"github.com/kenhaines/blogflow/internal/server/handlers"
	"github.com/kenhaines/blogflow/internal/theme"
)

// testDeps builds a Deps with a minimal theme engine and test content.
func testDeps(t *testing.T, posts []*content.Post, pages []*content.Post) *handlers.Deps {
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
		"templates/list.html": &fstest.MapFile{
			Data: []byte(`{{.Title}}|posts={{len .Posts}}|page={{.Pagination.CurrentPage}}|total={{.Pagination.TotalPages}}`),
		},
		"templates/post.html": &fstest.MapFile{
			Data: []byte(`post:{{.Post.Slug}}|{{.Title}}`),
		},
		"templates/page.html": &fstest.MapFile{
			Data: []byte(`page:{{.Page.Slug}}|{{.Title}}`),
		},
		"templates/404.html": &fstest.MapFile{
			Data: []byte(`404:{{.Title}}`),
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
		Content: template.HTML("<p>" + title + "</p>"),
	}
}

func makePage(slug, title string) *content.Post {
	return &content.Post{
		FrontMatter: content.FrontMatter{
			Title: title,
		},
		Slug:    slug,
		Content: template.HTML("<p>" + title + "</p>"),
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
