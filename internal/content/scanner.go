package content

import (
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"path"
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

	return idx, nil
}

// scanDir aborts on the first file error (fail-fast). Validate content
// before deployment to avoid partial index builds.
func (s *Scanner) scanDir(contentFS fs.FS, dir string, idx *Index, isPage bool) error {
	return fs.WalkDir(contentFS, dir, func(filePath string, d fs.DirEntry, err error) error {
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
			return fmt.Errorf("reading %s: %w", filePath, err)
		}

		fm, body, err := ParseFrontMatter(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", filePath, err)
		}
		if fm == nil {
			return nil // no front matter, skip
		}
		if fm.Draft {
			return nil // skip drafts
		}

		if !isPage && fm.Date.IsZero() {
			return fmt.Errorf("parsing %s: post requires a 'date' field", filePath)
		}

		// Render markdown
		rendered, err := s.renderer.Render(body)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", filePath, err)
		}

		// Build slug from front matter or filename
		slug := fm.Slug
		if slug == "" {
			slug = slugFromPath(filePath)
			if strings.Contains(slug, "/") || strings.Contains(slug, "\\") || strings.Contains(slug, "..") || strings.Contains(slug, "\x00") || strings.ContainsAny(slug, " \t") {
				return fmt.Errorf("scanning %s: generated slug %q contains invalid characters", filePath, slug)
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
			Content:     template.HTML(rendered),
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
