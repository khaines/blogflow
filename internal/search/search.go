package search

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/khaines/blogflow/internal/content"
	"golang.org/x/text/unicode/norm"
)

// Typed errors returned by Search. The handler maps these to HTTP status codes.
var (
	// ErrIndexUnavailable indicates the route is registered but this
	// generation's search index is nil (build failed). Handler returns 503.
	ErrIndexUnavailable = errors.New("search index unavailable")
	// ErrQueryTooComplex indicates the query has more unique normalized terms
	// than max_query_terms. Handler returns 400.
	ErrQueryTooComplex = errors.New("search query too complex")
)

// Searcher executes a query against a same-generation content index and returns
// a ranked page of results. It exists so the concrete SearchIndex can be
// swapped for an alternative implementation (e.g. a future Bleve-backed index)
// behind a stable boundary, per design §2.5.
//
// Note: the design sketches Search(ctx, *SiteSnapshot, ...); the concrete
// signature takes the same-generation *content.Index directly to avoid an
// import cycle between this package and the handlers package that owns
// SiteSnapshot. The handler always passes snapshot.Content from the same
// atomic snapshot as the SearchIndex, preserving the same-generation guarantee.
type Searcher interface {
	Search(ctx context.Context, src *content.Index, query string, opts SearchOptions) (SearchResponse, error)
}

// Compile-time assertion that *SearchIndex satisfies Searcher.
var _ Searcher = (*SearchIndex)(nil)

// scoreQuantum is the rounding granularity applied to scores before sorting so
// that ranking is reproducible and float noise does not affect tie-breaks.
const scoreQuantum = 1e6

// SearchOptions controls pagination for a query.
type SearchOptions struct { //nolint:revive // name matches the full-text-search design doc; Search prefix is intentional
	Limit  int // per-page window; defaults to cfg.MaxResults when <= 0
	Offset int // (page-1) * Limit
}

// SearchResult is a single ranked hit rendered by the search template. Every
// string field is plain text escaped by html/template; none is template.HTML.
type SearchResult struct { //nolint:revive // name matches the full-text-search design doc; Search prefix is intentional
	Title   string
	Slug    string
	URL     string
	Date    time.Time
	Excerpt string
	Score   float64 // ordering/metadata; not shown by the default result body
	Tags    []string
}

// SearchResponse is the full result of a query for one page.
type SearchResponse struct { //nolint:revive // name matches the full-text-search design doc; Search prefix is intentional
	Query      string
	Results    []SearchResult
	Total      int
	Page       int
	TotalPages int
	Duration   time.Duration
	Truncated  bool
}

// scored is an intermediate (document, quantized score) pair used for ranking.
type scored struct {
	docID     uint32
	quantized float64
}

// Search executes query against the index and returns the requested page of
// ranked results. src is the same-generation content index used only to
// re-derive excerpts for the current page; passing a different generation
// would break the same-generation excerpt guarantee and must not happen.
//
// Search validates only what the index owns: index availability and query term
// count. Query rune-length and page bounds are validated by the handler.
func (si *SearchIndex) Search(ctx context.Context, src *content.Index, query string, opts SearchOptions) (SearchResponse, error) {
	start := time.Now()
	if si == nil {
		return SearchResponse{}, ErrIndexUnavailable
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = si.cfg.MaxResults
	}
	if limit <= 0 {
		limit = 20
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	terms := parseQueryTerms(query)
	if len(terms) > si.cfg.MaxQueryTerms {
		return SearchResponse{}, ErrQueryTooComplex
	}

	resp := SearchResponse{
		Query:     query,
		Truncated: si.truncated,
		Duration:  0,
	}

	ranked := si.rank(terms)
	resp.Total = len(ranked)
	resp.TotalPages = totalPages(resp.Total, limit)
	resp.Page = offset/limit + 1

	if offset < resp.Total {
		end := offset + limit
		if end > resp.Total {
			end = resp.Total
		}
		termSet := toSet(terms)
		page := ranked[offset:end]
		resp.Results = make([]SearchResult, 0, len(page))
		for _, s := range page {
			doc := si.docs[s.docID]
			resp.Results = append(resp.Results, si.buildResult(doc, s.quantized, src, termSet))
		}
	}

	resp.Duration = time.Since(start)
	return resp, nil
}

// rank scores all documents matching any query term and returns them sorted by
// quantized score descending, then Date descending, then Slug ascending.
func (si *SearchIndex) rank(terms []string) []scored {
	if len(terms) == 0 {
		return nil
	}
	n := len(si.docs)
	scores := make(map[uint32]float64)

	// Accumulate contributions in sorted term order for reproducibility.
	for _, t := range terms {
		postings := si.postings[t]
		if len(postings) == 0 {
			continue
		}
		idf := si.idf(n, si.docFreq[t])
		for _, p := range postings {
			scores[p.DocID] += idf * fieldWeight(p.Field) * float64(p.TF)
		}
	}

	ranked := make([]scored, 0, len(scores))
	for id, sc := range scores {
		ranked = append(ranked, scored{docID: id, quantized: quantize(sc)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.quantized != b.quantized {
			return a.quantized > b.quantized
		}
		da, db := si.docs[a.docID].Date, si.docs[b.docID].Date
		if !da.Equal(db) {
			return da.After(db)
		}
		return si.docs[a.docID].Slug < si.docs[b.docID].Slug
	})
	return ranked
}

// idf computes the inverse document frequency for a term:
//
//	idf(t) = ln(1 + (N+1)/(df+1))
func (si *SearchIndex) idf(n, df int) float64 {
	return math.Log(1 + float64(n+1)/float64(df+1))
}

// buildResult assembles a SearchResult, re-deriving the excerpt from the
// same-generation content index.
func (si *SearchIndex) buildResult(doc SearchDoc, score float64, src *content.Index, termSet map[string]struct{}) SearchResult {
	var excerpt string
	if src != nil {
		if post, ok := src.BySlug[doc.Slug]; ok && post != nil {
			excerpt = makeExcerpt(post.PlainText(), termSet, si.cfg.ExcerptLength)
		}
	}
	tags := make([]string, len(doc.Tags))
	copy(tags, doc.Tags)
	return SearchResult{
		Title:   doc.Title,
		Slug:    doc.Slug,
		URL:     "/posts/" + doc.Slug,
		Date:    doc.Date,
		Excerpt: excerpt,
		Score:   score,
		Tags:    tags,
	}
}

// makeExcerpt returns a plain-text excerpt of up to length runes, centered on
// the first query-term match. When no term matches, the leading runes are used.
// The result is NFC-normalized and whitespace-collapsed, with ellipses marking
// truncation. It never contains HTML markup.
func makeExcerpt(plain string, termSet map[string]struct{}, length int) string {
	if length <= 0 {
		return ""
	}
	display := collapseWhitespace(norm.NFC.String(plain))
	if display == "" {
		return ""
	}
	runes := []rune(display)
	total := len(runes)

	matchRune := firstMatchRuneIndex(runes, termSet)
	var startRune int
	if matchRune < 0 {
		startRune = 0
	} else {
		startRune = matchRune - length/2
		if startRune < 0 {
			startRune = 0
		}
	}
	endRune := startRune + length
	if endRune > total {
		endRune = total
		if startRune > endRune-length {
			startRune = endRune - length
		}
		if startRune < 0 {
			startRune = 0
		}
	}

	var b strings.Builder
	if startRune > 0 {
		b.WriteString("…")
	}
	b.WriteString(string(runes[startRune:endRune]))
	if endRune < total {
		b.WriteString("…")
	}
	return b.String()
}

// firstMatchRuneIndex returns the rune index at which the first token that
// matches any query term begins, or -1 if none match. Matching is
// case-insensitive; runes are compared against the normalized term set.
func firstMatchRuneIndex(runes []rune, termSet map[string]struct{}) int {
	if len(termSet) == 0 {
		return -1
	}
	start := -1
	for i, r := range runes {
		if isTokenRune(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if _, ok := termSet[strings.ToLower(string(runes[start:i]))]; ok {
				return start
			}
			start = -1
		}
	}
	if start >= 0 {
		if _, ok := termSet[strings.ToLower(string(runes[start:]))]; ok {
			return start
		}
	}
	return -1
}

// collapseWhitespace replaces runs of Unicode whitespace with single spaces and
// trims the ends.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// quantize rounds a score to the fixed ranking granularity.
func quantize(score float64) float64 {
	return math.Round(score*scoreQuantum) / scoreQuantum
}

// totalPages returns the number of pages for total results at the given limit,
// never less than 1.
func totalPages(total, limit int) int {
	if limit <= 0 {
		return 1
	}
	pages := (total + limit - 1) / limit
	if pages < 1 {
		return 1
	}
	return pages
}

// toSet builds a set from a term slice.
func toSet(terms []string) map[string]struct{} {
	set := make(map[string]struct{}, len(terms))
	for _, t := range terms {
		set[t] = struct{}{}
	}
	return set
}
