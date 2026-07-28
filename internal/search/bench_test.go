package search

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/content"
)

// vocab is a small deterministic vocabulary for synthetic corpora.
var vocab = []string{
	"go", "search", "overlay", "filesystem", "content", "pipeline", "render",
	"cache", "template", "index", "token", "query", "score", "unicode", "café",
	"東京", "concurrency", "goroutine", "channel", "context", "handler", "server",
}

// genCorpus builds a deterministic synthetic corpus of n posts.
func genCorpus(n, bodyWords int) *content.Index {
	posts := make([]*content.Post, 0, n)
	for i := 0; i < n; i++ {
		var body strings.Builder
		for w := 0; w < bodyWords; w++ {
			body.WriteString(vocab[(i+w)%len(vocab)])
			body.WriteByte(' ')
		}
		p := mkPost(
			fmt.Sprintf("post-%d", i),
			fmt.Sprintf("%s %s post %d", vocab[i%len(vocab)], vocab[(i+3)%len(vocab)], i),
			[]string{vocab[i%len(vocab)], vocab[(i+5)%len(vocab)]},
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			body.String(),
		)
		posts = append(posts, p)
	}
	return mkIndex(posts...)
}

func BenchmarkTokenize(b *testing.B) {
	text := strings.Repeat("The quick brown fox jumps over the lazy overlay filesystem. ", 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tokenize(text)
	}
}

func benchmarkBuild(b *testing.B, n int) {
	idx := genCorpus(n, 200)
	cfg := testCfg()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si := Build(context.Background(), idx, cfg, uint64(i))
		if si.DocCount() == 0 {
			b.Fatal("empty index")
		}
	}
}

func BenchmarkBuild100(b *testing.B)   { benchmarkBuild(b, 100) }
func BenchmarkBuild1000(b *testing.B)  { benchmarkBuild(b, 1000) }
func BenchmarkBuild10000(b *testing.B) { benchmarkBuild(b, 10000) }

func benchmarkSearch(b *testing.B, query string) {
	idx := genCorpus(10000, 200)
	si := Build(context.Background(), idx, testCfg(), 1)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := si.Search(ctx, idx, query, SearchOptions{Limit: 20}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchCommon(b *testing.B)    { benchmarkSearch(b, "go") }
func BenchmarkSearchRare(b *testing.B)      { benchmarkSearch(b, "goroutine") }
func BenchmarkSearchMultiTerm(b *testing.B) { benchmarkSearch(b, "overlay filesystem content") }
func BenchmarkSearchUnicode(b *testing.B)   { benchmarkSearch(b, "café 東京") }

// BenchmarkBuildMemory10000 reports the logical index size for the default
// corpus so memory-budget regressions are visible in benchmark output.
func BenchmarkBuildMemory10000(b *testing.B) {
	idx := genCorpus(10000, 200)
	cfg := testCfg()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si := Build(context.Background(), idx, cfg, 1)
		b.ReportMetric(float64(si.LogicalBytes()), "logical_bytes")
	}
}
