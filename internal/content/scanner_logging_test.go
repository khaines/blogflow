package content

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestScan_LogsScanStarted(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello", "hello", "2025-06-01", nil, false, "Content.\n")),
		},
	}
	scanner := NewScanner(NewRenderer(), "", "", 200, logger)
	if _, err := scanner.Scan(fs); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "content scan started") {
		t.Errorf("expected 'content scan started' log, got:\n%s", out)
	}
	if !strings.Contains(out, "posts_dir=posts") {
		t.Errorf("expected posts_dir=posts in log, got:\n%s", out)
	}
	if !strings.Contains(out, "pages_dir=pages") {
		t.Errorf("expected pages_dir=pages in log, got:\n%s", out)
	}
}

func TestScan_LogsScanCompleted(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fs := fstest.MapFS{
		"posts/a.md": &fstest.MapFile{
			Data: []byte(mkPost("A", "a", "2025-06-01", nil, false, "A content.\n")),
		},
		"posts/b.md": &fstest.MapFile{
			Data: []byte(mkPost("B", "b", "2025-06-02", nil, false, "B content.\n")),
		},
		"pages/about.md": &fstest.MapFile{
			Data: []byte(mkPost("About", "about", "2025-01-01", nil, false, "About page.\n")),
		},
	}
	scanner := NewScanner(NewRenderer(), "", "", 200, logger)
	if _, err := scanner.Scan(fs); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "content scan completed") {
		t.Errorf("expected 'content scan completed' log, got:\n%s", out)
	}
	if !strings.Contains(out, "posts=2") {
		t.Errorf("expected posts=2 in log, got:\n%s", out)
	}
	if !strings.Contains(out, "pages=1") {
		t.Errorf("expected pages=1 in log, got:\n%s", out)
	}
	if !strings.Contains(out, "errors_skipped=0") {
		t.Errorf("expected errors_skipped=0 in log, got:\n%s", out)
	}
	if !strings.Contains(out, "duration=") {
		t.Errorf("expected duration in log, got:\n%s", out)
	}
}

func TestScan_LogsFileParseErrorAndContinues(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fs := fstest.MapFS{
		"posts/good.md": &fstest.MapFile{
			Data: []byte(mkPost("Good", "good", "2025-06-01", nil, false, "Content.\n")),
		},
		"posts/bad.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"Bad\"\nslug: \"bad\"\n---\nContent.\n"), // missing date
		},
	}
	scanner := NewScanner(NewRenderer(), "", "", 200, logger)
	idx, err := scanner.Scan(fs)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	// Good post should still be indexed
	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}
	if idx.Posts[0].Slug != "good" {
		t.Errorf("expected good post, got slug %q", idx.Posts[0].Slug)
	}

	out := buf.String()
	// Should have a WARN for the bad file
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("expected WARN level log for parse error, got:\n%s", out)
	}
	if !strings.Contains(out, "posts/bad.md") {
		t.Errorf("expected bad.md path in warn log, got:\n%s", out)
	}
	// Completion log should show errors_skipped=1
	if !strings.Contains(out, "errors_skipped=1") {
		t.Errorf("expected errors_skipped=1 in log, got:\n%s", out)
	}
}

func TestScan_NilLoggerDoesNotPanic(t *testing.T) {
	fs := fstest.MapFS{
		"posts/hello.md": &fstest.MapFile{
			Data: []byte(mkPost("Hello", "hello", "2025-06-01", nil, false, "Content.\n")),
		},
	}
	scanner := NewScanner(NewRenderer(), "", "", 200, nil)
	if _, err := scanner.Scan(fs); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
}

func TestScan_LogsMultipleParseErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	fs := fstest.MapFS{
		"posts/good.md": &fstest.MapFile{
			Data: []byte(mkPost("Good", "good", "2025-06-01", nil, false, "OK.\n")),
		},
		"posts/no-date1.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"No Date 1\"\nslug: \"nd1\"\n---\nContent.\n"),
		},
		"posts/no-date2.md": &fstest.MapFile{
			Data: []byte("---\ntitle: \"No Date 2\"\nslug: \"nd2\"\n---\nContent.\n"),
		},
	}
	scanner := NewScanner(NewRenderer(), "", "", 200, logger)
	idx, err := scanner.Scan(fs)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(idx.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(idx.Posts))
	}

	out := buf.String()
	if !strings.Contains(out, "errors_skipped=2") {
		t.Errorf("expected errors_skipped=2 in log, got:\n%s", out)
	}
}
