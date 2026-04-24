// Package handlers provides HTTP handlers that connect the content index,
// theme engine, and configuration to serve blog pages.
package handlers

import (
	"bytes"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/theme"
)

// Deps holds shared dependencies for all handlers.
// Both the content index and config are stored as atomic pointers so that
// background reloaders can swap them without racing with HTTP handlers.
type Deps struct {
	config atomic.Pointer[config.Config]
	index  atomic.Pointer[content.Index]
	Theme  *theme.Engine

	// Overlay is the layered filesystem (content → theme → defaults).
	// Used by HomeHandler to serve static HTML files.
	Overlay fs.FS

	// staticHTML caches the rendered static homepage content.
	// Cleared on content sync (SetIndex).
	staticHTML atomic.Pointer[[]byte]
}

// NewDeps creates a Deps with the given dependencies.
func NewDeps(cfg *config.Config, idx *content.Index, themeEngine *theme.Engine) *Deps {
	d := &Deps{Theme: themeEngine}
	d.config.Store(cfg)
	d.index.Store(idx)
	return d
}

// LoadConfig returns the current config. Safe for concurrent use.
func (d *Deps) LoadConfig() *config.Config { return d.config.Load() }

// SetConfig atomically replaces the config and clears the cached static
// homepage so a changed homepage path takes effect immediately.
func (d *Deps) SetConfig(cfg *config.Config) {
	d.config.Store(cfg)
	d.staticHTML.Store(nil) // invalidate; homepage path may have changed
}

// LoadIndex returns the current content index. Safe for concurrent use.
func (d *Deps) LoadIndex() *content.Index { return d.index.Load() }

// SetIndex atomically replaces the content index and clears cached
// static homepage content so the next request re-reads from the overlay FS.
func (d *Deps) SetIndex(idx *content.Index) {
	d.index.Store(idx)
	d.staticHTML.Store(nil) // invalidate static homepage cache
}

// PostCount returns the number of posts in the current index.
// Implements server.ContentChecker.
func (d *Deps) PostCount() int {
	if idx := d.index.Load(); idx != nil {
		return len(idx.Posts)
	}
	return 0
}

// PageData is the top-level data passed to all templates.
type PageData struct {
	Site       config.SiteConfig
	Feed       config.FeedConfig
	Post       *content.Post   // single post (post.html)
	Page       *content.Post   // single page (page.html)
	Posts      []*content.Post // post list (list.html)
	Tag        string          // current tag filter
	Title      string          // page title override
	Pagination *Pagination
}

// Pagination holds paging metadata for list views.
type Pagination struct {
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
	PrevPage    int
	NextPage    int
	PrevURL     string
	NextURL     string
}

// HomeHandler returns a handler for the root route that respects the
// site.homepage configuration:
//   - "post_list" or "": paginated post list
//   - "page:<slug>":     renders a content page through the theme
//   - "static:<path>":   serves a raw HTML file from the overlay FS
func HomeHandler(deps *Deps) http.HandlerFunc {
	listFallback := ListHandler(deps)
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.LoadConfig()
		hp := cfg.Site.Homepage
		if hp == "" || hp == "post_list" {
			listFallback(w, r)
			return
		}

		// static:<path> — serve raw HTML from overlay FS, no template wrapping.
		if strings.HasPrefix(hp, "static:") {
			serveStaticHome(w, r, deps, hp, listFallback)
			return
		}

		slug := strings.TrimPrefix(hp, "page:")
		idx := deps.LoadIndex()
		page, ok := idx.PageBySlug[slug]
		if !ok {
			slog.Warn("homepage page not found, falling back to post list",
				"slug", slug, "homepage", hp)
			listFallback(w, r)
			return
		}

		RecordContentView(r, ContentTypeHome, slug, page.Title, page.Tags)

		data := &PageData{
			Site:  cfg.Site,
			Feed:  cfg.Feed,
			Page:  page,
			Title: page.Title,
		}

		renderTemplate(w, r, deps.Theme, "templates/page.html", data, http.StatusOK)
	}
}

// serveStaticHome reads a static HTML file from the overlay FS and writes it
// directly to the response. The content is cached and only re-read when the
// index is replaced (content sync).
func serveStaticHome(w http.ResponseWriter, r *http.Request, deps *Deps, hp string, fallback http.HandlerFunc) {
	if cached := deps.staticHTML.Load(); cached != nil {
		w.Header().Del("Content-Security-Policy")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(*cached)
		return
	}

	filePath := strings.TrimPrefix(hp, "static:")
	if deps.Overlay == nil {
		slog.Warn("static homepage: overlay FS not configured, falling back to post list",
			"path", filePath, "homepage", hp)
		fallback(w, r)
		return
	}

	data, err := fs.ReadFile(deps.Overlay, filePath)
	if err != nil {
		slog.Warn("static homepage file not found, falling back to post list",
			"path", filePath, "homepage", hp, "error", err)
		fallback(w, r)
		return
	}

	deps.staticHTML.Store(&data)

	// Static homepage pages manage their own CSP via <meta> tags or inline
	// styles. Remove the server-set CSP so it does not block inline content.
	w.Header().Del("Content-Security-Policy")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// PostsListHandler returns a handler for /posts (paginated post list).
// This provides a dedicated post-list route that works regardless of
// the homepage configuration.
func PostsListHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.LoadConfig()
		idx := deps.LoadIndex()
		perPage := cfg.Content.PostsPerPage

		page := queryInt(r, "page", 1)
		if page < 1 {
			page = 1
		}

		paged, pag := paginate(idx.Posts, page, perPage)
		setPageURLs(pag, postsPageURL)

		RecordContentView(r, ContentTypePostsList, "", "Posts", nil)

		data := &PageData{
			Site:       cfg.Site,
			Feed:       cfg.Feed,
			Posts:      paged,
			Title:      "Posts",
			Pagination: pag,
		}

		renderTemplate(w, r, deps.Theme, "templates/list.html", data, http.StatusOK)
	}
}

// Route: GET /{$} and GET /page/{page}
func ListHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.LoadConfig()
		idx := deps.LoadIndex()
		perPage := cfg.Content.PostsPerPage

		page, pathBased := parsePage(r)
		if page < 0 {
			NotFoundHandler(deps)(w, r)
			return
		}

		// /page/1 duplicates /; redirect for canonical URLs.
		if pathBased && page == 1 {
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
			return
		}

		// For path-based requests, return 404 for out-of-range pages.
		if pathBased && !validPage(len(idx.Posts), page, perPage) {
			NotFoundHandler(deps)(w, r)
			return
		}

		paged, pag := paginate(idx.Posts, page, perPage)
		setPageURLs(pag, listPageURL)

		RecordContentView(r, ContentTypeList, "", "Posts", nil)

		data := &PageData{
			Site:       cfg.Site,
			Feed:       cfg.Feed,
			Posts:      paged,
			Title:      "Posts",
			Pagination: pag,
		}

		renderTemplate(w, r, deps.Theme, "templates/list.html", data, http.StatusOK)
	}
}

// PostHandler returns a handler for a single post page.
// Route: GET /posts/{slug}
func PostHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		idx := deps.LoadIndex()
		post, ok := idx.BySlug[slug]
		if !ok {
			NotFoundHandler(deps)(w, r)
			return
		}

		RecordContentView(r, ContentTypePost, slug, post.Title, post.Tags)

		cfg := deps.LoadConfig()
		data := &PageData{
			Site:  cfg.Site,
			Feed:  cfg.Feed,
			Post:  post,
			Title: post.Title,
		}

		renderTemplate(w, r, deps.Theme, "templates/post.html", data, http.StatusOK)
	}
}

// PageHandler returns a handler for a static page.
// Route: GET /pages/{slug}
func PageHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		idx := deps.LoadIndex()
		page, ok := idx.PageBySlug[slug]
		if !ok {
			NotFoundHandler(deps)(w, r)
			return
		}

		RecordContentView(r, ContentTypePage, slug, page.Title, page.Tags)

		cfg := deps.LoadConfig()
		data := &PageData{
			Site:  cfg.Site,
			Feed:  cfg.Feed,
			Page:  page,
			Title: page.Title,
		}

		renderTemplate(w, r, deps.Theme, "templates/page.html", data, http.StatusOK)
	}
}

// TagHandler returns a handler for tag-filtered post listings.
// Route: GET /tags/{tag}
func TagHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := r.PathValue("tag")

		idx := deps.LoadIndex()
		posts, ok := idx.ByTag[tag]
		if !ok || len(posts) == 0 {
			NotFoundHandler(deps)(w, r)
			return
		}

		RecordContentView(r, ContentTypeTag, tag, "", nil)

		cfg := deps.LoadConfig()
		page := queryInt(r, "page", 1)
		perPage := cfg.Content.PostsPerPage

		paged, pag := paginate(posts, page, perPage)
		setPageURLs(pag, func(p int) string { return tagPageURL(tag, p) })

		data := &PageData{
			Site:       cfg.Site,
			Feed:       cfg.Feed,
			Posts:      paged,
			Tag:        tag,
			Title:      "Posts tagged \"" + tag + "\"",
			Pagination: pag,
		}

		renderTemplate(w, r, deps.Theme, "templates/list.html", data, http.StatusOK)
	}
}

// NotFoundHandler returns a handler that renders the 404 page.
func NotFoundHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.LoadConfig()
		data := &PageData{
			Site:  cfg.Site,
			Feed:  cfg.Feed,
			Title: "Page Not Found",
		}

		renderTemplate(w, r, deps.Theme, "templates/404.html", data, http.StatusNotFound)
	}
}

// paginate returns a page slice and pagination metadata for the given posts.
func paginate(posts []*content.Post, page, perPage int) ([]*content.Post, *Pagination) {
	// perPage == 0 disables pagination: return all posts on a single page.
	if perPage == 0 {
		return posts, &Pagination{CurrentPage: 1, TotalPages: 1}
	}
	if perPage < 0 {
		perPage = 10
	}
	total := len(posts)
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}
	return posts[start:end], &Pagination{
		CurrentPage: page,
		TotalPages:  totalPages,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}
}

// parsePage extracts the page number from the request. It checks the path
// value first (/page/{page}), then falls back to the ?page= query parameter.
// Returns (-1, true) if a path value is present but invalid.
func parsePage(r *http.Request) (int, bool) {
	if p := r.PathValue("page"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil || v < 1 {
			return -1, true
		}
		return v, true
	}
	return queryInt(r, "page", 1), false
}

// validPage reports whether page is within the valid range for the given
// total post count and per-page size.
func validPage(totalPosts, page, perPage int) bool {
	if perPage <= 0 {
		return page == 1
	}
	totalPages := int(math.Ceil(float64(totalPosts) / float64(perPage)))
	if totalPages < 1 {
		totalPages = 1
	}
	return page >= 1 && page <= totalPages
}

// listPageURL returns the canonical URL for a page in the main post list.
func listPageURL(page int) string {
	if page <= 1 {
		return "/"
	}
	return "/page/" + strconv.Itoa(page)
}

// postsPageURL returns the canonical URL for a page in the /posts list.
func postsPageURL(page int) string {
	if page <= 1 {
		return "/posts"
	}
	return "/posts?page=" + strconv.Itoa(page)
}

// tagPageURL returns the URL for a page in a tag-filtered listing.
func tagPageURL(tag string, page int) string {
	escaped := url.PathEscape(tag)
	if page <= 1 {
		return "/tags/" + escaped
	}
	return "/tags/" + escaped + "?page=" + strconv.Itoa(page)
}

// setPageURLs populates PrevURL/NextURL on a Pagination using the given
// URL-building function.
func setPageURLs(pag *Pagination, urlFunc func(int) string) {
	if pag.HasPrev {
		pag.PrevURL = urlFunc(pag.PrevPage)
	}
	if pag.HasNext {
		pag.NextURL = urlFunc(pag.NextPage)
	}
}

// renderTemplate renders the named template into a buffer, then writes
// the response. If rendering fails the client receives a 500 error
// without any partial content.
func renderTemplate(w http.ResponseWriter, r *http.Request, engine *theme.Engine, name string, data *PageData, statusCode int) {
	var buf bytes.Buffer
	if err := engine.Render(r.Context(), &buf, name, data); err != nil {
		if r.Context().Err() != nil {
			slog.Debug("render aborted: client disconnected", "template", name, "error", err)
			return
		}
		slog.Error("template render failed", "template", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = buf.WriteTo(w)
}

// queryInt reads an integer query parameter with a default value.
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
