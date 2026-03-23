package content

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
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
	postsDir   string // default: "posts"
	pagesDir   string // default: "pages"
	summaryLen int    // default: 200
}

// NewScanner creates a content scanner.
func NewScanner(renderer *Renderer, postsDir, pagesDir string, summaryLen int) *Scanner {
	if postsDir == "" {
		postsDir = "posts"
	}
	if pagesDir == "" {
		pagesDir = "pages"
	}
	if summaryLen <= 0 {
		summaryLen = 200
	}
	return &Scanner{renderer: renderer, postsDir: postsDir, pagesDir: pagesDir, summaryLen: summaryLen}
}

// Scan walks the given fs.FS and builds a content Index.
// Only processes .md files. Skips drafts. Skips files without front matter.
func (s *Scanner) Scan(contentFS fs.FS) (*Index, error) {
	idx := &Index{
		BySlug:     make(map[string]*Post),
		ByTag:      make(map[string][]*Post),
		ByYear:     make(map[int][]*Post),
		PageBySlug: make(map[string]*Post),
	}

	// Scan posts
	if err := s.scanDir(contentFS, s.postsDir, idx, false); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scanning posts: %w", err)
		}
	}

	// Scan pages
	if err := s.scanDir(contentFS, s.pagesDir, idx, true); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scanning pages: %w", err)
		}
	}

	// Sort posts by date descending
	sort.Slice(idx.Posts, func(i, j int) bool {
		return idx.Posts[i].Date.After(idx.Posts[j].Date)
	})

	return idx, nil
}

func (s *Scanner) scanDir(contentFS fs.FS, dir string, idx *Index, isPage bool) error {
	return fs.WalkDir(contentFS, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := fs.ReadFile(contentFS, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		fm, body, err := ParseFrontMatter(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if fm == nil {
			return nil // no front matter, skip
		}
		if fm.Draft {
			return nil // skip drafts
		}

		// Render markdown
		html, err := s.renderer.Render(body)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", path, err)
		}

		// Build slug from front matter or filename
		slug := fm.Slug
		if slug == "" {
			slug = slugFromPath(path)
		}

		// Build summary (strip HTML, truncate)
		summary := truncateText(stripHTML(string(html)), s.summaryLen)

		// Calculate reading time
		readingTime := fm.ReadingTime
		if readingTime == 0 {
			readingTime = ReadingTimeMinutes(string(body))
		}

		post := &Post{
			FrontMatter: *fm,
			Slug:        slug,
			Content:     template.HTML(html),
			Summary:     summary,
			ReadingTime: readingTime,
			Path:        path,
		}

		if isPage {
			idx.Pages = append(idx.Pages, post)
			idx.PageBySlug[slug] = post
		} else {
			idx.Posts = append(idx.Posts, post)
			idx.BySlug[slug] = post
			for _, tag := range fm.Tags {
				tag = strings.ToLower(strings.TrimSpace(tag))
				idx.ByTag[tag] = append(idx.ByTag[tag], post)
			}
			idx.ByYear[fm.Date.Year()] = append(idx.ByYear[fm.Date.Year()], post)
		}

		return nil
	})
}

// slugFromPath generates a slug from a file path by stripping directory and extension.
func slugFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
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
	return b.String()
}

// truncateText shortens text to n characters at a word boundary.
func truncateText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ") // normalize whitespace
	if len(s) <= n {
		return s
	}
	truncated := s[:n]
	if idx := strings.LastIndex(truncated, " "); idx > 0 {
		truncated = truncated[:idx]
	}
	return truncated + "…"
}
