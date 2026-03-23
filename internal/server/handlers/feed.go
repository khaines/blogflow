// Package handlers provides HTTP handlers for BlogFlow content routes, feeds, and sitemaps.
package handlers

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
)

// Atom XML structures.

// AtomFeed represents an Atom 1.0 feed document.
type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	Links   []AtomLink  `xml:"link"`
	Updated string      `xml:"updated"`
	Author  AtomAuthor  `xml:"author"`
	ID      string      `xml:"id"`
	Entries []AtomEntry `xml:"entry"`
}

// AtomLink represents a link element in an Atom feed.
type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

// AtomAuthor represents an author element in an Atom feed.
type AtomAuthor struct {
	Name  string `xml:"name"`
	Email string `xml:"email,omitempty"`
}

// AtomEntry represents an entry element in an Atom feed.
type AtomEntry struct {
	Title   string   `xml:"title"`
	Link    AtomLink `xml:"link"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary"`
}

// RSS 2.0 XML structures.

// RSSFeed represents an RSS 2.0 feed document.
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel represents a channel element in an RSS feed.
type RSSChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language,omitempty"`
	PubDate     string    `xml:"pubDate,omitempty"`
	Items       []RSSItem `xml:"item"`
}

// RSSItem represents an item element in an RSS feed.
type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate,omitempty"`
	GUID        string `xml:"guid"`
}

// FeedHandler serves an Atom or RSS 2.0 feed from the content index.
type FeedHandler struct {
	cfg   *config.Config
	index *content.Index
}

// NewFeedHandler creates a FeedHandler.
func NewFeedHandler(cfg *config.Config, index *content.Index) *FeedHandler {
	return &FeedHandler{cfg: cfg, index: index}
}

func (h *FeedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	posts := h.limitedPosts()

	if h.cfg.Feed.Type == "rss" {
		h.serveRSS(w, posts)
		return
	}
	h.serveAtom(w, posts)
}

func (h *FeedHandler) limitedPosts() []*content.Post {
	limit := h.cfg.Feed.Items
	if limit <= 0 {
		limit = 20
	}
	posts := h.index.Posts
	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts
}

func (h *FeedHandler) serveAtom(w http.ResponseWriter, posts []*content.Post) {
	baseURL := h.cfg.Site.BaseURL

	updated := time.Now().UTC().Format(time.RFC3339)
	if len(posts) > 0 {
		updated = posts[0].Date.UTC().Format(time.RFC3339)
	}

	entries := make([]AtomEntry, 0, len(posts))
	for _, p := range posts {
		entries = append(entries, AtomEntry{
			Title: p.Title,
			Link: AtomLink{
				Href: fmt.Sprintf("%s/posts/%s", baseURL, p.Slug),
				Rel:  "alternate",
			},
			ID:      fmt.Sprintf("%s/posts/%s", baseURL, p.Slug),
			Updated: p.Date.UTC().Format(time.RFC3339),
			Summary: p.Summary,
		})
	}

	feed := AtomFeed{
		XMLNS: "http://www.w3.org/2005/Atom",
		Title: h.cfg.Site.Title,
		Links: []AtomLink{
			{Href: baseURL + "/feed.xml", Rel: "self", Type: "application/atom+xml"},
			{Href: baseURL + "/", Rel: "alternate", Type: "text/html"},
		},
		Updated: updated,
		Author: AtomAuthor{
			Name:  h.cfg.Site.Author.Name,
			Email: h.cfg.Site.Author.Email,
		},
		ID:      baseURL + "/",
		Entries: entries,
	}

	writeXML(w, "application/atom+xml; charset=utf-8", feed)
}

func (h *FeedHandler) serveRSS(w http.ResponseWriter, posts []*content.Post) {
	baseURL := h.cfg.Site.BaseURL

	var pubDate string
	if len(posts) > 0 {
		pubDate = posts[0].Date.UTC().Format(time.RFC1123Z)
	}

	items := make([]RSSItem, 0, len(posts))
	for _, p := range posts {
		items = append(items, RSSItem{
			Title:       p.Title,
			Link:        fmt.Sprintf("%s/posts/%s", baseURL, p.Slug),
			Description: p.Summary,
			PubDate:     p.Date.UTC().Format(time.RFC1123Z),
			GUID:        fmt.Sprintf("%s/posts/%s", baseURL, p.Slug),
		})
	}

	feed := RSSFeed{
		Version: "2.0",
		Channel: RSSChannel{
			Title:       h.cfg.Site.Title,
			Link:        baseURL,
			Description: h.cfg.Site.Description,
			Language:    h.cfg.Site.Language,
			PubDate:     pubDate,
			Items:       items,
		},
	}

	writeXML(w, "application/rss+xml; charset=utf-8", feed)
}

func writeXML(w http.ResponseWriter, contentType string, v any) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("XML encode failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(buf.Bytes())
}
