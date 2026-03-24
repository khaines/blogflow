package handlers

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Sitemap XML structures per sitemaps.org protocol.

// URLSet represents the root element of a sitemap.
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	XMLNS   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

// URL represents a URL entry in a sitemap.
type URL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

// SitemapHandler serves a sitemap.xml from the content index.
type SitemapHandler struct {
	deps *Deps
}

// NewSitemapHandler creates a SitemapHandler backed by the shared Deps.
func NewSitemapHandler(deps *Deps) *SitemapHandler {
	return &SitemapHandler{deps: deps}
}

func (h *SitemapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := h.deps.LoadConfig()
	idx := h.deps.LoadIndex()
	baseURL := cfg.Site.BaseURL

	// Derive homepage lastmod from the most recent post.
	homeURL := URL{Loc: baseURL + "/", ChangeFreq: "daily", Priority: "1.0"}
	if len(idx.Posts) > 0 && !idx.Posts[0].Date.IsZero() {
		homeURL.LastMod = idx.Posts[0].Date.UTC().Format("2006-01-02")
	}
	urls := []URL{homeURL}

	for _, p := range idx.Posts {
		u := URL{
			Loc:        fmt.Sprintf("%s/posts/%s", baseURL, p.Slug),
			ChangeFreq: "weekly",
			Priority:   "0.8",
		}
		if !p.Date.IsZero() {
			u.LastMod = p.Date.UTC().Format("2006-01-02")
		}
		urls = append(urls, u)
	}

	for _, p := range idx.Pages {
		u := URL{
			Loc:        fmt.Sprintf("%s/%s", baseURL, p.Slug),
			ChangeFreq: "monthly",
			Priority:   "0.6",
		}
		if !p.Date.IsZero() {
			u.LastMod = p.Date.UTC().Format("2006-01-02")
		}
		urls = append(urls, u)
	}

	const maxURLs = 50_000
	if len(urls) > maxURLs {
		slog.Warn("sitemap truncated", "total", len(urls), "max", maxURLs)
		urls = urls[:maxURLs]
	}

	sitemap := URLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	var lastMod time.Time
	for _, p := range idx.Posts {
		if p.Date.After(lastMod) {
			lastMod = p.Date
		}
	}
	for _, p := range idx.Pages {
		if p.Date.After(lastMod) {
			lastMod = p.Date
		}
	}

	writeXMLCached(w, r, "application/xml; charset=utf-8", sitemap, lastMod.UTC())
}
