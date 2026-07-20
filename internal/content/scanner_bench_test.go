package content

import (
	"fmt"
	"testing"
	"testing/fstest"
)

func benchContentFS(posts, pages int) fstest.MapFS {
	m := make(fstest.MapFS, posts+pages+2)
	for i := range posts {
		m[fmt.Sprintf("posts/2026/post-%04d.md", i)] = &fstest.MapFile{Data: []byte(benchPostMarkdown(i))}
	}
	for i := range pages {
		m[fmt.Sprintf("pages/page-%04d.md", i)] = &fstest.MapFile{Data: []byte(benchPageMarkdown(i))}
	}
	m["posts/draft.md"] = &fstest.MapFile{Data: []byte(mkPost("Draft", "draft", "2026-01-01", nil, true, "draft"))}
	m["static/ignored.txt"] = &fstest.MapFile{Data: []byte("not markdown")}
	return m
}

func benchPostMarkdown(i int) string {
	return mkPost(
		fmt.Sprintf("Benchmark Post %04d", i),
		fmt.Sprintf("benchmark-post-%04d", i),
		fmt.Sprintf("2026-01-%02d", (i%28)+1),
		[]string{fmt.Sprintf("tag-%02d", i%12), "reload"},
		false,
		fmt.Sprintf("# Heading %04d\n\nThis post exercises **markdown rendering**, summaries, tag indexes, and date sorting during reload scans.\n\n- item one\n- item two\n", i),
	)
}

func benchPageMarkdown(i int) string {
	return mkPage(
		fmt.Sprintf("Benchmark Page %04d", i),
		fmt.Sprintf("benchmark-page-%04d", i),
		i+1,
		"This page exercises static page indexing during content reload scans.\n",
	)
}

// BenchmarkContentReindex measures the content rescan/re-index entry point used
// by cmd/blogflow's ContentReloader after sync strategies report a change.
func BenchmarkContentReindex(b *testing.B) {
	fsys := benchContentFS(500, 75)
	scanner := NewScanner(NewRenderer(), "posts", "pages", 200, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		idx, err := scanner.Scan(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if len(idx.Posts) != 500 || len(idx.Pages) != 75 {
			b.Fatalf("Scan indexed %d posts/%d pages, want 500/75", len(idx.Posts), len(idx.Pages))
		}
	}
}
