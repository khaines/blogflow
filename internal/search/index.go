package search

import (
	"context"
	"sort"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Field identifies which part of a post a posting came from. Field weights are
// applied during scoring so title matches outrank tag matches, which outrank
// body matches.
const (
	FieldTitle uint8 = iota
	FieldTag
	FieldBody
)

// fieldWeight returns the scoring weight for a field.
func fieldWeight(field uint8) float64 {
	switch field {
	case FieldTitle:
		return 3.0
	case FieldTag:
		return 2.0
	default:
		return 1.0
	}
}

// Posting is one (document, field) contribution for a term. Term frequency is
// capped at 255 so a posting fits in 8 bytes after alignment. No per-occurrence
// strings are retained.
type Posting struct {
	DocID uint32
	TF    uint8 // min(rawCount, 255)
	Field uint8 // FieldTitle, FieldTag, or FieldBody
}

// SearchDoc is the retained metadata for one indexed post. The full body text
// is intentionally not stored; excerpts are re-derived on demand from the
// same-generation content index.
type SearchDoc struct { //nolint:revive // name matches the full-text-search design doc; Search prefix is intentional
	ID    uint32
	Slug  string
	Title string
	Tags  []string
	Date  time.Time
}

// TruncationReason names the cap that stopped admission when an index is
// published truncated.
type TruncationReason string

// Truncation reasons reported by TruncationReason.
const (
	TruncationNone      TruncationReason = ""
	TruncationMaxDocs   TruncationReason = "max_docs"
	TruncationMaxTokens TruncationReason = "max_tokens"
	TruncationMaxBytes  TruncationReason = "max_index_bytes"
)

// SearchIndex is an immutable in-memory inverted index over post title, tags,
// and body text. It is safe for concurrent reads; it is never mutated after
// Build returns.
type SearchIndex struct { //nolint:revive // name matches the full-text-search design doc; Search prefix is intentional
	docs         []SearchDoc
	postings     map[string][]Posting // normalized token -> postings sorted by DocID
	docFreq      map[string]int       // normalized token -> number of docs containing it
	logicalBytes int64
	tokenCount   int // total posting entries
	truncated    bool
	reason       TruncationReason
	generation   uint64
	cfg          config.SearchConfig
}

// perDocOverheadBytes is the fixed logical byte estimate charged per document
// for its ID and date, on top of variable slug/title/tag string bytes.
const perDocOverheadBytes = 16

// perTokenOverheadBytes is the logical byte estimate charged once per unique
// token for map-entry and posting-slice-header overhead.
const perTokenOverheadBytes = 48

// docBuild accumulates one candidate document's postings before admission.
type docBuild struct {
	// fieldTF[field][term] = capped term frequency in that field.
	titleTF map[string]uint8
	tagTF   map[string]uint8
	bodyTF  map[string]uint8
	terms   map[string]struct{} // union of terms across fields (for docFreq)
}

// Build constructs a SearchIndex from the posts in idx using the given search
// configuration. Admission is document-granular and deterministic: posts are
// considered in the scanner's post order (date descending) and admission stops
// before a document that would exceed max_docs, max_tokens, or max_index_bytes.
// A document is never partially indexed. When truncation occurs the incomplete
// index is still returned with Truncated()==true so content keeps serving.
//
// Build never returns nil; an empty corpus yields an empty, queryable index.
func Build(ctx context.Context, idx *content.Index, cfg config.SearchConfig, generation uint64) *SearchIndex {
	tracer := otel.Tracer("github.com/khaines/blogflow/search")
	ctx, span := tracer.Start(ctx, "search.IndexBuild")
	defer span.End()

	si := &SearchIndex{
		postings:   make(map[string][]Posting),
		docFreq:    make(map[string]int),
		generation: generation,
		cfg:        cfg,
	}

	if idx != nil {
		si.build(ctx, idx.Posts)
	}

	// Postings are appended in ascending DocID order (documents are admitted in
	// order and each doc's postings are added together), so no sort is required
	// to keep them DocID-sorted. Term order within a doc is stabilized below.
	span.SetAttributes(
		attribute.Int("search.index_docs", len(si.docs)),
		attribute.Int("search.index_tokens", si.tokenCount),
		attribute.Bool("search.truncated", si.truncated),
	)
	return si
}

func (si *SearchIndex) build(ctx context.Context, posts []*content.Post) {
	for _, post := range posts {
		if ctx.Err() != nil {
			return
		}
		if post == nil {
			continue
		}
		cand := buildDoc(post)

		postingCount := len(cand.titleTF) + len(cand.tagTF) + len(cand.bodyTF)
		delta := si.candidateBytes(post, cand)

		// Document-granular cap checks. Stop before admitting a doc that would
		// exceed any cap; never partially index a document. A non-positive cap
		// means "unlimited" for that dimension, consistently across all three
		// caps (memory stays bounded by whichever caps are positive).
		if si.cfg.MaxDocs > 0 && len(si.docs) >= si.cfg.MaxDocs {
			si.markTruncated(TruncationMaxDocs)
			return
		}
		if si.cfg.MaxTokens > 0 && si.tokenCount+postingCount > si.cfg.MaxTokens {
			si.markTruncated(TruncationMaxTokens)
			return
		}
		if si.cfg.MaxIndexBytes > 0 && si.logicalBytes+delta > si.cfg.MaxIndexBytes {
			si.markTruncated(TruncationMaxBytes)
			return
		}

		si.admit(post, cand, delta)
	}
}

// candidateBytes returns the logical-byte delta of admitting this candidate,
// using the same estimator exported by the memory metric.
func (si *SearchIndex) candidateBytes(post *content.Post, cand *docBuild) int64 {
	var b int64
	b += int64(len(post.Slug)) + int64(len(post.Title)) + perDocOverheadBytes
	for _, tag := range post.Tags {
		b += int64(len(tag))
	}
	postingCount := len(cand.titleTF) + len(cand.tagTF) + len(cand.bodyTF)
	b += int64(postingCount) * 8
	for term := range cand.terms {
		if _, exists := si.postings[term]; !exists {
			b += int64(len(term)) + perTokenOverheadBytes
		}
	}
	return b
}

// admit adds a fully-vetted candidate document to the index.
func (si *SearchIndex) admit(post *content.Post, cand *docBuild, delta int64) {
	id := uint32(len(si.docs)) //nolint:gosec // G115: len(si.docs) is bounded by max_docs (validated <= 1e6), well within uint32
	tags := make([]string, len(post.Tags))
	copy(tags, post.Tags)
	si.docs = append(si.docs, SearchDoc{
		ID:    id,
		Slug:  post.Slug,
		Title: post.Title,
		Tags:  tags,
		Date:  post.Date,
	})

	si.addPostings(id, cand.titleTF, FieldTitle)
	si.addPostings(id, cand.tagTF, FieldTag)
	si.addPostings(id, cand.bodyTF, FieldBody)
	for term := range cand.terms {
		si.docFreq[term]++
	}
	si.logicalBytes += delta
}

// addPostings appends this document's postings for one field. Terms are added
// in sorted order so index construction is deterministic for a given corpus.
func (si *SearchIndex) addPostings(id uint32, tf map[string]uint8, field uint8) {
	if len(tf) == 0 {
		return
	}
	terms := make([]string, 0, len(tf))
	for t := range tf {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	for _, t := range terms {
		si.postings[t] = append(si.postings[t], Posting{DocID: id, TF: tf[t], Field: field})
		si.tokenCount++
	}
}

func (si *SearchIndex) markTruncated(reason TruncationReason) {
	si.truncated = true
	if si.reason == TruncationNone {
		si.reason = reason
	}
}

// buildDoc tokenizes a post's title, tags, and body into per-field capped term
// frequencies and the union of terms across fields.
func buildDoc(post *content.Post) *docBuild {
	d := &docBuild{
		titleTF: map[string]uint8{},
		tagTF:   map[string]uint8{},
		bodyTF:  map[string]uint8{},
		terms:   map[string]struct{}{},
	}
	addField(d.titleTF, d.terms, tokenize(post.Title))
	for _, tag := range post.Tags {
		addField(d.tagTF, d.terms, tokenize(tag))
	}
	addField(d.bodyTF, d.terms, tokenize(post.PlainText()))
	return d
}

// addField accumulates capped term frequencies for one field and records the
// terms in the document-wide union set.
func addField(tf map[string]uint8, terms map[string]struct{}, tokens []string) {
	for _, t := range tokens {
		if tf[t] < 255 {
			tf[t]++
		}
		terms[t] = struct{}{}
	}
}

// Generation returns the content generation this index was built from.
func (si *SearchIndex) Generation() uint64 { return si.generation }

// DocCount returns the number of admitted documents.
func (si *SearchIndex) DocCount() int { return len(si.docs) }

// TokenCount returns the number of indexed posting entries.
func (si *SearchIndex) TokenCount() int { return si.tokenCount }

// LogicalBytes returns the conservative logical byte estimate of the active
// index (distinct from actual Go heap usage).
func (si *SearchIndex) LogicalBytes() int64 { return si.logicalBytes }

// Truncated reports whether admission stopped early due to a cap.
func (si *SearchIndex) Truncated() bool { return si.truncated }

// TruncationReason returns the first cap that triggered truncation, or the
// empty reason when the index is complete.
func (si *SearchIndex) TruncationReason() TruncationReason { return si.reason }
