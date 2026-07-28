package search

import (
	"context"
	"html/template"
	"math"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
)

const scoreEps = 1e-6

// mkPost builds a content.Post for tests. body is treated as already-rendered
// HTML (PlainText strips tags), so plain strings work directly.
func mkPost(slug, title string, tags []string, date time.Time, body string) *content.Post {
	p := &content.Post{
		Slug:    slug,
		Content: template.HTML(body), //nolint:gosec // G203: test data
	}
	p.Title = title
	p.Tags = tags
	p.Date = date
	return p
}

// mkIndex builds a minimal content.Index (Posts + BySlug) from posts.
func mkIndex(posts ...*content.Post) *content.Index {
	idx := &content.Index{BySlug: make(map[string]*content.Post, len(posts))}
	idx.Posts = posts
	for _, p := range posts {
		idx.BySlug[p.Slug] = p
	}
	return idx
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// baseCorpus is the golden fixture corpus from the design (§3.3), returned in
// scanner order (date descending).
func baseCorpus() *content.Index {
	d1 := mkPost("alpha", "Go Search", []string{"go"}, date(2026, 1, 2), "go go search")
	d2 := mkPost("beta", "Search", nil, date(2026, 1, 3), "go")
	d3 := mkPost("gamma", "Other", []string{"go"}, date(2026, 1, 1), "search search")
	return mkIndex(d2, d1, d3) // date desc: 01-03, 01-02, 01-01
}

func testCfg() config.SearchConfig {
	c := config.Default().Search
	c.Enabled = true
	return c
}

func runQuery(t *testing.T, idx *content.Index, query string) SearchResponse {
	t.Helper()
	si := Build(context.Background(), idx, testCfg(), 1)
	resp, err := si.Search(context.Background(), idx, query, SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("Search(%q) error: %v", query, err)
	}
	return resp
}

func assertOrder(t *testing.T, resp SearchResponse, wantSlugs []string) {
	t.Helper()
	if len(resp.Results) != len(wantSlugs) {
		t.Fatalf("got %d results, want %d: %+v", len(resp.Results), len(wantSlugs), slugsOf(resp))
	}
	for i, want := range wantSlugs {
		if resp.Results[i].Slug != want {
			t.Fatalf("result[%d].Slug = %q, want %q (order %v)", i, resp.Results[i].Slug, want, slugsOf(resp))
		}
	}
}

func slugsOf(resp SearchResponse) []string {
	out := make([]string, len(resp.Results))
	for i, r := range resp.Results {
		out[i] = r.Slug
	}
	return out
}

// TestSearch_GoldenRanking pins the exact weighted TF-IDF scores and ordering
// from the design's golden fixtures.
func TestSearch_GoldenRanking(t *testing.T) {
	idx := baseCorpus()
	ln2 := math.Log(2)

	t.Run("query go", func(t *testing.T) {
		resp := runQuery(t, idx, "go")
		assertOrder(t, resp, []string{"alpha", "gamma", "beta"})
		wantScores := map[string]float64{"alpha": 7 * ln2, "gamma": 2 * ln2, "beta": 1 * ln2}
		for _, r := range resp.Results {
			if math.Abs(r.Score-quantize(wantScores[r.Slug])) > scoreEps {
				t.Errorf("score(%s) = %f, want %f", r.Slug, r.Score, quantize(wantScores[r.Slug]))
			}
		}
		// Spot-check the documented decimal values.
		if math.Abs(resp.Results[0].Score-4.852030) > scoreEps {
			t.Errorf("alpha score = %f, want 4.852030", resp.Results[0].Score)
		}
	})

	t.Run("query search", func(t *testing.T) {
		resp := runQuery(t, idx, "search")
		assertOrder(t, resp, []string{"alpha", "beta", "gamma"})
		want := []float64{4 * ln2, 3 * ln2, 2 * ln2}
		for i, r := range resp.Results {
			if math.Abs(r.Score-quantize(want[i])) > scoreEps {
				t.Errorf("result[%d]=%s score %f, want %f", i, r.Slug, r.Score, quantize(want[i]))
			}
		}
	})

	t.Run("query go go equals go", func(t *testing.T) {
		a := runQuery(t, idx, "go")
		b := runQuery(t, idx, "go go")
		assertOrder(t, b, slugsOf(a))
		for i := range a.Results {
			if a.Results[i].Score != b.Results[i].Score {
				t.Errorf("duplicate term changed score: %f vs %f", a.Results[i].Score, b.Results[i].Score)
			}
		}
	})

	t.Run("query search go OR semantics with date tie-break", func(t *testing.T) {
		resp := runQuery(t, idx, "search go")
		// D1 top; D2 and D3 tie at 4*ln2 -> date desc puts beta (01-03) before gamma (01-01).
		assertOrder(t, resp, []string{"alpha", "beta", "gamma"})
		if math.Abs(resp.Results[0].Score-quantize(11*ln2)) > scoreEps {
			t.Errorf("alpha combined score = %f, want %f", resp.Results[0].Score, quantize(11*ln2))
		}
		if resp.Results[1].Score != resp.Results[2].Score {
			t.Errorf("expected beta/gamma tie, got %f vs %f", resp.Results[1].Score, resp.Results[2].Score)
		}
	})
}

// TestSearch_TFCapping verifies term frequency is capped at 255 and scored with
// the capped value.
func TestSearch_TFCapping(t *testing.T) {
	body := ""
	for i := 0; i < 300; i++ {
		body += "cap "
	}
	idx := mkIndex(mkPost("capdoc", "Title", nil, date(2026, 1, 1), body))
	si := Build(context.Background(), idx, testCfg(), 1)

	if got := si.postings["cap"][0].TF; got != 255 {
		t.Fatalf("stored TF = %d, want 255 (capped)", got)
	}
	resp, err := si.Search(context.Background(), idx, "cap", SearchOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	// N=1, df=1 -> idf=ln(2); body weight 1.0; TF 255 -> 255*ln(2).
	want := quantize(255 * math.Log(2))
	if math.Abs(resp.Results[0].Score-want) > scoreEps {
		t.Errorf("capped score = %f, want %f (176.752531)", resp.Results[0].Score, want)
	}
}

// TestSearch_TieBreaks verifies equal quantized scores break by date desc, then
// slug asc when dates are equal.
func TestSearch_TieBreaks(t *testing.T) {
	// Three docs identical except date/slug, each matching "term" once in body.
	same := date(2026, 5, 1)
	idx := mkIndex(
		mkPost("bbb", "T", nil, same, "term"),
		mkPost("aaa", "T", nil, same, "term"),
		mkPost("older", "T", nil, date(2026, 4, 1), "term"),
		mkPost("newer", "T", nil, date(2026, 6, 1), "term"),
	)
	resp := runQuery(t, idx, "term")
	// newer (06-01) first, then the same-date pair by slug asc (aaa, bbb), then older.
	assertOrder(t, resp, []string{"newer", "aaa", "bbb", "older"})
}

// TestSearch_MultiFieldRanking verifies a multi-field match outranks a body-only
// match.
func TestSearch_MultiFieldRanking(t *testing.T) {
	idx := mkIndex(
		mkPost("titlebody", "search", nil, date(2026, 1, 1), "search here"),
		mkPost("bodyonly", "Other", nil, date(2026, 1, 2), "search here"),
	)
	resp := runQuery(t, idx, "search")
	assertOrder(t, resp, []string{"titlebody", "bodyonly"})
	if resp.Results[0].Score <= resp.Results[1].Score {
		t.Errorf("multi-field score %f should exceed body-only %f", resp.Results[0].Score, resp.Results[1].Score)
	}
}

// TestSearch_NoResults returns an explicit empty state, page 1 of 1.
func TestSearch_NoResults(t *testing.T) {
	resp := runQuery(t, baseCorpus(), "nonexistentterm")
	if resp.Total != 0 || len(resp.Results) != 0 {
		t.Fatalf("expected no results, got total=%d results=%d", resp.Total, len(resp.Results))
	}
	if resp.Page != 1 || resp.TotalPages != 1 {
		t.Errorf("no-results paging = page %d of %d, want 1 of 1", resp.Page, resp.TotalPages)
	}
}

// TestSearch_QueryTooComplex rejects queries with more unique terms than allowed.
func TestSearch_QueryTooComplex(t *testing.T) {
	cfg := testCfg()
	cfg.MaxQueryTerms = 3
	si := Build(context.Background(), baseCorpus(), cfg, 1)
	_, err := si.Search(context.Background(), baseCorpus(), "one two three four five", SearchOptions{Limit: 20})
	if err != ErrQueryTooComplex {
		t.Fatalf("expected ErrQueryTooComplex, got %v", err)
	}
	// At the limit it is accepted.
	if _, err := si.Search(context.Background(), baseCorpus(), "one two three", SearchOptions{Limit: 20}); err != nil {
		t.Fatalf("query at term limit should succeed, got %v", err)
	}
}

// TestSearch_NilIndex returns ErrIndexUnavailable.
func TestSearch_NilIndex(t *testing.T) {
	var si *SearchIndex
	if _, err := si.Search(context.Background(), nil, "go", SearchOptions{}); err != ErrIndexUnavailable {
		t.Fatalf("nil index Search = %v, want ErrIndexUnavailable", err)
	}
}
