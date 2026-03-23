package handlers

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
)

// Sitemap XML structures per sitemaps.org protocol.

type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	XMLNS   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

type URL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

// SitemapHandler serves a sitemap.xml from the content index.
type SitemapHandler struct {
	cfg   *config.Config
	index *content.Index
}

// NewSitemapHandler creates a SitemapHandler.
func NewSitemapHandler(cfg *config.Config, index *content.Index) *SitemapHandler {
	return &SitemapHandler{cfg: cfg, index: index}
}

func (h *SitemapHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	baseURL := h.cfg.Site.BaseURL

	// Derive homepage lastmod from the most recent post.
	homeURL := URL{Loc: baseURL + "/", ChangeFreq: "daily", Priority: "1.0"}
	if len(h.index.Posts) > 0 && !h.index.Posts[0].Date.IsZero() {
		homeURL.LastMod = h.index.Posts[0].Date.UTC().Format("2006-01-02")
	}
	urls := []URL{homeURL}

	for _, p := range h.index.Posts {
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

	for _, p := range h.index.Pages {
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

	writeXML(w, "application/xml; charset=utf-8", sitemap)
}
