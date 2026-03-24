package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestScan_BestEffort_MixedValidInvalid(t *testing.T) {
	fs := fstest.MapFS{
		"posts/good.md": &fstest.MapFile{
			Data: []byte(mkPost("Good Post", "good", "2025-06-01", []string{"go"}, false, "Good content.\n")),
		},
		"posts/bad-fm.md": &fstest.MapFile{
			Data: []byte("---\nbad yaml: [unclosed\n---\nContent.\n"),
		},
		"posts/no-date.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"No Date\"\nslug: \"no-date\"\n---\nContent.\n"),
		},
		"posts/also-good.md": &fstest.MapFile{
			Data: []byte(mkPost("Also Good", "also-good", "2025-06-02", nil, false, "Also good.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("best-effort scan should not return error, got: %v", err)
	}
	if len(idx.Posts) != 2 {
		t.Fatalf("expected 2 valid posts, got %d", len(idx.Posts))
	}

	slugs := make(map[string]bool)
	for _, p := range idx.Posts {
		slugs[p.Slug] = true
	}
	if !slugs["good"] || !slugs["also-good"] {
		t.Errorf("expected slugs good and also-good, got %v", slugs)
	}

	// Errors() should be nil since bad-fm and no-date are "skipped" (warn-level), not collected errors
	if errs := idx.Errors(); len(errs) != 0 {
		t.Errorf("expected 0 collected errors (skips are not errors), got %d: %v", len(errs), errs)
	}
}

func TestScan_BestEffort_DuplicateSlug(t *testing.T) {
	fs := fstest.MapFS{
		"posts/first.md": &fstest.MapFile{
			Data: []byte(mkPost("First", "dupe", "2025-06-01", nil, false, "First.\n")),
		},
		"posts/second.md": &fstest.MapFile{
			Data: []byte(mkPost("Second", "dupe", "2025-06-02", nil, false, "Second.\n")),
		},
		"posts/third.md": &fstest.MapFile{
			Data: []byte(mkPost("Third", "unique", "2025-06-03", nil, false, "Third.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("best-effort scan should not return error, got: %v", err)
	}

	// Should have 2 posts: first with slug "dupe" and "unique"
	if len(idx.Posts) != 2 {
		t.Fatalf("expected 2 posts (first dupe kept + unique), got %d", len(idx.Posts))
	}
	if idx.BySlug["dupe"] == nil {
		t.Error("expected dupe slug to be indexed")
	}
	if idx.BySlug["unique"] == nil {
		t.Error("expected unique slug to be indexed")
	}

	// Should have 1 collected error for the duplicate
	errs := idx.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "duplicate post slug") {
		t.Errorf("error = %q, want it to mention duplicate post slug", errs[0])
	}
}

func TestScan_BestEffort_DuplicatePageSlug(t *testing.T) {
	fs := fstest.MapFS{
		"pages/first.md": &fstest.MapFile{
			Data: []byte(mkPost("First", "dupe", "2025-06-01", nil, false, "First.\n")),
		},
		"pages/second.md": &fstest.MapFile{
			Data: []byte(mkPost("Second", "dupe", "2025-06-02", nil, false, "Second.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("best-effort scan should not return error, got: %v", err)
	}
	if len(idx.Pages) != 1 {
		t.Fatalf("expected 1 page (first kept), got %d", len(idx.Pages))
	}

	errs := idx.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "duplicate page slug") {
		t.Errorf("error = %q, want it to mention duplicate page slug", errs[0])
	}
}

func TestScan_BestEffort_CrossNamespaceSlugConflict(t *testing.T) {
	fs := fstest.MapFS{
		"posts/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Blog", "about", "2025-06-01", nil, false, "Blog about.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Page", "about", "2025-01-01", nil, false, "About page.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("best-effort scan should not return error, got: %v", err)
	}

	// Both should be indexed in their respective namespaces
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if len(idx.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(idx.Pages))
	}

	errs := idx.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "slug conflict") {
		t.Errorf("error = %q, want it to mention slug conflict", errs[0])
	}
}

func TestScan_BestEffort_NoErrors(t *testing.T) {
	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello", "hello", "2025-06-01", nil, false, "Content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if errs := idx.Errors(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestScan_BestEffort_MultipleErrorTypes(t *testing.T) {
	fs := fstest.MapFS{
		// Valid post
		"posts/good.md": &fstest.MapFile{
			Data: []byte(mkPost("Good", "good", "2025-06-01", nil, false, "Good.\n")),
		},
		// Duplicate post slug
		"posts/dupe1.md": &fstest.MapFile{
			Data: []byte(mkPost("Dupe 1", "shared", "2025-06-02", nil, false, "Dupe 1.\n")),
		},
		"posts/dupe2.md": &fstest.MapFile{
			Data: []byte(mkPost("Dupe 2", "shared", "2025-06-03", nil, false, "Dupe 2.\n")),
		},
		// Cross-namespace conflict
		"posts/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Post", "about", "2025-06-04", nil, false, "About post.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Page", "about", "2025-01-01", nil, false, "About page.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("best-effort scan should not return error, got: %v", err)
	}

	// 3 valid posts (good + first shared + about), 1 page
	if len(idx.Posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(idx.Posts))
	}
	if len(idx.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(idx.Pages))
	}

	// 2 errors: duplicate slug + cross-namespace conflict
	errs := idx.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}

	var hasDupe, hasConflict bool
	for _, e := range errs {
		if strings.Contains(e.Error(), "duplicate post slug") {
			hasDupe = true
		}
		if strings.Contains(e.Error(), "slug conflict") {
			hasConflict = true
		}
	}
	if !hasDupe {
		t.Error("expected duplicate post slug error")
	}
	if !hasConflict {
		t.Error("expected slug conflict error")
	}
}

func TestScan_Strict_DuplicateSlugStillFails(t *testing.T) {
	fs := fstest.MapFS{
		"posts/first.md": &fstest.MapFile{
			Data: []byte(mkPost("First", "dupe", "2025-06-01", nil, false, "First.\n")),
		},
		"posts/second.md": &fstest.MapFile{
			Data: []byte(mkPost("Second", "dupe", "2025-06-02", nil, false, "Second.\n")),
		},
	}

	// Default (strict) mode — no options
	_, err := newTestScanner().Scan(fs)
	if err == nil {
		t.Fatal("expected error for duplicate slug in strict mode, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate post slug") {
		t.Errorf("error = %q, want it to mention duplicate post slug", err)
	}
}

func TestScan_Strict_CrossNamespaceConflictStillFails(t *testing.T) {
	fs := fstest.MapFS{
		"posts/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Post", "about", "2025-06-01", nil, false, "Post.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About Page", "about", "2025-01-01", nil, false, "Page.\n")),
		},
	}

	_, err := newTestScanner().Scan(fs)
	if err == nil {
		t.Fatal("expected error for slug conflict in strict mode, got nil")
	}
	if !strings.Contains(err.Error(), "slug conflict") {
		t.Errorf("error = %q, want it to mention slug conflict", err)
	}
}

func TestScan_BestEffort_ErrorsNilOnDefaultScan(t *testing.T) {
	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello", "hello", "2025-06-01", nil, false, "Content.\n")),
		},
	}

	// Default mode — Errors() should be nil
	idx, err := newTestScanner().Scan(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errs := idx.Errors(); errs != nil {
		t.Errorf("Errors() should be nil in default mode, got %v", errs)
	}
}

func TestScan_BestEffort_IndexesStillBuilt(t *testing.T) {
	fs := fstest.MapFS{
		"posts/aaa-go-post.md": &fstest.MapFile{
			Data: []byte(mkPost("Go Post", "go-post", "2025-06-01", []string{"Go"}, false, "Go content.\n")),
		},
		// Duplicate slug — will be collected as error (sorts after aaa-)
		"posts/zzz-dupe.md": &fstest.MapFile{
			Data: []byte(mkPost("Dupe", "go-post", "2025-06-02", nil, false, "Dupe.\n")),
		},
		"posts/rust-post.md": &fstest.MapFile{
			Data: []byte(mkPost("Rust Post", "rust-post", "2025-06-03", []string{"Rust"}, false, "Rust content.\n")),
		},
	}

	idx, err := newTestScanner().Scan(fs, WithBestEffort())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Secondary indexes should be built from valid posts
	if len(idx.ByTag["go"]) != 1 {
		t.Errorf("expected 1 post tagged 'go', got %d", len(idx.ByTag["go"]))
	}
	if len(idx.ByTag["rust"]) != 1 {
		t.Errorf("expected 1 post tagged 'rust', got %d", len(idx.ByTag["rust"]))
	}
	if len(idx.ByYear[2025]) != 2 {
		t.Errorf("expected 2 posts in 2025, got %d", len(idx.ByYear[2025]))
	}

	// Posts should be sorted by date descending
	if idx.Posts[0].Slug != "rust-post" {
		t.Errorf("first post should be rust-post (newest), got %s", idx.Posts[0].Slug)
	}
}
