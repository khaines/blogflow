package content

import (
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"
)

// Post represents a fully processed blog post.
type Post struct {
	FrontMatter
	Slug        string
	Content     template.HTML // rendered HTML from markdown
	Summary     string        // truncated plain text
	ReadingTime int           // minutes
	Path        string        // original .md file path relative to content root
}

// Index is the in-memory content index, built by scanning the content directory.
type Index struct {
	Posts      []*Post            // sorted by date descending
	BySlug     map[string]*Post   // O(1) slug lookup
	ByTag      map[string][]*Post // posts by tag
	ByYear     map[int][]*Post    // posts by year
	Pages      []*Post            // static pages (from pages/ dir)
	PageBySlug map[string]*Post   // O(1) page slug lookup
}

// Scanner walks a content filesystem and builds an Index.
type Scanner struct {
	renderer   *Renderer
	logger     *slog.Logger
	postsDir   string // default: "posts"
	pagesDir   string // default: "pages"
	summaryLen int    // default: 200
}

// discardHandler is a slog.Handler that discards all log records.
type discardHandler struct{}

func (discardHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (discardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (discardHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return discardHandler{} }
func (discardHandler) WithGroup(_ string) slog.Handler               { return discardHandler{} }

// NewScanner creates a content scanner.
// If logger is nil, a no-op logger is used.
func NewScanner(renderer *Renderer, postsDir, pagesDir string, summaryLen int, logger *slog.Logger) *Scanner {
	if postsDir == "" {
		postsDir = "posts"
	}
	if pagesDir == "" {
		pagesDir = "pages"
	}
	if summaryLen <= 0 {
		summaryLen = 200
	}
	if logger == nil {
		logger = slog.New(discardHandler{})
	}
	return &Scanner{renderer: renderer, logger: logger, postsDir: postsDir, pagesDir: pagesDir, summaryLen: summaryLen}
}

// Scan walks the given fs.FS and builds a content Index.
// Only processes .md files. Skips drafts. Skips files without front matter.
// Individual file parse errors are logged at WARN level and skipped.
func (s *Scanner) Scan(contentFS fs.FS) (*Index, error) {
	start := time.Now()
	s.logger.Info("content scan started",
		"posts_dir", s.postsDir,
		"pages_dir", s.pagesDir,
	)

	idx := &Index{
		BySlug:     make(map[string]*Post),
		ByTag:      make(map[string][]*Post),
		ByYear:     make(map[int][]*Post),
		PageBySlug: make(map[string]*Post),
	}

	var errCount int

	// Scan posts
	if skipped, err := s.scanDir(contentFS, s.postsDir, idx, false); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scanning posts: %w", err)
		}
	} else {
		errCount += skipped
	}

	// Scan pages
	if skipped, err := s.scanDir(contentFS, s.pagesDir, idx, true); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scanning pages: %w", err)
		}
	} else {
		errCount += skipped
	}

	// Detect post/page cross-namespace slug collisions
	for slug, page := range idx.PageBySlug {
		if post, ok := idx.BySlug[slug]; ok {
			return nil, fmt.Errorf("slug conflict: page %s and post %s share slug %q", page.Path, post.Path, slug)
		}
	}

	// Sort posts by date descending
	sort.SliceStable(idx.Posts, func(i, j int) bool {
		return idx.Posts[i].Date.After(idx.Posts[j].Date)
	})

	// Rebuild secondary indexes from sorted Posts for deterministic ordering
	idx.ByTag = make(map[string][]*Post)
	idx.ByYear = make(map[int][]*Post)
	for _, post := range idx.Posts {
		for _, tag := range post.Tags {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag != "" {
				idx.ByTag[tag] = append(idx.ByTag[tag], post)
			}
		}
		idx.ByYear[post.Date.Year()] = append(idx.ByYear[post.Date.Year()], post)
	}

	s.logger.Info("content scan completed",
		"posts", len(idx.Posts),
		"pages", len(idx.Pages),
		"errors_skipped", errCount,
		"duration", time.Since(start),
	)

	return idx, nil
}

// scanDir walks a directory, logging and skipping individual file parse errors.
// Returns the count of skipped files due to errors.
func (s *Scanner) scanDir(contentFS fs.FS, dir string, idx *Index, isPage bool) (int, error) {
	var skipped int
	err := fs.WalkDir(contentFS, dir, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(filePath, ".md") {
			return nil
		}

		data, err := fs.ReadFile(contentFS, filePath)
		if err != nil {
			s.logger.Warn("skipping file: read error",
				"path", filePath,
				"error", err,
			)
			skipped++
			return nil
		}

		fm, body, err := ParseFrontMatter(data)
		if err != nil {
			s.logger.Warn("skipping file: front matter parse error",
				"path", filePath,
				"error", err,
			)
			skipped++
			return nil
		}
		if fm == nil {
			return nil // no front matter, skip
		}
		if fm.Draft {
			return nil // skip drafts
		}

		if !isPage && fm.Date.IsZero() {
			s.logger.Warn("skipping file: post missing required date",
				"path", filePath,
			)
			skipped++
			return nil
		}

		// Render markdown
		rendered, err := s.renderer.Render(body)
		if err != nil {
			s.logger.Warn("skipping file: render error",
				"path", filePath,
				"error", err,
			)
			skipped++
			return nil
		}

		// Build slug from front matter or filename
		slug := fm.Slug
		if slug == "" {
			slug = slugFromPath(filePath)
			if strings.Contains(slug, "/") || strings.Contains(slug, "\\") || strings.Contains(slug, "..") || strings.Contains(slug, "\x00") || strings.ContainsAny(slug, " \t") {
				s.logger.Warn("skipping file: invalid generated slug",
					"path", filePath,
					"slug", slug,
				)
				skipped++
				return nil
			}
		}

		// Build summary (strip HTML, truncate)
		summary := truncateText(stripHTML(string(rendered)), s.summaryLen)

		// Calculate reading time
		readingTime := fm.ReadingTime
		if readingTime == 0 {
			readingTime = ReadingTimeMinutes(string(body))
		}

		post := &Post{
			FrontMatter: *fm,
			Slug:        slug,
			Content:     template.HTML(rendered), //nolint:gosec // G203: content is sanitized by goldmark (raw HTML stripped by default)
			Summary:     summary,
			ReadingTime: readingTime,
			Path:        filePath,
		}

		if isPage {
			if existing, ok := idx.PageBySlug[slug]; ok {
				return fmt.Errorf("duplicate page slug %q: %s conflicts with %s", slug, filePath, existing.Path)
			}
			idx.Pages = append(idx.Pages, post)
			idx.PageBySlug[slug] = post
		} else {
			if existing, ok := idx.BySlug[slug]; ok {
				return fmt.Errorf("duplicate post slug %q: %s conflicts with %s", slug, filePath, existing.Path)
			}
			idx.Posts = append(idx.Posts, post)
			idx.BySlug[slug] = post
		}

		return nil
	})
	return skipped, err
}

// slugFromPath generates a slug from a file path by stripping directory and extension.
func slugFromPath(p string) string {
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

// stripHTML removes HTML tags using a simple state-machine approach.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return html.UnescapeString(b.String())
}

// truncateText shortens text to n runes at a word boundary.
func truncateText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	truncated := string(runes[:n])
	if idx := strings.LastIndex(truncated, " "); idx > 0 {
		truncated = truncated[:idx]
	}
	return truncated + "…"
}
