package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

func mkPost(title, slug, date string, tags []string, draft bool, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: \"" + title + "\"\n")
	if slug != "" {
		b.WriteString("slug: \"" + slug + "\"\n")
	}
	b.WriteString("date: " + date + "\n")
	if draft {
		b.WriteString("draft: true\n")
	}
	if len(tags) > 0 {
		b.WriteString("tags: [\"" + strings.Join(tags, "\", \"") + "\"]\n")
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}

func newTestScanner() *Scanner {
	return NewScanner(NewRenderer(), "", "", 200)
}

func TestScan_BasicPost(t *testing.T) {
	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello World", "hello-world", "2025-06-01", []string{"go"}, false, "This is **bold** text.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}

	post := idx.Posts[0]
	if post.Slug != "hello-world" {
		t.Errorf("slug = %q, want %q", post.Slug, "hello-world")
	}
	if post.Title != "Hello World" {
		t.Errorf("title = %q, want %q", post.Title, "Hello World")
	}
	if !strings.Contains(string(post.Content), "<strong>bold</strong>") {
		t.Errorf("content missing rendered bold: %s", post.Content)
	}
	if post.Summary == "" {
		t.Error("summary should not be empty")
	}
	if post.ReadingTime < 1 {
		t.Errorf("reading time = %d, want >= 1", post.ReadingTime)
	}
	if post.Path != "posts/hello.md" {
		t.Errorf("path = %q, want %q", post.Path, "posts/hello.md")
	}

	// Verify index lookups
	if idx.BySlug["hello-world"] != post {
		t.Error("BySlug lookup failed")
	}
}

func TestScan_MultiplePosts(t *testing.T) {
	fs := fstest.MapFS{
		"posts/old.md": &fstest.MapFile{
			Data: []byte(mkPost("Old Post", "old", "2024-01-01", nil, false, "Old content.\n")),
		},
		"posts/mid.md": &fstest.MapFile{
			Data: []byte(mkPost("Mid Post", "mid", "2025-03-15", nil, false, "Mid content.\n")),
		},
		"posts/new.md": &fstest.MapFile{
			Data: []byte(mkPost("New Post", "new", "2025-06-01", nil, false, "New content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(idx.Posts))
	}

	// Verify date descending order
	if idx.Posts[0].Slug != "new" {
		t.Errorf("first post slug = %q, want %q", idx.Posts[0].Slug, "new")
	}
	if idx.Posts[1].Slug != "mid" {
		t.Errorf("second post slug = %q, want %q", idx.Posts[1].Slug, "mid")
	}
	if idx.Posts[2].Slug != "old" {
		t.Errorf("third post slug = %q, want %q", idx.Posts[2].Slug, "old")
	}
}

func TestScan_SkipDrafts(t *testing.T) {
	fs := fstest.MapFS{
		"posts/published.md": &fstest.MapFile{
			Data: []byte(mkPost("Published", "pub", "2025-06-01", nil, false, "Content.\n")),
		},
		"posts/draft.md": &fstest.MapFile{
			Data: []byte(mkPost("Draft", "draft", "2025-06-02", nil, true, "Draft content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post (draft skipped), got %d", len(idx.Posts))
	}
	if idx.Posts[0].Slug != "pub" {
		t.Errorf("expected published post, got slug %q", idx.Posts[0].Slug)
	}
}

func TestScan_SkipNonMarkdown(t *testing.T) {
	fs := fstest.MapFS{
		"posts/good.md": &fstest.MapFile{
			Data: []byte(mkPost("Good", "good", "2025-06-01", nil, false, "Content.\n")),
		},
		"posts/readme.txt": &fstest.MapFile{
			Data: []byte("This is a text file"),
		},
		"posts/data.json": &fstest.MapFile{
			Data: []byte(`{"key": "value"}`),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post (non-md skipped), got %d", len(idx.Posts))
	}
}

func TestScan_NoFrontMatter(t *testing.T) {
	fs := fstest.MapFS{
		"posts/with-fm.md": &fstest.MapFile{
			Data: []byte(mkPost("With FM", "with-fm", "2025-06-01", nil, false, "Content.\n")),
		},
		"posts/no-fm.md": &fstest.MapFile{
			Data: []byte("# Just Markdown\n\nNo front matter here.\n"),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post (no-fm skipped), got %d", len(idx.Posts))
	}
	if idx.Posts[0].Slug != "with-fm" {
		t.Errorf("expected with-fm post, got slug %q", idx.Posts[0].Slug)
	}
}

func TestScan_Pages(t *testing.T) {
	fs := fstest.MapFS{
		"posts/blog.md": &fstest.MapFile{
			Data: []byte(mkPost("Blog Post", "blog", "2025-06-01", nil, false, "Blog.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About", "about", "2025-01-01", nil, false, "About page.\n")),
		},
		"pages/contact.md": &fstest.MapFile{
			Data: []byte(mkPost("Contact", "contact", "2025-01-01", nil, false, "Contact page.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if len(idx.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(idx.Pages))
	}

	if idx.PageBySlug["about"] == nil {
		t.Error("PageBySlug missing 'about'")
	}
	if idx.PageBySlug["contact"] == nil {
		t.Error("PageBySlug missing 'contact'")
	}

	// Pages should not appear in BySlug (post-only index)
	if idx.BySlug["about"] != nil {
		t.Error("pages should not appear in BySlug")
	}
}

func TestScan_TagIndex(t *testing.T) {
	fs := fstest.MapFS{
		"posts/go-post.md": &fstest.MapFile{
			Data: []byte(mkPost("Go Post", "go-post", "2025-06-01", []string{"Go", "Programming"}, false, "Go.\n")),
		},
		"posts/rust-post.md": &fstest.MapFile{
			Data: []byte(mkPost("Rust Post", "rust-post", "2025-06-02", []string{"Rust", "Programming"}, false, "Rust.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tags are lowercased
	if len(idx.ByTag["go"]) != 1 {
		t.Errorf("expected 1 post tagged 'go', got %d", len(idx.ByTag["go"]))
	}
	if len(idx.ByTag["rust"]) != 1 {
		t.Errorf("expected 1 post tagged 'rust', got %d", len(idx.ByTag["rust"]))
	}
	if len(idx.ByTag["programming"]) != 2 {
		t.Errorf("expected 2 posts tagged 'programming', got %d", len(idx.ByTag["programming"]))
	}
}

func TestScan_YearIndex(t *testing.T) {
	fs := fstest.MapFS{
		"posts/post2024.md": &fstest.MapFile{
			Data: []byte(mkPost("2024 Post", "post-2024", "2024-05-10", nil, false, "Old.\n")),
		},
		"posts/post2025a.md": &fstest.MapFile{
			Data: []byte(mkPost("2025 Post A", "post-2025a", "2025-03-01", nil, false, "New A.\n")),
		},
		"posts/post2025b.md": &fstest.MapFile{
			Data: []byte(mkPost("2025 Post B", "post-2025b", "2025-06-15", nil, false, "New B.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(idx.ByYear[2024]) != 1 {
		t.Errorf("expected 1 post in 2024, got %d", len(idx.ByYear[2024]))
	}
	if len(idx.ByYear[2025]) != 2 {
		t.Errorf("expected 2 posts in 2025, got %d", len(idx.ByYear[2025]))
	}
}

func TestScan_SlugFromFilename(t *testing.T) {
	// Post without slug in front matter should derive slug from filename
	fs := fstest.MapFS{
		"posts/my-great-post.md": &fstest.MapFile{
			Data: []byte(mkPost("My Great Post", "", "2025-06-01", nil, false, "Content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if idx.Posts[0].Slug != "my-great-post" {
		t.Errorf("slug = %q, want %q", idx.Posts[0].Slug, "my-great-post")
	}
	if idx.BySlug["my-great-post"] == nil {
		t.Error("BySlug lookup by filename-derived slug failed")
	}
}

func TestScan_EmptyDir(t *testing.T) {
	// posts directory exists but is empty
	fs := fstest.MapFS{
		"posts/.gitkeep": &fstest.MapFile{Data: []byte{}},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 0 {
		t.Errorf("expected 0 posts, got %d", len(idx.Posts))
	}
}

func TestScan_MissingDir(t *testing.T) {
	// Completely empty FS — no posts or pages dirs
	fs := fstest.MapFS{}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 0 {
		t.Errorf("expected 0 posts, got %d", len(idx.Posts))
	}
	if len(idx.Pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(idx.Pages))
	}
}

func TestSlugFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"posts/hello-world.md", "hello-world"},
		{"posts/sub/nested.md", "nested"},
		{"my-page.md", "my-page"},
		{"posts/2025-01-01-post.md", "2025-01-01-post"},
		{"file.markdown", "file"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := slugFromPath(tt.path)
			if got != tt.want {
				t.Errorf("slugFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"simple tags", "<p>hello</p>", "hello"},
		{"nested tags", "<div><p><strong>bold</strong> text</p></div>", "bold text"},
		{"empty tags", "<br/><hr/>", ""},
		{"mixed content", "before <em>mid</em> after", "before mid after"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name string
		text string
		n    int
		want string
	}{
		{"short text", "hello world", 200, "hello world"},
		{"exact length", "hello", 5, "hello"},
		{"truncate at word boundary", "the quick brown fox jumps over the lazy dog", 20, "the quick brown fox…"},
		{"whitespace normalization", "  hello   world  ", 200, "hello world"},
		{"empty string", "", 200, ""},
		{"unicode rune boundary", "こんにちは世界のテスト", 5, "こんにちは…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.text, tt.n)
			if got != tt.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.n, got, tt.want)
			}
		})
	}
}

func TestScan_DuplicateSlug(t *testing.T) {
	fs := fstest.MapFS{
		"posts/first.md": &fstest.MapFile{
			Data: []byte(mkPost("First", "dupe", "2025-06-01", nil, false, "First.\n")),
		},
		"posts/second.md": &fstest.MapFile{
			Data: []byte(mkPost("Second", "dupe", "2025-06-02", nil, false, "Second.\n")),
		},
	}

	_, err := newTestScanner().Scan(fs)
	if err == nil {
		t.Fatal("expected error for duplicate slug, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate post slug") {
		t.Errorf("error = %q, want it to mention duplicate post slug", err)
	}
}

func TestScan_DuplicatePageSlug(t *testing.T) {
	fs := fstest.MapFS{
		"pages/first.md": &fstest.MapFile{
			Data: []byte(mkPost("First", "dupe", "2025-06-01", nil, false, "First.\n")),
		},
		"pages/second.md": &fstest.MapFile{
			Data: []byte(mkPost("Second", "dupe", "2025-06-02", nil, false, "Second.\n")),
		},
	}

	_, err := newTestScanner().Scan(fs)
	if err == nil {
		t.Fatal("expected error for duplicate page slug, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate page slug") {
		t.Errorf("error = %q, want it to mention duplicate page slug", err)
	}
}

func TestScan_MissingDate(t *testing.T) {
	fs := fstest.MapFS{
		"posts/no-date.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"No Date\"\nslug: \"no-date\"\n---\nContent.\n"),
		},
	}

	_, err := newTestScanner().Scan(fs)
	if err == nil {
		t.Fatal("expected error for missing date, got nil")
	}
	if !strings.Contains(err.Error(), "requires a 'date' field") {
		t.Errorf("error = %q, want it to mention date requirement", err)
	}
}

func TestScan_PageAllowedWithoutDate(t *testing.T) {
	fs := fstest.MapFS{
		"pages/about.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"About\"\nslug: \"about\"\n---\nAbout page.\n"),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("pages should allow missing date, got error: %v", err)
	}
	if len(idx.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(idx.Pages))
	}
}

func TestScan_EmptyTagSkipped(t *testing.T) {
	fs := fstest.MapFS{
		"posts/post.md": &fstest.MapFile{
			Data: []byte(mkPost("Post", "post", "2025-06-01", []string{"go", " ", ""}, false, "Content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := idx.ByTag[""]; ok {
		t.Error("empty string tag should not be indexed")
	}
	if len(idx.ByTag["go"]) != 1 {
		t.Errorf("expected 1 post tagged 'go', got %d", len(idx.ByTag["go"]))
	}
}
