// Package handlers provides HTTP handlers that connect the content index,
// theme engine, and configuration to serve blog pages.
package handlers

import (
	"bytes"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
	"github.com/kenhaines/blogflow/internal/theme"
)

// Deps holds shared dependencies for all handlers.
type Deps struct {
	Config *config.Config
	Index  *content.Index
	Theme  *theme.Engine
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
}

// ListHandler returns a handler for the home page (paginated post list).
// Route: GET /{$}
func ListHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := queryInt(r, "page", 1)
		perPage := deps.Config.Content.PostsPerPage

		paged, pag := paginate(deps.Index.Posts, page, perPage)

		data := &PageData{
			Site:       deps.Config.Site,
			Feed:       deps.Config.Feed,
			Posts:      paged,
			Title:      deps.Config.Site.Title,
			Pagination: pag,
		}

		renderTemplate(w, deps.Theme, "templates/list.html", data, http.StatusOK)
	}
}

// PostHandler returns a handler for a single post page.
// Route: GET /posts/{slug}
func PostHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		post, ok := deps.Index.BySlug[slug]
		if !ok {
			NotFoundHandler(deps)(w, r)
			return
		}

		data := &PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Post:  post,
			Title: post.Title,
		}

		renderTemplate(w, deps.Theme, "templates/post.html", data, http.StatusOK)
	}
}

// PageHandler returns a handler for a static page.
// Route: GET /pages/{slug}
func PageHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		page, ok := deps.Index.PageBySlug[slug]
		if !ok {
			NotFoundHandler(deps)(w, r)
			return
		}

		data := &PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Page:  page,
			Title: page.Title,
		}

		renderTemplate(w, deps.Theme, "templates/page.html", data, http.StatusOK)
	}
}

// TagHandler returns a handler for tag-filtered post listings.
// Route: GET /tags/{tag}
func TagHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := r.PathValue("tag")

		posts, ok := deps.Index.ByTag[tag]
		if !ok || len(posts) == 0 {
			NotFoundHandler(deps)(w, r)
			return
		}

		page := queryInt(r, "page", 1)
		perPage := deps.Config.Content.PostsPerPage

		paged, pag := paginate(posts, page, perPage)

		data := &PageData{
			Site:       deps.Config.Site,
			Feed:       deps.Config.Feed,
			Posts:      paged,
			Tag:        tag,
			Title:      "Posts tagged \"" + tag + "\"",
			Pagination: pag,
		}

		renderTemplate(w, deps.Theme, "templates/list.html", data, http.StatusOK)
	}
}

// NotFoundHandler returns a handler that renders the 404 page.
func NotFoundHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := &PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Title: "Page Not Found",
		}

		renderTemplate(w, deps.Theme, "templates/404.html", data, http.StatusNotFound)
	}
}

// paginate returns a page slice and pagination metadata for the given posts.
func paginate(posts []*content.Post, page, perPage int) ([]*content.Post, *Pagination) {
	if perPage <= 0 {
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

// renderTemplate renders the named template into a buffer, then writes
// the response. If rendering fails the client receives a 500 error
// without any partial content.
func renderTemplate(w http.ResponseWriter, engine *theme.Engine, name string, data *PageData, statusCode int) {
	var buf bytes.Buffer
	if err := engine.Render(&buf, name, data); err != nil {
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
