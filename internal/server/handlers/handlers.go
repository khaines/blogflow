// Package handlers provides HTTP handlers that connect the content index,
// theme engine, and configuration to serve blog pages.
package handlers

import (
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
		if page < 1 {
			page = 1
		}

		posts := deps.Index.Posts
		perPage := deps.Config.Content.PostsPerPage
		if perPage <= 0 {
			perPage = 10
		}

		totalPages := int(math.Ceil(float64(len(posts)) / float64(perPage)))
		if totalPages < 1 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		start := (page - 1) * perPage
		end := start + perPage
		if end > len(posts) {
			end = len(posts)
		}

		data := PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Posts: posts[start:end],
			Title: deps.Config.Site.Title,
			Pagination: &Pagination{
				CurrentPage: page,
				TotalPages:  totalPages,
				HasPrev:     page > 1,
				HasNext:     page < totalPages,
				PrevPage:    page - 1,
				NextPage:    page + 1,
			},
		}

		if err := deps.Theme.Render(w, "templates/list.html", data); err != nil {
			slog.Error("rendering list", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
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

		data := PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Post:  post,
			Title: post.Title,
		}

		if err := deps.Theme.Render(w, "templates/post.html", data); err != nil {
			slog.Error("rendering post", "slug", slug, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
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

		data := PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Page:  page,
			Title: page.Title,
		}

		if err := deps.Theme.Render(w, "templates/page.html", data); err != nil {
			slog.Error("rendering page", "slug", slug, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
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

		data := PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Posts: posts,
			Tag:   tag,
			Title: "Posts tagged \"" + tag + "\"",
			Pagination: &Pagination{
				CurrentPage: 1,
				TotalPages:  1,
				HasPrev:     false,
				HasNext:     false,
				PrevPage:    0,
				NextPage:    0,
			},
		}

		if err := deps.Theme.Render(w, "templates/list.html", data); err != nil {
			slog.Error("rendering tag", "tag", tag, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// NotFoundHandler returns a handler that renders the 404 page.
func NotFoundHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)

		data := PageData{
			Site:  deps.Config.Site,
			Feed:  deps.Config.Feed,
			Title: "Page Not Found",
		}

		if err := deps.Theme.Render(w, "templates/404.html", data); err != nil {
			slog.Error("rendering 404", "error", err)
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}
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
