package search

import (
	"context"
	"testing"

	"github.com/khaines/blogflow/internal/content"
)

// TestBuild_LogicalBytesEstimate locks the documented logical-byte estimator
// for a known tiny corpus.
func TestBuild_LogicalBytesEstimate(t *testing.T) {
	// slug "a" (1) + title "" (0) + tags (0) + 16 overhead = 17
	// body "go go" -> 1 posting (8 bytes); 1 unique token "go" -> 2+48 = 50
	// total = 17 + 8 + 50 = 75
	idx := mkIndex(mkPost("a", "", nil, date(2026, 1, 1), "go go"))
	si := Build(context.Background(), idx, testCfg(), 1)
	if got := si.LogicalBytes(); got != 75 {
		t.Fatalf("LogicalBytes = %d, want 75", got)
	}
	if si.TokenCount() != 1 {
		t.Fatalf("TokenCount = %d, want 1", si.TokenCount())
	}
	if si.DocCount() != 1 {
		t.Fatalf("DocCount = %d, want 1", si.DocCount())
	}
}

func TestBuild_EmptyCorpus(t *testing.T) {
	si := Build(context.Background(), mkIndex(), testCfg(), 7)
	if si == nil {
		t.Fatal("Build returned nil for empty corpus")
	}
	if si.DocCount() != 0 || si.Truncated() {
		t.Fatalf("empty corpus: docs=%d truncated=%v", si.DocCount(), si.Truncated())
	}
	if si.Generation() != 7 {
		t.Errorf("Generation = %d, want 7", si.Generation())
	}
	resp, err := si.Search(context.Background(), mkIndex(), "go", SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("query on empty index: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("empty index Total = %d, want 0", resp.Total)
	}
}

func TestBuild_NilIndex(t *testing.T) {
	si := Build(context.Background(), nil, testCfg(), 1)
	if si == nil || si.DocCount() != 0 {
		t.Fatal("Build(nil) should return an empty non-nil index")
	}
}

// TestBuild_TruncateMaxDocs stops admission at the document boundary and reports
// the reason.
func TestBuild_TruncateMaxDocs(t *testing.T) {
	cfg := testCfg()
	cfg.MaxDocs = 2
	idx := mkIndex(
		mkPost("d1", "one", nil, date(2026, 1, 3), "alpha"),
		mkPost("d2", "two", nil, date(2026, 1, 2), "beta"),
		mkPost("d3", "three", nil, date(2026, 1, 1), "gamma"),
	)
	si := Build(context.Background(), idx, cfg, 1)
	if si.DocCount() != 2 {
		t.Fatalf("DocCount = %d, want 2", si.DocCount())
	}
	if !si.Truncated() || si.TruncationReason() != TruncationMaxDocs {
		t.Fatalf("truncated=%v reason=%q, want true/max_docs", si.Truncated(), si.TruncationReason())
	}
	// The third document must not be searchable (never partially indexed).
	resp, _ := si.Search(context.Background(), idx, "gamma", SearchOptions{Limit: 20})
	if resp.Total != 0 {
		t.Errorf("truncated-out doc is searchable: total=%d", resp.Total)
	}
}

// TestBuild_TruncateMaxTokens stops when the posting budget is exhausted.
func TestBuild_TruncateMaxTokens(t *testing.T) {
	cfg := testCfg()
	cfg.MaxTokens = 1 // each doc contributes exactly one body posting
	idx := mkIndex(
		mkPost("d1", "", nil, date(2026, 1, 2), "alpha"),
		mkPost("d2", "", nil, date(2026, 1, 1), "beta"),
	)
	si := Build(context.Background(), idx, cfg, 1)
	if si.DocCount() != 1 {
		t.Fatalf("DocCount = %d, want 1", si.DocCount())
	}
	if si.TruncationReason() != TruncationMaxTokens {
		t.Fatalf("reason = %q, want max_tokens", si.TruncationReason())
	}
}

// TestBuild_TruncateMaxBytes stops when the logical byte budget is exhausted.
func TestBuild_TruncateMaxBytes(t *testing.T) {
	cfg := testCfg()
	// First doc "aa"/body "x": 2 + 16 + 8 + (1+48) = 75 bytes. Budget 100 admits
	// one doc; the second would push past 100.
	cfg.MaxIndexBytes = 100
	idx := mkIndex(
		mkPost("aa", "", nil, date(2026, 1, 2), "x"),
		mkPost("bb", "", nil, date(2026, 1, 1), "y"),
	)
	si := Build(context.Background(), idx, cfg, 1)
	if si.DocCount() != 1 {
		t.Fatalf("DocCount = %d, want 1 (bytes=%d)", si.DocCount(), si.LogicalBytes())
	}
	if si.TruncationReason() != TruncationMaxBytes {
		t.Fatalf("reason = %q, want max_index_bytes", si.TruncationReason())
	}
}

func TestBuild_NoTruncationWithDefaults(t *testing.T) {
	si := Build(context.Background(), baseCorpus(), testCfg(), 1)
	if si.Truncated() {
		t.Errorf("default caps should not truncate the tiny corpus")
	}
}

// TestBuild_NonPositiveMaxDocsMeansUnlimited verifies max_docs <= 0 is treated
// as "unlimited" (consistent with max_tokens/max_index_bytes), not as an
// index-nothing cap. This guards against a restart-scoped reload feeding a
// degenerate cap into the rebuild and silently emptying the index.
func TestBuild_NonPositiveMaxDocsMeansUnlimited(t *testing.T) {
	for _, maxDocs := range []int{0, -1} {
		cfg := testCfg()
		cfg.MaxDocs = maxDocs
		si := Build(context.Background(), baseCorpus(), cfg, 1)
		if si.DocCount() != 3 {
			t.Errorf("max_docs=%d: DocCount = %d, want 3 (unlimited)", maxDocs, si.DocCount())
		}
		if si.Truncated() {
			t.Errorf("max_docs=%d: should not truncate on doc count", maxDocs)
		}
	}
}

// TestSearch_Pagination exercises limit/offset windows and page-overflow.
func TestSearch_Pagination(t *testing.T) {
	posts := make([]*content.Post, 0, 5)
	for i, slug := range []string{"p1", "p2", "p3", "p4", "p5"} {
		// All match "term"; distinct dates keep ordering deterministic.
		posts = append(posts, mkPost(slug, "term", nil, date(2026, 1, 10-i), "term"))
	}
	idx := mkIndex(posts...)
	si := Build(context.Background(), idx, testCfg(), 1)

	page1, _ := si.Search(context.Background(), idx, "term", SearchOptions{Limit: 2, Offset: 0})
	if page1.Total != 5 || len(page1.Results) != 2 || page1.Page != 1 || page1.TotalPages != 3 {
		t.Fatalf("page1: total=%d n=%d page=%d/%d", page1.Total, len(page1.Results), page1.Page, page1.TotalPages)
	}
	page3, _ := si.Search(context.Background(), idx, "term", SearchOptions{Limit: 2, Offset: 4})
	if len(page3.Results) != 1 || page3.Page != 3 {
		t.Fatalf("page3: n=%d page=%d", len(page3.Results), page3.Page)
	}
	// Overflow: offset beyond total -> empty page, true last page reported.
	overflow, _ := si.Search(context.Background(), idx, "term", SearchOptions{Limit: 2, Offset: 10})
	if len(overflow.Results) != 0 {
		t.Fatalf("overflow should have 0 results, got %d", len(overflow.Results))
	}
	if overflow.Page != 6 || overflow.TotalPages != 3 {
		t.Errorf("overflow paging = page %d of %d, want 6 of 3", overflow.Page, overflow.TotalPages)
	}
}
