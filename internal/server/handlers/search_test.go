package handlers_test

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	blogflow "github.com/khaines/blogflow"
	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/search"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// mkSearchPost builds a post with an explicit rendered-HTML body for search.
func mkSearchPost(slug, title string, tags []string, day int, body string) *content.Post {
	p := &content.Post{
		Slug:    slug,
		Content: template.HTML(body), //nolint:gosec // test data
	}
	p.Title = title
	p.Tags = tags
	p.Date = time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC)
	return p
}

func searchIndex(posts ...*content.Post) *content.Index {
	idx := &content.Index{
		BySlug:     make(map[string]*content.Post),
		ByTag:      make(map[string][]*content.Post),
		ByYear:     make(map[int][]*content.Post),
		PageBySlug: make(map[string]*content.Post),
	}
	idx.Posts = posts
	for _, p := range posts {
		idx.BySlug[p.Slug] = p
	}
	return idx
}

// searchDeps builds Deps backed by the real embedded default theme so tests
// exercise the shipped search.html and partials. When buildIndex is false, the
// snapshot is published with a nil search index to exercise the 503 path.
func searchDeps(t *testing.T, buildIndex bool, posts ...*content.Post) *handlers.Deps {
	t.Helper()
	eng, err := theme.NewEngine(blogflow.Defaults)
	if err != nil {
		t.Fatalf("theme engine: %v", err)
	}
	cfg := config.Default()
	cfg.Search.Enabled = true
	cfg.Search.MinQueryLength = 2
	cfg.Search.MaxResults = 2 // small page for pagination tests

	idx := searchIndex(posts...)
	deps := handlers.NewDeps(cfg, idx, eng)
	deps.SetSearchEnabled(true)
	if buildIndex {
		si := search.Build(context.Background(), idx, cfg.Search, 1)
		deps.SetSnapshot(1, idx, si)
	} else {
		deps.SetSnapshot(1, idx, nil) // enabled but index unavailable
	}
	return deps
}

func doSearch(t *testing.T, deps *handlers.Deps, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	handlers.SearchHandler(deps)(rec, req)
	return rec
}

func sampleCorpus() []*content.Post {
	return []*content.Post{
		mkSearchPost("go-internals", "Go internals", []string{"go"}, 2, "<p>A deep dive into Go internals and the runtime.</p>"),
		mkSearchPost("overlay-fs", "Overlay filesystem", []string{"architecture"}, 3, "<p>The overlay filesystem merges layers.</p>"),
		mkSearchPost("caching", "Caching strategies", []string{"go"}, 1, "<p>Caching improves throughput significantly.</p>"),
	}
}

func TestSearchHandler_EmptyQueryRendersForm(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	rec := doSearch(t, deps, "/search")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `role="search"`) {
		t.Error("landing form missing role=\"search\"")
	}
	if !strings.Contains(body, `name="q"`) {
		t.Error("landing form missing query input")
	}
	if strings.Contains(body, "search-results") {
		t.Error("landing form should not show a results list")
	}
}

func TestSearchHandler_BelowMinLength(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	rec := doSearch(t, deps, "/search?q=a")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `role="alert"`) {
		t.Error("below-min query should render an accessible validation alert")
	}
	if !strings.Contains(body, "at least 2 characters") {
		t.Errorf("missing min-length message: %s", body)
	}
}

func TestSearchHandler_OverMaxLength(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	long := strings.Repeat("x", 200)
	rec := doSearch(t, deps, "/search?q="+long)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `role="alert"`) {
		t.Error("over-length query should render a validation alert")
	}
}

func TestSearchHandler_ResultsAndAccessibility(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	rec := doSearch(t, deps, "/search?q=overlay")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Overlay filesystem") {
		t.Error("expected matching post title in results")
	}
	if !strings.Contains(body, `role="status"`) {
		t.Error("result count should be exposed via role=\"status\"")
	}
	if !strings.Contains(body, `href="/posts/overlay-fs"`) {
		t.Error("result should link to the post")
	}
}

// TestSearchHandler_ScoreNotInBody verifies the maintainer-approved refinement:
// numeric relevance scores are not shown in the default reader-visible body.
func TestSearchHandler_ScoreNotInBody(t *testing.T) {
	posts := sampleCorpus()
	deps := searchDeps(t, true, posts...)
	// Compute the top result's full-precision score independently.
	idx := searchIndex(posts...)
	si := search.Build(context.Background(), idx, func() config.SearchConfig {
		c := config.Default().Search
		c.Enabled = true
		return c
	}(), 1)
	resp, err := si.Search(context.Background(), idx, "go", search.SearchOptions{Limit: 20})
	if err != nil || len(resp.Results) == 0 {
		t.Fatalf("setup search failed: %v", err)
	}
	scoreStr := fmt.Sprintf("%.6f", resp.Results[0].Score)

	rec := doSearch(t, deps, "/search?q=go")
	if strings.Contains(rec.Body.String(), scoreStr) {
		t.Errorf("numeric score %q leaked into default result body", scoreStr)
	}
}

func TestSearchHandler_NoResults(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	rec := doSearch(t, deps, "/search?q=nonexistentterm")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No results") {
		t.Error("expected explicit no-results state")
	}
}

func TestSearchHandler_MaliciousQueryEscaped(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	rec := doSearch(t, deps, "/search?q=%3Cscript%3Ealert(1)%3C%2Fscript%3Ego")
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("query echo was not escaped — potential XSS")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped query echo")
	}
}

func TestSearchHandler_IndexUnavailable503(t *testing.T) {
	deps := searchDeps(t, false, sampleCorpus()...) // enabled but nil search index
	rec := doSearch(t, deps, "/search?q=go")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "temporarily unavailable") {
		t.Error("expected friendly unavailable message")
	}
}

func TestSearchHandler_PaginationLinks(t *testing.T) {
	// 5 posts all matching "go", page size 2 -> 3 pages with nav links.
	posts := []*content.Post{}
	for i := 1; i <= 5; i++ {
		posts = append(posts, mkSearchPost(fmt.Sprintf("p%d", i), fmt.Sprintf("Go post %d", i), []string{"go"}, i, "<p>go content</p>"))
	}
	deps := searchDeps(t, true, posts...)
	rec := doSearch(t, deps, "/search?q=go&page=2")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(body, `class="pagination search-pagination"`) {
		t.Error("expected search pagination nav")
	}
	if !strings.Contains(body, `rel="prev"`) || !strings.Contains(body, `rel="next"`) {
		t.Errorf("page 2 of 3 should have prev and next links: %s", body)
	}
}

func TestSearchHandler_PageOverflowExposesLastPage(t *testing.T) {
	posts := []*content.Post{}
	for i := 1; i <= 3; i++ {
		posts = append(posts, mkSearchPost(fmt.Sprintf("p%d", i), fmt.Sprintf("Go post %d", i), []string{"go"}, i, "<p>go content</p>"))
	}
	deps := searchDeps(t, true, posts...) // page size 2 -> 2 pages total
	rec := doSearch(t, deps, "/search?q=go&page=9")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Overflow page has no results but must expose navigation back to results.
	if !strings.Contains(body, `rel="prev"`) {
		t.Errorf("overflow page should expose a previous link: %s", body)
	}
}
