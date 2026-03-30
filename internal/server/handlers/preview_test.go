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

func previewDeps(t *testing.T, token string) *handlers.Deps {
	t.Helper()

	published := &content.Post{
		FrontMatter: content.FrontMatter{
			Title: "Published Post",
			Tags:  []string{"go"},
			Date:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		Slug:    "published",
		Content: template.HTML("<p>Published content</p>"), //nolint:gosec
	}
	draft := &content.Post{
		FrontMatter: content.FrontMatter{
			Title: "Draft Post",
			Draft: true,
			Tags:  []string{"go"},
			Date:  time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
		},
		Slug:    "my-draft",
		Content: template.HTML("<p>Draft content</p>"), //nolint:gosec
	}

	idx := &content.Index{
		Posts:       []*content.Post{published},
		Drafts:      []*content.Post{draft},
		BySlug:      map[string]*content.Post{"published": published},
		DraftBySlug: map[string]*content.Post{"my-draft": draft},
		ByTag:       map[string][]*content.Post{"go": {published}},
		ByYear:      map[int][]*content.Post{2025: {published}},
		Pages:       nil,
		PageBySlug:  make(map[string]*content.Post),
	}

	tmplFS := fstest.MapFS{
		"templates/base.html": &fstest.MapFile{
			Data: []byte(`{{block "content" .}}{{end}}`),
		},
		"templates/list.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}preview={{.Preview}}|posts={{len .Posts}}|{{range .Posts}}{{.Slug}}:draft={{.Draft}},{{end}}{{end}}`),
		},
		"templates/post.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}preview={{.Preview}}|post:{{.Post.Slug}}|draft={{.Post.Draft}}{{end}}`),
		},
		"templates/404.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}404{{end}}`),
		},
	}
	eng, err := theme.NewEngine(tmplFS)
	if err != nil {
		t.Fatalf("creating theme engine: %v", err)
	}

	cfg := config.Default()
	cfg.Site.PreviewToken = token
	cfg.Content.PostsPerPage = 10

	return handlers.NewDeps(cfg, idx, eng)
}

// wrapWithPreview applies the preview middleware to a handler for testing.
func wrapWithPreview(deps *handlers.Deps, h http.HandlerFunc) http.Handler {
	return handlers.PreviewMiddleware(deps)(h)
}

func TestPreview_DraftsHiddenByDefault(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=1") {
		t.Errorf("expected 1 post without preview, got body: %s", body)
	}
	if strings.Contains(body, "my-draft") {
		t.Errorf("draft should be hidden without token, got body: %s", body)
	}
	if !strings.Contains(body, "preview=false") {
		t.Errorf("expected preview=false, got body: %s", body)
	}
}

func TestPreview_DraftsVisibleWithCorrectToken(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/?preview=true&token=secret-token", nil)
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=2") {
		t.Errorf("expected 2 posts (published+draft) in preview, got body: %s", body)
	}
	if !strings.Contains(body, "my-draft") {
		t.Errorf("expected draft post in preview mode, got body: %s", body)
	}
	if !strings.Contains(body, "preview=true") {
		t.Errorf("expected preview=true, got body: %s", body)
	}
}

func TestPreview_InvalidTokenHidesDrafts(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/?preview=true&token=wrong-token", nil)
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=1") {
		t.Errorf("expected 1 post with wrong token, got body: %s", body)
	}
	if strings.Contains(body, "my-draft") {
		t.Errorf("draft should be hidden with wrong token, got body: %s", body)
	}
}

func TestPreview_BearerToken(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "posts=2") {
		t.Errorf("expected 2 posts with Bearer token, got body: %s", body)
	}
	if !strings.Contains(body, "preview=true") {
		t.Errorf("expected preview=true with Bearer token, got body: %s", body)
	}
}

func TestPreview_PostHandler_DraftVisible(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/posts/my-draft?preview=true&token=secret-token", nil)
	req.SetPathValue("slug", "my-draft")
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.PostHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "post:my-draft") {
		t.Errorf("expected draft post, got body: %s", body)
	}
	if !strings.Contains(body, "draft=true") {
		t.Errorf("expected draft=true, got body: %s", body)
	}
}

func TestPreview_PostHandler_DraftHiddenWithoutToken(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/posts/my-draft", nil)
	req.SetPathValue("slug", "my-draft")
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.PostHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for draft without token, got %d", rec.Code)
	}
}

func TestPreview_EmptyTokenDisablesPreview(t *testing.T) {
	deps := previewDeps(t, "")

	req := httptest.NewRequest(http.MethodGet, "/?preview=true&token=anything", nil)
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "posts=1") {
		t.Errorf("expected 1 post when token is empty (preview disabled), got body: %s", body)
	}
	if !strings.Contains(body, "preview=false") {
		t.Errorf("expected preview=false when token is empty, got body: %s", body)
	}
}

func TestPreview_PreviewFlagNotSetOnQueryOnly(t *testing.T) {
	deps := previewDeps(t, "secret-token")

	// preview=true but no token provided
	req := httptest.NewRequest(http.MethodGet, "/?preview=true", nil)
	rec := httptest.NewRecorder()
	wrapWithPreview(deps, handlers.ListHandler(deps)).ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "preview=false") {
		t.Errorf("expected preview=false without token, got body: %s", body)
	}
}
